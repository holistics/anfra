//go:build !windows

package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

// On Linux and macOS a sidecar unpacks to sidecars/<name>-<hash>, a file, and
// unpacking a new version removes the old one.
func TestExtractRuntimeLayout(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	old, err := extractRuntime("anfra-test", []byte("v1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(os.Getenv("ANFRA_HOME"), "sidecars", "anfra-test-3bfc269594ef6492"); old != want {
		t.Errorf("unpacked to %s, want %s", old, want)
	}
	if _, err := extractRuntime("anfra-test", []byte("v2"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("the old version %s is still there", old)
	}
}
