package repo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFilesReportsEachPathAndKeepsGoing(t *testing.T) {
	dir := t.TempDir()
	r := Resolve(dir).Repo()
	path := filepath.Join(dir, "existing")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	files := []File{{"nested/new", "created"}, {"nested", "directory"}, {"existing", "replaced"}}
	results := r.WriteFiles(files)
	if len(results) != len(files) {
		t.Fatalf("results = %+v", results)
	}
	for i, got := range results {
		file := files[i]
		if got.Path != file.Path {
			t.Fatalf("result %d has path %q, want %q", i, got.Path, file.Path)
		}
		if i == 1 {
			if !errors.Is(got.Err, WriteFailed) {
				t.Fatalf("directory write = %v", got.Err)
			}
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, got.Path))
		if got.Err != nil || err != nil || string(content) != file.Content {
			t.Fatalf("write %q: result %v, content %q, read error %v", got.Path, got.Err, content, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("replacement attributes: %v, %v", info, err)
	}
}

func TestWriteFilesRejectsUnsafePaths(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "original")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "symlink")); err != nil {
		t.Fatal(err)
	}
	r := Resolve(dir).Repo()
	for _, path := range []string{"../escape", target, "linked/original", "linked/new/file", "symlink"} {
		if got := r.WriteFiles([]File{{path, "replacement"}})[0]; !errors.Is(got.Err, WriteFailed) {
			t.Errorf("write %q = %v", path, got.Err)
		}
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "original" {
		t.Fatalf("outside file = %q, %v", content, err)
	}
}
