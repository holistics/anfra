package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
)

// serveTestEnv runs serve in a fresh repo with no sidecar binaries, so runServe
// returns at sidecar startup once it is past its socket checks.
func serveTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("ANFRA_NODE_BIN", "")
	t.Setenv("ANFRA_CANAL_QUERY_BIN", "")
}

// shortSocketPath is a socket path in a fresh dir under the system temp dir:
// t.TempDir can exceed the ~104-byte sun_path limit on macOS.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "anfra-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func TestRunServeRefusesALiveSocket(t *testing.T) {
	serveTestEnv(t)
	sock := shortSocketPath(t)
	live, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()

	err = runServe(context.Background(), sock)
	if err == nil || !strings.Contains(err.Error(), "already running on socket "+sock) {
		t.Fatalf("runServe error = %v, want already running on %s", err, sock)
	}
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("the live server's socket was removed: %v", err)
	}
}

func TestRunServeOnACustomSocketIgnoresTheDefaultOne(t *testing.T) {
	serveTestEnv(t)
	cwd, _ := os.Getwd()
	def, err := net.Listen("unix", serveSocketPath(repo.Resolve(cwd)))
	if err != nil {
		t.Fatal(err)
	}
	defer def.Close()

	err = runServe(context.Background(), shortSocketPath(t))
	if err == nil || strings.Contains(err.Error(), "already running") {
		t.Fatalf("runServe error = %v, want it past the running check (a sidecar error)", err)
	}
}

func TestRunServeOnTheDefaultSocketRefusesARunningServer(t *testing.T) {
	serveTestEnv(t)
	cwd, _ := os.Getwd()
	sock := serveSocketPath(repo.Resolve(cwd))
	def, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer def.Close()

	err = runServe(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "already running on socket "+sock) {
		t.Fatalf("runServe error = %v, want already running on %s", err, sock)
	}
}

func TestRunServeTreatsADeadSocketFileAsStale(t *testing.T) {
	serveTestEnv(t)
	sock := shortSocketPath(t)
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	err := runServe(context.Background(), sock)
	if err == nil || strings.Contains(err.Error(), "already running") {
		t.Fatalf("runServe error = %v, want it past the running check (a sidecar error)", err)
	}
}
