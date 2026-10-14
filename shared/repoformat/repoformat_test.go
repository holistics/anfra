package repoformat_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/repoformat"
)

// BlobID is git's own name for the content, as hash-object computes it with no
// filters.
func TestBlobIDIsGits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	for _, content := range []string{"", "Model orders {}\n", "a\r\nb\r\n", strings.Repeat("x", 100000)} {
		cmd := exec.Command("git", "hash-object", "--no-filters", "--stdin")
		cmd.Stdin = strings.NewReader(content)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		if got, want := repoformat.BlobID([]byte(content)), strings.TrimSpace(string(out)); got != want {
			t.Errorf("BlobID(%d bytes) = %s, git says %s", len(content), got, want)
		}
	}
}

// A publish carries the repo's AML and Data Apps, and nothing else.
func TestPublishable(t *testing.T) {
	for path, want := range map[string]bool{
		"models/orders.aml":                 true,
		"datasets/sales.AML":                true,
		"orders.aml":                        true,
		"apps/dash/index.html":              true,
		"apps/dash/data/rows.json":          true,
		"apps":                              false, // a file named apps
		".anfra/data_sources.yml":           false,
		".anfra/context_sources.yml":        false,
		".git/config":                       false,
		"lib/.git":                          false,
		".hidden/x.aml":                     false,
		"models/.draft.aml":                 false,
		"apps/dash/node_modules/x/index.js": false,
		"apps/dash/.cache/x":                false,
		".env":                              false,
		"README.md":                         false,
		"scripts/build.sh":                  false,
		"models/orders.aml.bak":             false,
		"":                                  false,
		"models//a.aml":                     false,
	} {
		if got := repoformat.Publishable(path); got != want {
			t.Errorf("Publishable(%q) = %v, want %v", path, got, want)
		}
	}
}
