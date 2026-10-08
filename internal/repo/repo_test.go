package repo

import (
	"path/filepath"
	"testing"
)

func TestCatalogPathUsesPerRepoDataDir(t *testing.T) {
	home := t.TempDir()
	repoDir := filepath.Join(t.TempDir(), "semantic-repo")
	t.Setenv("HOME", home)
	t.Setenv("ANFRA_HOME", "")

	r := Resolve(repoDir)
	want := filepath.Join(home, ".anfra", "repos", ID(repoDir), "catalog", "catalog.duckdb")
	if r.CatalogPath() != want {
		t.Fatalf("CatalogPath() = %q, want %q", r.CatalogPath(), want)
	}
	if r.ConfigDir != filepath.Join(repoDir, ".anfra") {
		t.Errorf("ConfigDir = %q", r.ConfigDir)
	}
}

// ANFRA_HOME moves anfra's state, and only that: a repo's config stays in the repo.
func TestAnfraHome(t *testing.T) {
	anfraHome := t.TempDir()
	repoDir := filepath.Join(t.TempDir(), "semantic-repo")
	t.Setenv("ANFRA_HOME", anfraHome)

	r := Resolve(repoDir)
	if want := filepath.Join(anfraHome, "repos", ID(repoDir)); r.DataDir != want {
		t.Errorf("DataDir = %q, want %q", r.DataDir, want)
	}
	if r.ConfigDir != filepath.Join(repoDir, ".anfra") {
		t.Errorf("ConfigDir = %q", r.ConfigDir)
	}
}
