package repo

import (
	"path/filepath"
	"strings"
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

const (
	testRepoID   = "0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b"
	testRevision = testRepoID + "@0123456789abcdef0123456789abcdef01234567"
)

// A Repo is exactly one kind: a local repo has a directory and no revision, a
// managed repo the reverse, and the zero Repo neither.
func TestRepoKinds(t *testing.T) {
	local := Resolve(t.TempDir())
	l := local.Repo()
	if got, ok := l.Local(); !ok || got.Dir != local.Dir || l.ID() != local.ID {
		t.Errorf("local: Local() = %+v, %v; ID = %q", got, ok, l.ID())
	}
	if _, ok := l.Revision(); ok {
		t.Error("a local repo has a revision")
	}

	r, err := Managed(testRepoID, testRevision)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Local(); ok {
		t.Error("a managed repo has a directory")
	}
	if rev, ok := r.Revision(); !ok || rev != testRevision || r.ID() != testRepoID {
		t.Errorf("revision: Revision() = %q, %v; ID = %q", rev, ok, r.ID())
	}

	var zero Repo
	if !zero.IsZero() || l.IsZero() || r.IsZero() {
		t.Error("IsZero is not exactly the zero Repo")
	}
}

// A revision names a commit by its full sha, never an abbreviation or a ref, and
// the id that keys the compile cache is required.
func TestManagedRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ id, revision string }{
		"no id":       {"", testRevision},
		"no repo":     {testRepoID, "@0123456789abcdef0123456789abcdef01234567"},
		"no sha":      {testRepoID, testRepoID},
		"abbreviated": {testRepoID, testRepoID + "@0123456"},
		"a ref":       {testRepoID, testRepoID + "@main"},
		"upper hex":   {testRepoID, testRepoID + "@0123456789ABCDEF0123456789ABCDEF01234567"},
		"a path":      {testRepoID, "/repos/acme@0123456789abcdef0123456789abcdef01234567 x"},
	} {
		if _, err := Managed(tc.id, tc.revision); err == nil {
			t.Errorf("%s: Managed(%q, %q) succeeded", name, tc.id, tc.revision)
		}
	}
	if _, err := Managed(testRepoID, testRepoID+"@"+strings.Repeat("a", 64)); err != nil {
		t.Errorf("a SHA-256 revision: %v", err)
	}
}
