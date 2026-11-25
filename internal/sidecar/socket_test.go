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
	checkPrivate(t, filepath.Dir(a))

	RemoveSocket(a)
	if _, err := os.Stat(filepath.Dir(a)); !os.IsNotExist(err) {
		t.Errorf("dir left after RemoveSocket: %v", err)
	}
}
