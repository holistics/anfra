package canalquery

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/sidecar"
)

// canal-query's error object is read into a typed error, whatever it says.
func TestCanalError(t *testing.T) {
	e := canalError(map[string]any{"message": "run query: boom", "scope": "User", "type": "GenericError"})
	if e.Message != "run query: boom" || e.Scope != "User" || e.Type != "GenericError" {
		t.Errorf("read %+v", e)
	}
	if e := canalError(map[string]any{"code": 7}); e.Message == "" {
		t.Error("an error object with no message reads as an empty message")
	}
}

// A canal-query that does not answer is an outage, classified.
func TestUnreachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + l.Addr().String()
	l.Close() // nothing listens there now

	canal := NewClient(url, false)
	if _, err := canal.Execute(context.Background(), "postgres", nil, "select 1", 0); !errors.Is(err, errcode.SidecarUnavailable) {
		t.Errorf("canal-query down: %v, want sidecar_unavailable", err)
	}
}

// With Config.CanalQueryURL set the supervisor dials an existing canal-query: no
// binary is resolved, and Close leaves the process alone.
func TestExternalDoesNotSpawn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	canal := New(sidecar.Config{CanalQueryURL: srv.URL})
	if err := canal.Start(context.Background()); err != nil {
		t.Fatalf("Start against an external canal-query: %v", err)
	}
	if canal.proc != nil {
		t.Error("a process was spawned for an external canal-query")
	}
	if canal.Client() == nil {
		t.Fatal("Client() is nil")
	}
	canal.Close()
}
