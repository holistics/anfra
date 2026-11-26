package cliauth

import (
	"testing"

	"github.com/holistics/anfra/internal/fileperm"
)

// assertPrivateFile fails t unless only this user can read path.
func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	if ok, err := fileperm.IsOwnerOnly(path); !ok || err != nil {
		t.Fatalf("%s: only you can read it = %v, %v", path, ok, err)
	}
}
