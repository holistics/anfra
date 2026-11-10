package syncstate_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/publish/syncstate"
	"github.com/holistics/anfra/internal/repo"
)

const org = "https://cloud.example.com/o/acme"

// newRepo is a repo folder, with ANFRA_HOME a folder of the test's own.
func newRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("ANFRA_HOME", t.TempDir())
	return t.TempDir()
}

func TestFreshFolderHasNoState(t *testing.T) {
	dir := newRepo(t)
	s, err := syncstate.For(dir).Load()
	if err != nil || len(s) != 0 {
		t.Fatalf("%v, %v", s, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".anfra")); !os.IsNotExist(err) {
		t.Error("loading wrote something")
	}
}

// Saving writes the state, in a folder that ignores itself, and loads back.
func TestSaveAndLoad(t *testing.T) {
	dir := newRepo(t)
	f := syncstate.For(dir)
	want := syncstate.State{org: {
		Base: "acme@1",
		Pending: &syncstate.Pending{
			Revision: "acme@2",
			Stage:    syncstate.Conflicted,
			Conflicts: []syncstate.Conflict{{
				Kind: "content", MarkerSize: 7,
				Files: []syncstate.File{{Path: "datasets/sales.aml", Base: "b", Local: "l", Org: "o"}},
			}},
			Added: []string{"models/refunds.aml"},
		},
	}}
	if err := f.Save(want); err != nil {
		t.Fatal(err)
	}
	ignore, err := os.ReadFile(filepath.Join(dir, ".anfra", "publish", ".gitignore"))
	if err != nil || !strings.Contains(string(ignore), "\n*\n") {
		t.Errorf(".gitignore: %q, %v", ignore, err)
	}
	got, err := f.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := got[org].Pending
	if got[org].Base != "acme@1" || p == nil || p.Stage != syncstate.Conflicted || len(p.Conflicts) != 1 ||
		p.Conflicts[0].Files[0].Local != "l" || p.Added[0] != "models/refunds.aml" {
		t.Errorf("loaded %+v, pending %+v", got[org], p)
	}
}

// The first time, the revisions ANFRA_HOME's published.json recorded become
// the bases, and saving moves them in.
func TestMovesPublishedJSON(t *testing.T) {
	dir := newRepo(t)
	abs, _ := filepath.Abs(dir)
	legacy := filepath.Join(repo.Resolve(abs).DataDir, "published.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{"`+org+`":"acme@7"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f := syncstate.For(dir)
	s, err := f.Load()
	if err != nil || s[org] == nil || s[org].Base != "acme@7" {
		t.Fatalf("%+v, %v", s, err)
	}
	if err := f.Save(s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("published.json is still there")
	}
	if s, _ := f.Load(); s[org].Base != "acme@7" {
		t.Errorf("after moving: %+v", s[org])
	}
}

// A state file that can't be read fails, saying how to start over.
func TestUnreadableState(t *testing.T) {
	dir := newRepo(t)
	path := filepath.Join(dir, ".anfra", "publish", "state.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte("{not json"), 0o600)
	if _, err := syncstate.For(dir).Load(); err == nil || !strings.Contains(err.Error(), "Delete it") {
		t.Errorf("%v", err)
	}
}

// A second lock in the same folder fails at once, and works once released.
func TestLock(t *testing.T) {
	f := syncstate.For(newRepo(t))
	release, err := f.Lock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Lock(); err == nil || !strings.Contains(err.Error(), "another anfra pull or publish is running") {
		t.Errorf("second lock: %v", err)
	}
	release()
	again, err := f.Lock()
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}

// A folder that can't be written still loads, and saving fails, for a caller
// to treat as best-effort.
func TestReadOnlyFolder(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions don't stop writes here")
	}
	dir := newRepo(t)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	f := syncstate.For(dir)
	if _, err := f.Load(); err != nil {
		t.Errorf("load: %v", err)
	}
	if err := f.Save(syncstate.State{org: {Base: "acme@1"}}); err == nil {
		t.Error("saved into a read-only folder")
	}
}
