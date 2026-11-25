//go:build windows

package robustio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// holdOpen opens path the way a reader does, which keeps Windows from
// renaming over it, and closes it after d (or at the test's end).
func holdOpen(t *testing.T, path string, d time.Duration) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	if d > 0 {
		time.AfterFunc(d, func() { f.Close() })
	}
}

func files(t *testing.T) (tmp, path string) {
	t.Helper()
	dir := t.TempDir()
	path, tmp = filepath.Join(dir, "state.json"), filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	return tmp, path
}

// A plain rename fails while a reader has the file open: why Rename exists.
func TestPlainRenameFailsWhileOpen(t *testing.T) {
	tmp, path := files(t)
	holdOpen(t, path, 0)
	if err := os.Rename(tmp, path); err == nil {
		t.Skip("this Windows renames over an open file; Rename's retries aren't needed here")
	}
}

// Rename succeeds once the reader closes the file, within its retries.
func TestRenameWaitsForReader(t *testing.T) {
	tmp, path := files(t)
	holdOpen(t, path, 200*time.Millisecond)
	if err := Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Errorf("%s holds %q, want the new contents", path, b)
	}
}

// When the reader never closes it, Rename gives up and says why.
func TestRenameGivesUp(t *testing.T) {
	tmp, path := files(t)
	holdOpen(t, path, 0)
	start := time.Now()
	err := Rename(tmp, path)
	if err == nil {
		t.Fatal("renamed over a file held open")
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("gave up after %s, want about a second", took)
	}
	if !strings.Contains(err.Error(), "another program") {
		t.Errorf("error %q doesn't say another program may have it open", err)
	}
}
