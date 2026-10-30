package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSocketPath(t *testing.T) {
	a, err := SocketPath("canal-query")
	if err != nil {
		t.Fatal(err)
	}
	defer RemoveSocket(a)
	b, err := SocketPath("canal-query")
	if err != nil {
		t.Fatal(err)
	}
	defer RemoveSocket(b)

	if filepath.Base(a) != "canal-query.sock" {
		t.Errorf("socket = %q, want canal-query.sock", filepath.Base(a))
	}
	if filepath.Dir(a) == filepath.Dir(b) {
		t.Errorf("two sockets share the dir %q", filepath.Dir(a))
	}
	info, err := os.Stat(filepath.Dir(a))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir mode = %o, want 700", perm)
	}

	RemoveSocket(a)
	if _, err := os.Stat(filepath.Dir(a)); !os.IsNotExist(err) {
		t.Errorf("dir left after RemoveSocket: %v", err)
	}
}
