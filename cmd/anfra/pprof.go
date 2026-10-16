package main

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"
)

// startPprof serves Go's profiling endpoints (/debug/pprof/) for anfra itself
// at ANFRA_PPROF_ADDR, e.g. localhost:6060, on a listener of their own: never
// on the server's, since they show memory and stacks to anyone who reaches
// them. Unset, it serves nothing. An address it cannot listen on fails the
// server's start rather than leaving it unprofiled without a word. stop closes
// it.
//
// canal-query has its own (ANFRA_CANAL_QUERY_ENV_PPROF_ADDR), at another address.
func startPprof(logger *slog.Logger) (stop func(), err error) {
	addr := strings.TrimSpace(os.Getenv("ANFRA_PPROF_ADDR"))
	if addr == "" {
		return func() {}, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ANFRA_PPROF_ADDR: listen on %s: %w", addr, err)
	}
	if host, _, _ := net.SplitHostPort(ln.Addr().String()); !isLoopback(host) {
		logger.Warn("pprof.exposed", "addr", ln.Addr().String())
		fmt.Fprintf(os.Stderr, "warning: pprof at %s is not on a loopback address; anyone who reaches it can read anfra's memory\n", ln.Addr())
	}
	srv := &http.Server{Handler: pprofHandler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	logger.Info("pprof.listening", "addr", ln.Addr().String())
	return func() { _ = srv.Close() }, nil
}

// pprofHandler is net/http/pprof's endpoints on a mux of their own. The
// package also registers them on http.DefaultServeMux, which anfra never
// serves.
func pprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index) // and every named profile below it
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}
