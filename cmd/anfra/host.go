package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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

	queryCache := localquerycache.New(filepath.Join(repo.CacheDir(), "query"), queryCacheDisabled(repo.Dir, lg.Logger))
	defer queryCache.Close()

	return fn(ctx, hostContext{
		repo:        repo,
		exportFiles: exports.New(exportLinkLife),
		appLogger:   lg.App,
		queryCache:  queryCache,
		cfg: sidecar.Config{
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
		},
	})
}

// queryCacheDisabled says what turns the repo's query cache off, if anything:
// ANFRA_CACHE_QUERY_ENABLED, whatever the repo sets, or the repo's
// cache.query.enabled. A settings file that can't be read or parsed leaves the
// cache on, its default, and is logged.
func queryCacheDisabled(repoDir string, logger *slog.Logger) string {
	if os.Getenv(envflag.CacheQueryEnabled) != "" {
		if envflag.On(envflag.CacheQueryEnabled) {
			return ""
		}
		return envflag.CacheQueryEnabled + "=0 turns the query cache off for this process."
	}
	c, err := repoconfig.Read(repoDir)
	if err != nil {
		logger.Warn("Query cache settings unknown", logkit.EventKey, eventQueryCacheSettingsUnknown, "path", repoconfig.Path(repoDir), "error", err,
			"hint", "the query cache stays on, its default, until anfra can read its settings from the file")
		return ""
	}
	if !c.QueryCacheEnabled() {
		return repo.ConfigDirName + "/" + repoconfig.FileName + " turns the query cache off (cache.query.enabled: false)."
	}
	return ""
}
