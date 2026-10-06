package httpkit_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/holistics/anfra/shared/httpkit"
)

// Serve drains and returns nil when its context ends: the signal path.
func TestServeStopsWhenTheContextEnds(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- httpkit.Serve(ctx, srv) }()
	time.Sleep(50 * time.Millisecond) // let it listen
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v after its context ended, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop after its context ended")
	}
}

// A listener that cannot start ends Serve with that error, without waiting for
// a signal.
func TestServeReturnsAListenFailure(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	srv := &http.Server{Addr: taken.Addr().String(), Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- httpkit.Serve(context.Background(), srv) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Serve returned nil though the port was taken")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve hung on a listener that could not start")
	}
}
