// Package sidecar is the framework anfra's sidecars are run with: the Config a
// host gives them, the lifecycle of one it spawns (Process), how its binary is
// found (Binary), and how a sidecar that does not answer is classified
// (Unreachable). The sidecars themselves live in the packages under this one, a
// package per sidecar — its supervisor, its client and its wire types — so what
// one speaks stays its own.
package sidecar

import (
	"io"
	"log/slog"
	"net/http"
)

// Config is shared by the sidecar supervisors: how to tag and forward their
// output, and where per-repo state lives.
type Config struct {
	// RepoID is passed to the sidecar as ANFRA_REPO_ID, which tags the sidecar's
	// logs and nothing else. It is deliberately not a cache or isolation identity:
	// a sidecar may serve many repos, so each request names its own repo (see
	// anfranode.CompileToSQLRequest.RepoID) and the sidecar refuses requests that
	// don't.
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
	// docker-compose / k8s deployment. When set, the supervisor resolves no
	// binary, starts no process, and Close is a no-op. Empty means spawn one.
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

// Wrap applies WrapTransport to a ready sidecar's client.
func (c Config) Wrap(sidecar string, hc *http.Client) {
	if c.WrapTransport == nil {
		return
	}
	rt := hc.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	hc.Transport = c.WrapTransport(sidecar, rt)
}

// Log returns the configured logger, or the default. The spawn path gets one
// from the started process; the external path has no process to ask.
func (c Config) Log() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}
