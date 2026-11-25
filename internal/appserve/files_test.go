package appserve

import (
	"os"
	"path/filepath"
	"testing"
)

// A Data App file's path holds no backslash, which Windows reads as a
// separator, getting past the dot-file rule, and no colon, which reaches a
// file's hidden stream on Windows.
func TestAppFileRefusesBackslashAndColon(t *testing.T) {
	repo := t.TempDir()
	for _, f := range []string{"apps/x.html", "apps/sub/.secret"} {
		path := filepath.Join(repo, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if appFile(repo, "x.html") == "" {
		t.Fatal("x.html isn't served")
	}
	for _, rel := range []string{`sub\.secret`, "x.html:hidden", `sub\..\x.html`} {
		if got := appFile(repo, rel); got != "" {
			t.Errorf("appFile(%q) = %s, want refused", rel, got)
		}
	}
}
