package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"go.opentelemetry.io/otel"

	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/envflag"
	"github.com/holistics/anfra/internal/exports"
	"github.com/holistics/anfra/internal/logging"
	localquerycache "github.com/holistics/anfra/internal/querycache/local"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/sidecar/canallake"
	"github.com/holistics/anfra/internal/telemetry"

	"github.com/holistics/anfra/shared/logkit"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// commandContext is the CommandContext both surfaces run commands with — the
// one-shot CLI and serve's /call. The local user owns the repo they pointed
// anfra at, so no data restrictions apply. That is stated rather than
// defaulted: a zero dataperm.Set is refused, not treated as permissive.
//
// Its Attribution names the repo, so traces of several repos in one backend
// can be told apart (repoAttribution). One tenant, LocalTenant: there is one
// user, reading their own logs.
//
// Each query is written onto its span (telemetry.Queries), and the user's data
// recorded as the ANFRA_TRACE_* settings say (docs/designs/telemetry.md).
func (h hostContext) commandContext(clients command.Clients) command.CommandContext {
	cc := command.CommandContext{
		Clients:       clients,
		Exports:       exports.Store{Files: h.exportFiles}, // file:// links; serve links them where it serves them
		Repo:          h.repo.Repo(),
		QueryCache:    h.queryCache,
		QueryDefaults: h.queryDefaults,
		TenantID:      LocalTenant,
		Attribution:   repoAttribution(h.repo),
		DataPerms:     dataperm.Ungoverned(),
		// anfra core serves the directory it was pointed at: the local repo is the
		// point, not a mistake.
		AllowLocalRepo: true,
		Observers:      command.Observers{Queries: telemetry.Queries{}},
		TraceData:      traceData.Settings(),
		Telemetry:      telemetryInfo,
	}
	if !updateNotifyDisabled() {
		cc.Update = knownUpdate
		cc.Skills = knownSkills
	}
	return cc
}

// repoAttribution names the repo on every span and log record of a command in
// it (docs/designs/telemetry.md, the caller): its repo id, the folder name and
// a hash of its path, which names its state folder and tells two checkouts
// apart. Never its path, which often holds the user's name. Nothing for a
// folder that isn't a repo.
func repoAttribution(r repo.Local) attribution.Fields {
	if !r.IsRepo() {
		return nil
	}
	return attribution.Fields{caller.RepoID.String(): r.ID}
}

// LocalTenant is the one tenant anfra's CLI and serve run for.
const LocalTenant = "local"

// hostContext carries the per-invocation repo + the sidecar Config (with the
// host-aggregated log sink) so commands can spawn whichever sidecars they need.
// The context is passed to fn as a parameter (not stored here) so it stays a
// properly-inherited, cancelable value.
type hostContext struct {
	repo repo.Local
	cfg  sidecar.Config
	// exportFiles are this process's exports' files: the one-shot CLI's, or the
	// server's, which closes them when it stops.
	exportFiles *exports.Files
	// appLogger is where the repo's Data Apps' reports from the browser go: anfra.log, as
	// "service":"app".
	appLogger *slog.Logger
	// queryCache is the repo's query cache, its store opened at first use.
	queryCache *localquerycache.Cache
	// queryDefaults are what the repo's queries get when they set none.
	queryDefaults command.QueryDefaults
}

// withRepo resolves the repo and sets up host-aggregated logging, then runs fn
// with the caller's (signal-cancelable) context and the host context. Sidecar
// lifecycle is the command's choice (some need only anfra-node, query execution
// also needs canal-query). logStderr is whether the log also goes to stderr
// when ANFRA_LOG_STDERR is unset: a server's does, a command's does not.
func withRepo(ctx context.Context, logStderr bool, fn func(ctx context.Context, h hostContext) error) error {
	repoDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve repo dir: %w", err)
	}
	repo := repo.Resolve(repoDir)

	lg, err := logging.Setup(repo.LogsDir(), repo.ID, envflag.OnOr(envflag.LogStderr, logStderr))
	if err != nil {
		return fmt.Errorf("set up the repo's log in anfra's folder (ANFRA_HOME): %w", err)
	}
	defer lg.Close()
	// The exporter's failures (a collector down) go to the log, not the terminal.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { reportTelemetryFailure(lg.Logger, err) }))

	cfg := sidecar.Config{
		RepoID:           repo.ID,
		CompileCachePath: filepath.Join(repo.CacheDir(), "compile-cache"),
		StderrWriter:     lg.StderrWriter, // sidecar stderr -> the log stream (anfra.log)
		StdoutWriter:     lg.StdoutWriter, // sidecar stdout -> discarded / host stdout
		Logger:           lg.Logger,
		// Point the CLI at sidecars it does not own. Unset (the normal case)
		// means spawn them; set means a compose/k8s deployment already runs
		// them, and this process only dials. Useful for development against
		// `docker compose up` without rebuilding an embedded binary.
		NodeURL:       os.Getenv("ANFRA_NODE_URL"),
		CanalQueryURL: os.Getenv("ANFRA_CANAL_QUERY_URL"),
		WrapTransport: traceSidecar,
	}
	// The settings file is read once: a server's at startup.
	settings, settingsErr := repoconfig.Read(repo.Dir)
	if settingsErr != nil {
		lg.Logger.Warn("Repo settings unusable", logkit.EventKey, eventQueryCacheSettingsUnknown, "path", repoconfig.Path(repo.Dir), "error", settingsErr,
			"hint", "queries are refused until the settings file is fixed")
	}
	queryCache := newQueryCache(repo, cfg, settings, settingsErr)
	// canal-query reaches canal-lake at its socket, chosen before either runs.
	cfg.CanalLakeSocket = queryCache.lakeSocket
	defer queryCache.Close(context.WithoutCancel(ctx))

	return fn(ctx, hostContext{
		repo:          repo,
		exportFiles:   exports.New(exportLinkLife),
		appLogger:     lg.App,
		queryCache:    queryCache.Cache,
		queryDefaults: queryDefaults(settings, settingsErr),
		cfg:           cfg,
	})
}

