package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel"

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

	"github.com/holistics/anfra/shared/logkit"
)

// commandContext is the CommandContext both surfaces run commands with — the
// one-shot CLI and serve's /call. The local user owns the repo they pointed
// anfra at, so no data restrictions apply. That is stated rather than
// defaulted: a zero dataperm.Set is refused, not treated as permissive.
//
// No Attribution: there is one user and they are reading their own logs. One
// tenant, LocalTenant, for the same reason.
func (h hostContext) commandContext(clients command.Clients) command.CommandContext {
	cc := command.CommandContext{
		Clients:    clients,
		Exports:    exports.Store{Files: h.exportFiles}, // file:// links; serve links them where it serves them
		Repo:       h.repo.Repo(),
		QueryCache: h.queryCache,
		TenantID:   LocalTenant,
		DataPerms:  dataperm.Ungoverned(),
		// anfra core serves the directory it was pointed at: the local repo is the
		// point, not a mistake.
		AllowLocalRepo: true,
	}
	if !updateNotifyDisabled() {
		cc.Update = knownUpdate
		cc.Skills = knownSkills
	}
	return cc
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
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		lg.Logger.Warn("Telemetry failed", logkit.EventKey, eventTelemetryFailed, "error", err)
	}))

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
	queryCache := newQueryCache(repo, cfg)
	// canal-query reaches canal-lake at its socket, chosen before either runs.
	cfg.CanalLakeSocket = queryCache.lakeSocket
	defer queryCache.Close(context.WithoutCancel(ctx))

	return fn(ctx, hostContext{
		repo:        repo,
		exportFiles: exports.New(exportLinkLife),
		appLogger:   lg.App,
		queryCache:  queryCache.Cache,
		cfg:         cfg,
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
func newQueryCache(repo repo.Local, cfg sidecar.Config) queryCache {
	dir := filepath.Join(repo.CacheDir(), "query")
	disabled, defaultDuration := queryCacheSettings(repo.Dir, cfg.Logger)
	o := localquerycache.Options{
		Disabled: disabled, Namespace: os.Getenv(localquerycache.NamespaceEnv), Default: command.QueryCacheDefaults{Duration: defaultDuration},
		Report: func(ctx context.Context, err error) {
			cfg.Logger.WarnContext(ctx, "Query ran without the cache", logkit.EventKey, eventQueryCacheUnavailable, "error", err,
				"hint", "the query cache failed, so the query ran on the warehouse, uncached; anfra status shows the cache's state")
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

// queryCacheSettings are the repo's query cache settings: what turns the
// cache off, if anything (ANFRA_CACHE_QUERY_ENABLED, whatever the repo sets,
// or the repo's cache.query.enabled), and the default duration of a query
// that sets none (cache.query.default.duration). A settings file that can't be
// read or parsed, or a default duration anfra doesn't take, leaves the
// defaults (the cache on, no default duration), and is logged.
func queryCacheSettings(repoDir string, logger *slog.Logger) (disabled string, defaultDuration time.Duration) {
	c, readErr := repoconfig.Read(repoDir)
	if readErr != nil {
		logger.Warn("Query cache settings unknown", logkit.EventKey, eventQueryCacheSettingsUnknown, "path", repoconfig.Path(repoDir), "error", readErr,
			"hint", "the query cache keeps its defaults until anfra can read its settings from the file")
	} else if d, err := c.QueryCacheDefaultDuration(); err != nil {
		logger.Warn("Query cache settings unknown", logkit.EventKey, eventQueryCacheSettingsUnknown, "path", repoconfig.Path(repoDir), "error", err,
			"hint", "queries that set no cache.duration run without the cache until the setting is fixed")
	} else {
		defaultDuration = d
	}
	switch {
	case os.Getenv(envflag.CacheQueryEnabled) != "":
		if !envflag.On(envflag.CacheQueryEnabled) {
			disabled = envflag.CacheQueryEnabled + "=0 turns the query cache off for this process."
		}
	case readErr == nil && !c.QueryCacheEnabled():
		disabled = repo.ConfigDirName + "/" + repoconfig.FileName + " turns the query cache off (cache.query.enabled: false)."
	}
	return disabled, defaultDuration
}
