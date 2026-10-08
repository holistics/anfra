package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.opentelemetry.io/otel"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/logging"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

// commandContext is the CommandContext both surfaces run commands with — the
// one-shot CLI and serve's /call. The local user owns the repo they pointed
// anfra at, so no data restrictions apply. That is stated rather than
// defaulted: a zero dataperm.Set is refused, not treated as permissive.
//
// No Attribution: there is one user and they are reading their own logs.
func (h hostContext) commandContext(clients command.Clients) command.CommandContext {
	cc := command.CommandContext{
		Clients:   clients,
		Repo:      h.repo,
		DataPerms: dataperm.Unrestricted(),
	}
	if !updateNotifyDisabled() {
		cc.Update = knownUpdate
	}
	return cc
}

// hostContext carries the per-invocation repo + the sidecar Config (with the
// host-aggregated log sink) so commands can spawn whichever sidecars they need.
// The context is passed to fn as a parameter (not stored here) so it stays a
// properly-inherited, cancelable value.
type hostContext struct {
	repo repo.Repo
	cfg  sidecar.Config
}

// withRepo resolves the repo and sets up host-aggregated logging, then runs fn
// with the caller's (signal-cancelable) context and the host context. Sidecar
// lifecycle is the command's choice (some need only anfra-node, query execution
// also needs canal-query).
func withRepo(ctx context.Context, fn func(ctx context.Context, h hostContext) error) error {
	repoDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve repo dir: %w", err)
	}
	repo := repo.Resolve(repoDir)

	lg, err := logging.Setup(repo.LogsDir(), repo.ID)
	if err != nil {
		return fmt.Errorf("set up logging: %w", err)
	}
	defer lg.Close()
	// The exporter's failures (a collector down) go to the log, not the terminal.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		lg.Logger.Warn("telemetry", "error", err)
	}))

	return fn(ctx, hostContext{
		repo: repo,
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
