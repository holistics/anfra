package sidecar

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

// Config is shared by the sidecar managers: how to tag and forward their output,
// and where per-repo state lives.
type Config struct {
	// RepoID is passed to the sidecar as ANFRA_REPO_ID, which tags the sidecar's
	// logs and nothing else. It is deliberately not a cache or isolation identity:
	// a sidecar may serve many repos, so each request names its own repo (see
	// CompileToSQLRequest.RepoID) and the sidecar refuses requests that don't.
	RepoID           string
	CompileCachePath string    // anfra-node's AML compile cache dir (per-repo); passed as ANFRA_COMPILE_CACHE_PATH
	StderrWriter     io.Writer // sidecar stderr sink (the log stream); defaults to os.Stderr
	StdoutWriter     io.Writer // sidecar stdout sink (banners/incidental); defaults to io.Discard
	Logger           *slog.Logger
	// EnablePooling turns on canal-query connection pooling. Only useful when the
	// sidecar is long-lived (the pool is reused across requests under `anfra
	// serve`); ignored by anfra-node.
	EnablePooling bool

	// NodeURL / CanalQueryURL address sidecars this process does not own, as in a
	// docker-compose / k8s deployment. When set, the manager resolves no binary,
	// starts no process, and Close is a no-op. Empty means spawn one.
	NodeURL       string
	CanalQueryURL string

	// WrapTransport, if set, wraps each sidecar client's transport once the
	// sidecar is ready — e.g. with otelhttp, so a host traces the calls and
	// propagates its trace into the sidecar. It receives the client's own
	// transport, so the engine's settings (the Unix socket dialer, pooling) are
	// kept; sidecar names the sidecar ("anfra-node", "canal-query"). Readiness
	// polling is not wrapped: it would trace every poll.
	WrapTransport func(sidecar string, rt http.RoundTripper) http.RoundTripper
}

// wrap applies WrapTransport to a ready sidecar's client.
func (c Config) wrap(sidecar string, hc *http.Client) {
	if c.WrapTransport == nil {
		return
	}
	rt := hc.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	hc.Transport = c.WrapTransport(sidecar, rt)
}

// logger returns the configured logger, or the default. The spawn path gets one
// from the started process; the external path has no process to ask.
func (c Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
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

// Start connects to the sidecar and waits until it reports healthy: to an
// external one when Config.NodeURL is set, otherwise to one it spawns.
func (a *AnfraNode) Start(ctx context.Context) error {
	// Before binary resolution: a build with no embedded sidecar must still be
	// able to use a remote one.
	if a.cfg.NodeURL != "" {
		a.client = NewAnfraNodeClientHTTP(a.cfg.NodeURL)
		if err := a.client.WaitReady(ctx); err != nil {
			return fmt.Errorf("anfra-node at %s not ready: %w", a.cfg.NodeURL, err)
		}
		a.cfg.wrap("anfra-node", a.client.http)
		a.cfg.logger().Info("sidecar.ready", "name", "anfra-node", "url", a.cfg.NodeURL, "owned", false)
		return nil
	}

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
	a.cfg.wrap("anfra-node", a.client.http)
	a.proc.Logger().Info("sidecar.ready", "name", "anfra-node", "socket", a.socketPath)
	return nil
}

// Client returns the RPC client for the spawned sidecar.
func (a *AnfraNode) Client() *AnfraNodeClient { return a.client }

// Close stops the sidecar and removes its socket file. A no-op for an external
// sidecar, which this process never owned.
func (a *AnfraNode) Close() {
	if a.proc != nil {
		a.proc.close()
	}
	if a.socketPath != "" {
		_ = os.Remove(a.socketPath)
	}
}
