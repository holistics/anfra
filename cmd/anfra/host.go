package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anfra-ai/anfra/internal/logging"
	"github.com/anfra-ai/anfra/internal/project"
	"github.com/anfra-ai/anfra/internal/sidecar"
)

// hostContext carries the per-invocation project + the sidecar Config (with the
// host-aggregated log sink) so commands can spawn whichever sidecars they need.
type hostContext struct {
	ctx  context.Context
	proj project.Project
	cfg  sidecar.Config
}

// withProject resolves the project and sets up host-aggregated logging, then
// runs fn. Sidecar lifecycle is the command's choice (some need only anfra-node,
// query execution also needs canal-query).
func withProject(fn func(h hostContext) error) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}
	proj := project.Resolve(projectDir)

	lg, err := logging.Setup(proj.LogsDir(), proj.ID)
	if err != nil {
		return fmt.Errorf("set up logging: %w", err)
	}
	defer lg.Close()

	return fn(hostContext{
		ctx:  context.Background(),
		proj: proj,
		cfg: sidecar.Config{
			ProjectID:        proj.ID,
			CompileCachePath: filepath.Join(proj.CacheDir(), "compile-cache"),
			StderrWriter:     lg.StderrWriter, // sidecar stderr -> the log stream (anfra.log)
			StdoutWriter:     lg.StdoutWriter, // sidecar stdout -> discarded / host stdout
			Logger:           lg.Logger,
		},
	})
}

// withSidecar runs fn with just the anfra-node sidecar spawned (ping/hold/generate).
func withSidecar(fn func(ctx context.Context, node *sidecar.AnfraNodeClient, proj project.Project) error) error {
	return withProject(func(h hostContext) error {
		node := sidecar.NewAnfraNode(h.cfg)
		if err := node.Start(h.ctx); err != nil {
			return fmt.Errorf("start anfra-node sidecar: %w", err)
		}
		defer node.Close()
		return fn(h.ctx, node.Client(), h.proj)
	})
}
