// Package canalquery is the canal-query sidecar, the SQL execution engine: its
// supervisor, which spawns one or connects to an external one, and its client.
package canalquery

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/holistics/anfra/internal/sidecar"
)

// Name is the sidecar's name, in its logs, its errors and its binary's.
const Name = "canal-query"

// Sidecar supervises a host-spawned canal-query and exposes a Client pointed at
// it. Consumers talk to the client (which also works against an external
// canal-query).
type Sidecar struct {
	cfg    sidecar.Config
	proc   *sidecar.Process
	client *Client
	port   int
}

// New is canal-query's supervisor, not yet started.
func New(cfg sidecar.Config) *Sidecar {
	return &Sidecar{cfg: cfg}
}

// Start connects to canal-query and waits until it's healthy: to an external one
// when Config.CanalQueryURL is set, otherwise to one it spawns on a free
// loopback port.
func (c *Sidecar) Start(ctx context.Context) error {
	// Before binary resolution: a build with no embedded sidecar must still be
	// able to use a remote one.
	if c.cfg.CanalQueryURL != "" {
		c.client = NewClient(c.cfg.CanalQueryURL, c.cfg.EnablePooling)
		if err := c.client.WaitReady(ctx); err != nil {
			return fmt.Errorf("canal-query at %s not ready: %w", c.cfg.CanalQueryURL, err)
		}
		c.cfg.Wrap(Name, c.client.http)
		c.cfg.Log().Info("sidecar.ready", "name", Name, "url", c.cfg.CanalQueryURL, "owned", false)
		return nil
	}

	binPath, err := sidecar.Binary(Name, "ANFRA_CANAL_QUERY_BIN", embedded)
	if err != nil {
		return fmt.Errorf("resolve canal-query binary: %w", err)
	}
	port, err := freePort()
	if err != nil {
		return fmt.Errorf("pick canal-query port: %w", err)
	}
	c.port = port

	proc, err := sidecar.StartProcess(sidecar.ProcSpec{
		Name: Name,
		Path: binPath,
		ExtraEnv: []string{
			"PORT=" + strconv.Itoa(port),
			"SKIP_HOLISTICS_DB=1",       // standalone: no monolith DB / job ops
			"ENABLE_DUCKDB_CONNECTOR=1", // duckdb connector on by default under anfra
			"ANFRA_REPO_ID=" + c.cfg.RepoID,
		},
		Stdout:    c.cfg.StdoutWriter,
		Stderr:    c.cfg.StderrWriter,
		Logger:    c.cfg.Logger,
		PipeStdin: false, // canal-query relies on Pdeathsig/process-group, not a stdin watchdog
	})
	if err != nil {
		return err
	}
	c.proc = proc
	c.client = NewClient(fmt.Sprintf("http://127.0.0.1:%d", port), c.cfg.EnablePooling)

	if err := c.client.WaitReady(ctx); err != nil {
		c.Close()
		return err
	}
	c.cfg.Wrap(Name, c.client.http)
	c.proc.Logger().Info("sidecar.ready", "name", Name, "port", port)
	return nil
}

// Client returns the query client for the spawned canal-query.
func (c *Sidecar) Client() *Client { return c.client }

// Close stops canal-query. A no-op for an external one, which this process
// never owned.
func (c *Sidecar) Close() {
	if c.proc != nil {
		c.proc.Close()
	}
}

// freePort asks the OS for an unused loopback TCP port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
