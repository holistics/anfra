package repo

import (
	"path/filepath"
	"testing"
)

func TestCatalogPathUsesPerRepoDataDir(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(t.TempDir(), "semantic-repo")
	t.Setenv("HOME", home)

	r := Resolve(repoDir)
	want := filepath.Join(home, DefaultDirName, "repos", ID(repoDir), "catalog", "catalog.duckdb")
	if r.CatalogPath() != want {
		t.Fatalf("CatalogPath() = %q, want %q", r.CatalogPath(), want)
	}
}
