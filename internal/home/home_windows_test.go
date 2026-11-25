//go:build windows

package home

import (
	"os"
	"path/filepath"
	"testing"
)

// anfra never kept files in %LOCALAPPDATA%\anfra on Windows: RemoveLegacy
// leaves it alone, whoever's it is.
func TestRemoveLegacyLeavesWindowsCacheAlone(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LocalAppData", local)
	kept := filepath.Join(local, "anfra", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	RemoveLegacy()
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("RemoveLegacy removed %s: %v", kept, err)
	}
}
