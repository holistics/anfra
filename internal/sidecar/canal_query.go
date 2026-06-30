package sidecar

import (
	"context"
	"fmt"
	"net"
	"strconv"
)

// CanalQuery supervises a host-spawned canal-query sidecar (the SQL execution
// engine) and exposes a CanalQueryClient pointed at it. Consumers talk to the
// client (which also works against an external canal-query).
type CanalQuery struct {
	cfg    Config
	proc   *process
	client *CanalQueryClient
	port   int
}

func NewCanalQuery(cfg Config) *CanalQuery {
	return &CanalQuery{cfg: cfg}
}

// Start spawns canal-query on a free loopback port and waits until it's healthy.
func (c *CanalQuery) Start(ctx context.Context) error {
	binPath, err := resolveCanalQueryBinary()
	if err != nil {
		return fmt.Errorf("resolve canal-query binary: %w", err)
	}
	port, err := freePort()
	if err != nil {
		return fmt.Errorf("pick canal-query port: %w", err)
	}
	c.port = port

	proc, err := startProcess(procSpec{
		Name: "canal-query",
		Path: binPath,
		ExtraEnv: []string{
			"PORT=" + strconv.Itoa(port),
			"SKIP_HOLISTICS_DB=1", // standalone: no monolith DB / job ops
			"ANFRA_PROJECT_ID=" + c.cfg.ProjectID,
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
	c.client = NewCanalQueryClient(fmt.Sprintf("http://127.0.0.1:%d", port))

	if err := c.client.WaitReady(ctx); err != nil {
		c.Close()
		return err
	}
	c.proc.Logger().Info("sidecar.ready", "name", "canal-query", "port", port)
	return nil
}

// Client returns the query client for the spawned canal-query.
func (c *CanalQuery) Client() *CanalQueryClient { return c.client }

// Close stops canal-query.
func (c *CanalQuery) Close() {
	if c.proc != nil {
		c.proc.close()
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
