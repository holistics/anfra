//go:build !windows

package cliauth

import (
	"os"
	"testing"
)

// assertPrivateFile fails t unless only this user can read path: mode 600.
func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path) //nolint:gosec // G703: the test's own credential file
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%s: mode %v, %v; want 600", path, info, err)
	}
}
