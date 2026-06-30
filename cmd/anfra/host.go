package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anfra-ai/anfra/internal/logging"
	"github.com/anfra-ai/anfra/internal/repo"
	"github.com/anfra-ai/anfra/internal/sidecar"
)

// hostContext carries the per-invocation repo + the sidecar Config (with the
// host-aggregated log sink) so commands can spawn whichever sidecars they need.
type hostContext struct {
	ctx  context.Context
	repo repo.Repo
	cfg  sidecar.Config
}

// withRepo resolves the repo and sets up host-aggregated logging, then
// runs fn. Sidecar lifecycle is the command's choice (some need only anfra-node,
// query execution also needs canal-query).
func withRepo(fn func(h hostContext) error) error {
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

	return fn(hostContext{
		ctx:  context.Background(),
		repo: repo,
		cfg: sidecar.Config{
			RepoID:           repo.ID,
			CompileCachePath: filepath.Join(repo.CacheDir(), "compile-cache"),
			StderrWriter:     lg.StderrWriter, // sidecar stderr -> the log stream (anfra.log)
			StdoutWriter:     lg.StdoutWriter, // sidecar stdout -> discarded / host stdout
			Logger:           lg.Logger,
		},
	})
}
