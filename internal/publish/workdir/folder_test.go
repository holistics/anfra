package workdir_test

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/publish/workdir"
	"github.com/holistics/anfra/shared/repoformat"
)

func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// contents is every regular file under dir, outside .anfra/, by path.
func contents(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	fsys := os.DirFS(dir)
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, _ error) error {
		if d.IsDir() && d.Name() == ".anfra" {
			return fs.SkipDir
		}
		if d.Type().IsRegular() {
			b, _ := fs.ReadFile(fsys, p)
			out[p] = string(b)
		}
		return nil
	})
	return out
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"models/a.aml": "a", "apps/x/index.html": "<html>", "notes.md": "n"})
	tree, err := workdir.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(maps.Keys(tree.Files())); !slices.Equal(got, []string{"apps/x/index.html", "models/a.aml"}) {
		t.Errorf("files %v", got)
	}
	id := tree.Files()["models/a.aml"]
	if id != repoformat.BlobID([]byte("a")) {
		t.Errorf("id %s", id)
	}
	if b, err := tree.ReadBlob(context.Background(), id); err != nil || string(b) != "a" {
		t.Errorf("ReadBlob: %q, %v", b, err)
	}
	// A file changed since Read isn't read as if it hadn't.
	write(t, dir, map[string]string{"models/a.aml": "changed"})
	if _, err := tree.ReadBlob(context.Background(), id); err == nil || !strings.Contains(err.Error(), "changed while") {
		t.Errorf("changed file: %v", err)
	}
}

// A path on disk that isn't selected is named, not read as deleted.
func TestUnselected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"models/a.aml": "a", "apps/old.txt": "x"})
	tree, err := workdir.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	// apps/old.txt is selected (everything under apps/ is); pretend the org
	// has a path this anfra doesn't select, with a file on disk there.
	write(t, dir, map[string]string{"scripts/build.aml.bak": "b"})
	got := tree.Unselected([]string{"models/a.aml", "models/gone.aml", "scripts/build.aml.bak"})
	if !slices.Equal(got, []string{"scripts/build.aml.bak"}) {
		t.Errorf("unselected %v", got)
	}
}

func TestFilesRefusesNonRegular(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"models/a.aml": "a"})
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "models", "link.aml")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	_, err := workdir.Files(dir)
	if e, ok := errors.AsType[*workdir.NotRegularError](err); !ok || !slices.Equal(e.Files, []string{"models/link.aml (a symlink)"}) {
		t.Errorf("%v", err)
	}
}

// Apply backs up what it changes, by blob id, records it before writing, and
// writes; Restore puts it all back.
func TestApplyAndRestore(t *testing.T) {
	dir, backup := t.TempDir(), filepath.Join(t.TempDir(), "backup")
	original := map[string]string{"models/a.aml": "a", "models/b.aml": "b", "apps/x/index.html": "<html>"}
	write(t, dir, original)
	changes := []workdir.Change{
		{Path: "models/a.aml", Content: []byte("A")},
		{Path: "models/b.aml", Delete: true},
		{Path: "models/new/c.aml", Content: []byte("c")},
	}
	var before map[string]string
	err := workdir.Apply(dir, backup, changes, func(b map[string]string) error {
		before = b
		if got := contents(t, dir); !maps.Equal(got, original) {
			t.Errorf("written before begin: %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"models/a.aml": "A", "models/new/c.aml": "c", "apps/x/index.html": "<html>"}
	if got := contents(t, dir); !maps.Equal(got, want) {
		t.Errorf("after Apply %v", got)
	}
	if before["models/a.aml"] != repoformat.BlobID([]byte("a")) || before["models/new/c.aml"] != "" {
		t.Errorf("before %v", before)
	}
	// The backup is named by blob id: nothing in it looks like a repo file.
	entries, _ := os.ReadDir(backup)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".") {
			t.Errorf("backup file %q is named like a repo file", e.Name())
		}
	}
	if err := workdir.Restore(dir, backup, before); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, dir); !maps.Equal(got, original) {
		t.Errorf("after Restore %v, want %v", got, original)
	}
}

// begin failing writes nothing.
func TestApplyStopsWhenBeginFails(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"models/a.aml": "a"})
	err := workdir.Apply(dir, filepath.Join(t.TempDir(), "b"), []workdir.Change{{Path: "models/a.aml", Content: []byte("A")}},
		func(map[string]string) error { return errors.New("can't record") })
	if err == nil || contents(t, dir)["models/a.aml"] != "a" {
		t.Errorf("%v, %v", err, contents(t, dir))
	}
}

// A write cut short, here by a folder where a file should go, leaves what
// was written restorable from what begin recorded.
func TestRestoreAfterInterruptedApply(t *testing.T) {
	dir, backup := t.TempDir(), filepath.Join(t.TempDir(), "backup")
	original := map[string]string{"models/a.aml": "a"}
	write(t, dir, original)
	changes := []workdir.Change{
		{Path: "models/a.aml", Content: []byte("A")},
		{Path: "models/z.aml", Content: []byte("z")},
	}
	var before map[string]string
	err := workdir.Apply(dir, backup, changes, func(b map[string]string) error {
		before = b
		// Something goes wrong between the first write and the second.
		return os.MkdirAll(filepath.Join(dir, "models", "z.aml"), 0o755)
	})
	if err == nil {
		t.Fatal("the second write succeeded")
	}
	if err := os.Remove(filepath.Join(dir, "models", "z.aml")); err != nil {
		t.Fatal(err)
	}
	if err := workdir.Restore(dir, backup, before); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, dir); !maps.Equal(got, original) {
		t.Errorf("after Restore %v", got)
	}
}

func TestCheckPath(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"models/a.aml": "a", "apps/report": "a file"})
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	ok := []string{"models/a.aml", "models/new/b.aml", "apps/x/index.html"}
	for _, p := range ok {
		if err := workdir.CheckPath(dir, p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	refused := []string{
		"", "/etc/x.aml", "../x.aml", "models/../../x.aml", "./models/a.aml", "models//a.aml",
		`models\a.aml`, "notes.md", ".anfra/data_sources.yml", ".git/hooks/x.aml",
		"linked/x.aml",           // through a symlink
		"apps/report/index.html", // through a file
		"models",                 // a folder
	}
	for _, p := range refused {
		var unsafe *workdir.UnsafePathError
		if err := workdir.CheckPath(dir, p); !errors.As(err, &unsafe) {
			t.Errorf("%q: %v, want refused", p, err)
		}
	}
	// One unsafe path writes nothing, not even the safe ones.
	err := workdir.Apply(dir, filepath.Join(t.TempDir(), "b"), []workdir.Change{
		{Path: "models/a.aml", Content: []byte("A")},
		{Path: "linked/x.aml", Content: []byte("x")},
	}, func(map[string]string) error { return nil })
	if err == nil || contents(t, dir)["models/a.aml"] != "a" {
		t.Errorf("%v", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Error("wrote outside the folder")
	}
}
