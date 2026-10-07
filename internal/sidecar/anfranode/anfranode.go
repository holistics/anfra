// Package anfranode is the anfra-node sidecar, the AML/AQL engine: its
// supervisor, which spawns one or connects to an external one, and its JSON-RPC
// client.
package anfranode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/sidecar"
)

// Name is the sidecar's name, in its logs, its errors and its binary's.
const Name = "anfra-node"

// Sidecar supervises a host-spawned anfra-node. It owns the process lifecycle
// and exposes a Client pointed at it; callers do their RPC through Client()
// (which also works against external sidecars, so consumers don't depend on
// host-spawning).
type Sidecar struct {
	cfg        sidecar.Config
	socketPath string
	proc       *sidecar.Process
	client     *Client
}

// New is anfra-node's supervisor, not yet started.
func New(cfg sidecar.Config) *Sidecar {
	return &Sidecar{cfg: cfg}
}

// Start connects to the sidecar and waits until it reports healthy: to an
// external one when Config.NodeURL is set, otherwise to one it spawns.
func (a *Sidecar) Start(ctx context.Context) error {
	// Before binary resolution: a build with no embedded sidecar must still be
	// able to use a remote one.
	if a.cfg.NodeURL != "" {
		a.client = NewClientHTTP(a.cfg.NodeURL)
		if err := a.client.WaitReady(ctx); err != nil {
			return fmt.Errorf("anfra-node at %s not ready: %w", a.cfg.NodeURL, err)
		}
		a.cfg.Wrap(Name, a.client.http)
		a.cfg.Log().Info("sidecar.ready", "name", Name, "url", a.cfg.NodeURL, "owned", false)
		return nil
	}

	binPath, err := sidecar.Binary(Name, "ANFRA_NODE_BIN", embedded)
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
	proc, err := sidecar.StartProcess(sidecar.ProcSpec{
		Name:      Name,
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
	a.client = NewClientUnix(a.socketPath)

	if err := a.client.WaitReady(ctx); err != nil {
		a.Close()
		return err
	}
	a.cfg.Wrap(Name, a.client.http)
	a.proc.Logger().Info("sidecar.ready", "name", Name, "socket", a.socketPath)
	return nil
}

// Client returns the RPC client for the spawned sidecar.
func (a *Sidecar) Client() *Client { return a.client }

// Close stops the sidecar and removes its socket file. A no-op for an external
// sidecar, which this process never owned.
func (a *Sidecar) Close() {
	if a.proc != nil {
		a.proc.Close()
	}
	if a.socketPath != "" {
		_ = os.Remove(a.socketPath)
	}
}
