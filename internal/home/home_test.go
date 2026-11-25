package home

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDir(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)        // os.UserHomeDir on Unix
	t.Setenv("USERPROFILE", userHome) // and on Windows

	t.Setenv("ANFRA_HOME", "")
	if got, want := Dir(), filepath.Join(userHome, ".anfra"); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}

	t.Setenv("ANFRA_HOME", "/srv/anfra")
	if got := Dir(); got != "/srv/anfra" {
		t.Errorf("Dir() with ANFRA_HOME = %q", got)
	}
}

// What anfra kept in the user's cache folder goes; the rest of that folder stays.
func TestRemoveLegacy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no legacy folder, and ignores XDG_CACHE_HOME: TestRemoveLegacyLeavesWindowsCacheAlone")
	}
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	legacy := filepath.Join(cache, "anfra", "runtime")
	other := filepath.Join(cache, "other-tool")
	for _, dir := range []string{legacy, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	RemoveLegacy()
	if _, err := os.Stat(filepath.Join(cache, "anfra")); !os.IsNotExist(err) {
		t.Errorf("the legacy folder is still there: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("another tool's folder went: %v", err)
	}
	RemoveLegacy() // nothing left: no error, no panic
}
