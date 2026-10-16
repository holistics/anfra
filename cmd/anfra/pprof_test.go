package main

import (
	"log/slog"
	"net"
	"net/http"
	"testing"
)

// Unset, nothing is served; set, an address that cannot be listened on fails
// the start, naming the variable; and the handler serves the profiles.
func TestPprof(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Setenv("ANFRA_PPROF_ADDR", "")
	stop, err := startPprof(logger)
	if err != nil {
		t.Fatalf("unset: %v", err)
	}
	stop()

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	t.Setenv("ANFRA_PPROF_ADDR", taken.Addr().String())
	if _, err := startPprof(logger); err == nil {
		t.Errorf("a taken address was accepted")
	}

	t.Setenv("ANFRA_PPROF_ADDR", "127.0.0.1:0")
	stop, err = startPprof(logger)
	if err != nil {
		t.Fatalf("a free address: %v", err)
	}
	stop()

	h := pprofHandler()
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/heap", "/debug/pprof/goroutine?debug=1", "/debug/pprof/cmdline"} {
		if w := do(h, http.MethodGet, path, ""); w.Code != http.StatusOK {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
	if w := do(h, http.MethodGet, "/api/ops", ""); w.Code != http.StatusNotFound {
		t.Errorf("/api/ops on the pprof listener: %d, want 404", w.Code)
	}
}
