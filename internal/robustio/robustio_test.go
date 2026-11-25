package robustio

import (
	"os"
	"path/filepath"
	"testing"
)

// Rename replaces path with tmp's contents, as os.Rename does.
func TestRename(t *testing.T) {
	dir := t.TempDir()
	path, tmp := filepath.Join(dir, "a"), filepath.Join(dir, "a.tmp")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Errorf("%s holds %q, want new", path, b)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("%s still there", tmp)
	}
}
