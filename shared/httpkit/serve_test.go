package httpkit_test

import (
	"context"
	"errors"
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
	go func() { done <- httpkit.Serve(ctx, srv, time.Second) }()
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
	go func() { done <- httpkit.Serve(context.Background(), srv, time.Second) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Serve returned nil though the port was taken")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve hung on a listener that could not start")
	}
}

// A request still running when drain ends is cut, and Serve says so: it returns
// within drain, not when the request would have finished.
func TestServeDrainsWithinItsBound(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	srv := &http.Server{Addr: addr, ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- httpkit.Serve(ctx, srv, 100*time.Millisecond) }()
	go func() {
		for range 50 {
			if resp, err := http.Get("http://" + addr); err == nil { //nolint:noctx // a test's own server
				_ = resp.Body.Close()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never arrived")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Serve returned %v, want the drain's deadline", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve outlasted its drain")
	}
}
