// Package canalquery is the canal-query sidecar, the SQL execution engine: its
// supervisor, which spawns one or connects to an external one, and its client.
package canalquery

import (
	"context"
	"fmt"

	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/shared/logkit"
)

// Name is the sidecar's name, in its logs, its errors and its binary's.
const Name = "canal-query"

// EnvPrefix sets canal-query's own settings from anfra's environment, for the
// one anfra spawns: ANFRA_CANAL_QUERY_ENV_PPROF_ADDR=localhost:6061 is its
// PPROF_ADDR (sidecar.ProcSpec.EnvPrefix).
const EnvPrefix = "ANFRA_CANAL_QUERY_ENV_"

// Sidecar supervises a host-spawned canal-query and exposes a Client pointed at
// it. Consumers talk to the client (which also works against an external
// canal-query).
type Sidecar struct {
	cfg        sidecar.Config
	socketPath string
	proc       *sidecar.Process
	client     *Client
}

// New is canal-query's supervisor, not yet started.
func New(cfg sidecar.Config) *Sidecar {
	return &Sidecar{cfg: cfg}
}

// Start connects to canal-query and waits until it's healthy: to an external one
// when Config.CanalQueryURL is set, otherwise to one it spawns on a Unix
// socket.
func (c *Sidecar) Start(ctx context.Context) error {
	// Before binary resolution: a build with no embedded sidecar must still be
	// able to use a remote one.
	if c.cfg.CanalQueryURL != "" {
		c.client = NewClient(c.cfg.CanalQueryURL, c.cfg.EnablePooling)
		if err := c.client.WaitReady(ctx); err != nil {
			return err
		}
		c.cfg.Wrap(Name, c.client.http)
		c.cfg.Log().InfoContext(ctx, "Sidecar ready", logkit.EventKey, sidecar.EventReady, "name", Name, "url", c.cfg.CanalQueryURL, "owned", false)
		return nil
	}

	binPath, err := sidecar.Binary(Name, "ANFRA_CANAL_QUERY_BIN", embedded)
	if err != nil {
		return fmt.Errorf("resolve canal-query binary: %w", err)
	}
	c.socketPath, err = sidecar.SocketPath(Name)
	if err != nil {
		return err
	}

	proc, err := sidecar.StartProcess(ctx, sidecar.ProcSpec{
		Name: Name,
		Path: binPath,
		ExtraEnv: []string{
			"SOCKET=" + c.socketPath,
			"SKIP_HOLISTICS_DB=1",       // standalone: no monolith DB / job ops
			"ENABLE_DUCKDB_CONNECTOR=1", // duckdb connector on by default under anfra
			"ANFRA_REPO_ID=" + c.cfg.RepoID,
			// Its failures' causes, for anfra to log, and to show its clients
			// unless ANFRA_HIDE_ERROR_CAUSES. Never in their message.
			"EXPOSE_ERROR_CAUSES=1",
		},
		EnvPrefix: EnvPrefix,
		Stdout:    c.cfg.StdoutWriter,
		Stderr:    c.cfg.StderrWriter,
		Logger:    c.cfg.Logger,
		PipeStdin: false, // canal-query relies on Pdeathsig/process-group, not a stdin watchdog
	})
	if err != nil {
		return err
	}
	c.proc = proc
	c.client = NewClientUnix(c.socketPath, c.cfg.EnablePooling)

	if err := c.client.WaitReady(ctx); err != nil {
		c.Close(ctx)
		return err
	}
	c.cfg.Wrap(Name, c.client.http)
	c.proc.Logger().InfoContext(ctx, "Sidecar ready", logkit.EventKey, sidecar.EventReady, "name", Name, "socket", c.socketPath)
	return nil
}

// Client returns the query client for the spawned canal-query.
func (c *Sidecar) Client() *Client { return c.client }

// Close stops canal-query and removes its socket, which one killed before it
// could leaves. A no-op for an external one, which this process never owned.
//
// ctx carries the trace its log record joins; it is not for cancellation
// (sidecar.Process.Close).
func (c *Sidecar) Close(ctx context.Context) {
	if c.proc != nil {
		c.proc.Close(ctx)
	}
	sidecar.RemoveSocket(c.socketPath)
}
