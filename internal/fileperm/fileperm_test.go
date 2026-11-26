package fileperm

import (
	"os"
	"path/filepath"
	"testing"
)

// A written file holds its data, and only this user can read it.
func TestWriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data_sources.yml")
	if err := WriteFile(path, []byte("password: x")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "password: x" { //nolint:gosec // G304: the test's own file
		t.Errorf("holds %q", b)
	}
	if ok, err := IsOwnerOnly(path); !ok || err != nil {
		t.Errorf("IsOwnerOnly = %v, %v; want only this user", ok, err)
	}
}

// A file made the usual way isn't owner-only (it takes its folder's access),
// until OwnerOnly makes it so.
func TestOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G304: the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if ok, _ := IsOwnerOnly(path); ok {
		t.Fatal("a file made the usual way already counts as owner-only: the check proves nothing")
	}
	if err := OwnerOnly(f); err != nil {
		t.Fatal(err)
	}
	if ok, err := IsOwnerOnly(path); !ok || err != nil {
		t.Errorf("IsOwnerOnly = %v, %v after OwnerOnly", ok, err)
	}
}
