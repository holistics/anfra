package sidecar

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Config is shared by the sidecar managers: how to tag and forward their output,
// and where per-repo state lives.
type Config struct {
	RepoID           string    // passed to the sidecar as ANFRA_REPO_ID; tags its logs
	CompileCachePath string    // anfra-node's AML compile cache dir (per-repo); passed as ANFRA_COMPILE_CACHE_PATH
	StderrWriter     io.Writer // sidecar stderr sink (the log stream); defaults to os.Stderr
	StdoutWriter     io.Writer // sidecar stdout sink (banners/incidental); defaults to io.Discard
	Logger           *slog.Logger
}

// AnfraNode supervises a host-spawned anfra-node sidecar (the AML/AQL engine).
// It owns the process lifecycle and exposes an AnfraNodeClient pointed at it;
// callers do their RPC through Client() (which also works against external
// sidecars, so consumers don't depend on host-spawning).
type AnfraNode struct {
	cfg        Config
	socketPath string
	proc       *process
	client     *AnfraNodeClient
}

func NewAnfraNode(cfg Config) *AnfraNode {
	return &AnfraNode{cfg: cfg}
}

// Start spawns the sidecar and waits until its client reports healthy.
func (a *AnfraNode) Start(ctx context.Context) error {
	binPath, err := resolveAnfraNodeBinary()
	if err != nil {
		return fmt.Errorf("resolve anfra-node binary: %w", err)
	}

	// UDS paths are capped at ~108 bytes (sun_path), so keep it short and in the
	// temp dir; per-pid so multiple repos don't collide.
	a.socketPath = filepath.Join(os.TempDir(), fmt.Sprintf("anfra-%d.sock", os.Getpid()))
	_ = os.Remove(a.socketPath)

	env := []string{"ANFRA_REPO_ID=" + a.cfg.RepoID}
	if a.cfg.CompileCachePath != "" {
		env = append(env, "ANFRA_COMPILE_CACHE_PATH="+a.cfg.CompileCachePath)
	}
	proc, err := startProcess(procSpec{
		Name:      "anfra-node",
		Path:      binPath,
		Args:      []string{"--socket=" + a.socketPath},
		ExtraEnv:  env,
		Stdout:    a.cfg.StdoutWriter,
		Stderr:    a.cfg.StderrWriter,
		Logger:    a.cfg.Logger,
		PipeStdin: true, // the Node sidecar uses stdin-EOF as a parent-death watchdog
	})
	if err != nil {
		return err
	}
	a.proc = proc
	a.client = NewAnfraNodeClientUnix(a.socketPath)

	if err := a.client.WaitReady(ctx); err != nil {
		a.Close()
		return err
	}
	a.proc.Logger().Info("sidecar.ready", "name", "anfra-node", "socket", a.socketPath)
	return nil
}

// Client returns the RPC client for the spawned sidecar.
func (a *AnfraNode) Client() *AnfraNodeClient { return a.client }

// Close stops the sidecar and removes its socket file.
func (a *AnfraNode) Close() {
	if a.proc != nil {
		a.proc.close()
	}
	if a.socketPath != "" {
		_ = os.Remove(a.socketPath)
	}
}