// queryCache is the repo's query cache, and its canal-lake's socket: empty
// when the cache is off, which runs no canal-lake.
type queryCache struct {
	*localquerycache.Cache
	lakeSocket string
}

// newQueryCache is the repo's query cache, under its folder in anfra's home.
// Its canal-lake is chosen a socket now, and started when first needed. One
// whose socket can't be chosen fails when started, so a command that never
// caches isn't failed by it. A canal-query at a URL is a deployment's, which
// runs its canal-lake too: anfra starts none.
func newQueryCache(repo repo.Local, cfg sidecar.Config, settings repoconfig.Config, settingsErr error) queryCache {
	dir := filepath.Join(repo.CacheDir(), "query")
	disabled := queryCacheDisabled(settings, settingsErr)
	o := localquerycache.Options{
		Disabled: disabled, MaxDuration: settings.QueryCacheSettings().MaxDuration, Namespace: os.Getenv(localquerycache.NamespaceEnv),
		Report: func(ctx context.Context, err error) {
			cfg.Logger.WarnContext(ctx, "Query ran without the cache", logkit.EventKey, eventQueryCacheUnavailable, "error", err,
				"hint", "the query cache failed, so the query ran on the warehouse, uncached; a sidecar's lines just before this one may say why")
		},
		Notice: func(ctx context.Context, n localquerycache.Notice) {
			cfg.Logger.WarnContext(ctx, "Query cache folder holds orphaned files", logkit.EventKey, eventQueryCacheOrphanedFiles,
				"dir", localquerycache.LakeDir(dir), "hint", n.Message)
		},
	}
	switch {
	case disabled != "":
		return queryCache{Cache: localquerycache.New(dir, o)}
	case cfg.CanalQueryURL != "":
		o.Lake = deployedLake{}
		return queryCache{Cache: localquerycache.New(dir, o)}
	}
	lake, err := canallake.New(cfg, localquerycache.LakeDir(dir))
	if err != nil {
		o.Lake = failedLake{err}
		return queryCache{Cache: localquerycache.New(dir, o)}
	}
	o.Lake = lake
	return queryCache{Cache: localquerycache.New(dir, o), lakeSocket: lake.SocketPath()}
}

// deployedLake is the canal-lake a deployment runs beside its canal-query:
// anfra neither starts nor stops it.
type deployedLake struct{}

func (deployedLake) Start(context.Context) error { return nil }
func (deployedLake) Close(context.Context)       {}

// failedLake is a canal-lake that couldn't be set up: starting it fails.
type failedLake struct{ err error }

func (l failedLake) Start(context.Context) error { return l.err }
func (failedLake) Close(context.Context)         {}

// queryCacheDisabled says what turns the repo's query cache off, if anything:
// ANFRA_QUERY_CACHE_ENABLED, whatever the repo sets, or the repo's
// query.cache.enabled, from its settings file as Read returned it. A file that
// can't be used leaves the cache on: queries are refused anyway (queryDefaults).
func queryCacheDisabled(c repoconfig.Config, readErr error) string {
	switch {
	case os.Getenv(envflag.QueryCacheEnabled) != "":
		if !envflag.On(envflag.QueryCacheEnabled) {
			return envflag.QueryCacheEnabled + "=0 turns the query cache off for this process."
		}
	case readErr == nil && !c.QueryCacheEnabled():
		return repoconfig.RelPath + " turns the query cache off (query.cache.enabled: false)."
	}
	return ""
}

// queryDefaults are the repo's query defaults, from its settings file as Read
// returned it: when it can't be used, why, which refuses every query.
func queryDefaults(c repoconfig.Config, readErr error) command.QueryDefaults {
	if readErr != nil {
		return command.QueryDefaults{Err: readErr}
	}
	return c.QueryDefaults()
}
