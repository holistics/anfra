//go:build !windows

package sidecar

import (
	"os"
	"testing"
)

// checkPrivate fails t unless only this user can open dir.
func checkPrivate(t *testing.T, dir string) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir mode = %o, want 700", perm)
	}
}
