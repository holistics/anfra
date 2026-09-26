package sidecar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// anfra-node keeps no repo identity of its own, so every repo-scoped request
// carries one. The client refuses before the request leaves the process — the
// sidecar enforces it too, but failing here puts the error in the caller's stack.
func TestClientRequiresRepoID(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	c := NewAnfraNodeClientHTTP(srv.URL)

	t.Run("refused without a RepoID", func(t *testing.T) {
		_, err := c.CompileToSQL(context.Background(), CompileToSQLRequest{RepoPath: "/repos/a"})
		if err == nil {
			t.Fatal("CompileToSQL succeeded with an empty RepoID; want refusal")
		}
		if !strings.Contains(err.Error(), "RepoID is required") {
			t.Errorf("error = %v, want it to name the missing RepoID", err)
		}
		if calls != 0 {
			t.Errorf("sidecar saw %d request(s); the guard should short-circuit", calls)
		}
	})

	t.Run("allowed with a RepoID", func(t *testing.T) {
		calls = 0
		// The stub's reply is not a valid result for this RPC, so an error here is
		// expected and uninteresting — what matters is that the call got past the
		// guard and onto the wire.
		_, _ = c.CompileToSQL(context.Background(), CompileToSQLRequest{
			RepoPath: "/repos/a", RepoID: "acme/sales",
		})
		if calls != 1 {
			t.Errorf("sidecar saw %d request(s), want 1", calls)
		}
	})

	t.Run("validate RPCs are guarded too", func(t *testing.T) {
		calls = 0
		if _, err := c.ValidateAML(context.Background(), ValidateAMLRequest{RepoPath: "/repos/a"}); err == nil {
			t.Error("ValidateAML succeeded with an empty RepoID; want refusal")
		}
		if _, err := c.ValidateAQL(context.Background(), CompileToSQLRequest{RepoPath: "/repos/a"}); err == nil {
			t.Error("ValidateAQL succeeded with an empty RepoID; want refusal")
		}
		if calls != 0 {
			t.Errorf("sidecar saw %d request(s); both should short-circuit", calls)
		}
	})
}

// The rule is the same for a host-spawned sidecar: it serves one repo today, but
// the identity is the caller's to supply either way, so there is no second
// behaviour to reason about. Refused before any dial is attempted.
func TestUnixClientRequiresRepoIDToo(t *testing.T) {
	c := NewAnfraNodeClientUnix("/nonexistent/anfra-test.sock")
	_, err := c.CompileToSQL(context.Background(), CompileToSQLRequest{RepoPath: "/repos/a"})
	if err == nil {
		t.Fatal("CompileToSQL succeeded with an empty RepoID; want refusal")
	}
	if !strings.Contains(err.Error(), "RepoID is required") {
		t.Errorf("error = %v, want the RepoID guard, not a dial error", err)
	}
}

// With Config.NodeURL set the manager dials an existing sidecar: no binary is
// resolved, and Close leaves the process alone.
func TestAnfraNodeExternalDoesNotSpawn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	node := NewAnfraNode(Config{NodeURL: srv.URL})
	if err := node.Start(context.Background()); err != nil {
		t.Fatalf("Start against an external sidecar: %v", err)
	}
	if node.proc != nil {
		t.Error("a process was spawned for an external sidecar")
	}
	if node.socketPath != "" {
		t.Errorf("socketPath = %q, want empty for an external sidecar", node.socketPath)
	}
	if node.Client() == nil {
		t.Fatal("Client() is nil")
	}
	node.Close() // must not panic with no process of its own
}

func TestAnfraNodeExternalUnreachable(t *testing.T) {
	// Start must fail rather than fall back to spawning, and must give up when the
	// caller does: WaitReady has its own 10s deadline, so ignoring ctx blocks for
	// the full 10s.
	node := NewAnfraNode(Config{NodeURL: "http://127.0.0.1:1"})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := node.Start(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Start succeeded against an unreachable sidecar")
	}
	if elapsed > 3*time.Second {
		t.Errorf("Start took %v; it should stop when the context does, not run the full deadline", elapsed)
	}
	if node.proc != nil {
		t.Error("a process was spawned after the external sidecar failed")
	}
}

func TestCanalQueryExternalDoesNotSpawn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer srv.Close()

	canal := NewCanalQuery(Config{CanalQueryURL: srv.URL})
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
