package workdir

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/holistics/anfra/shared/repoformat"
)

// Tree is a folder's files as a merge sees them: each selected file's path and
// blob id, as git names its content (repoformat.BlobID).
type Tree struct {
	dir   string
	files map[string]string // path → blob id
	byID  map[string]string // blob id → a path holding it
}

// Read selects the folder's files (Files) and hashes each.
func Read(dir string) (Tree, error) {
	paths, err := Files(dir)
	if err != nil {
		return Tree{}, err
	}
	t := Tree{dir: dir, files: make(map[string]string, len(paths)), byID: make(map[string]string, len(paths))}
	for _, p := range paths {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			return Tree{}, err
		}
		id := repoformat.BlobID(content)
		t.files[p], t.byID[id] = id, p
	}
	return t, nil
}

// Files is the tree's paths and blob ids. Callers mustn't change it.
func (t Tree) Files() map[string]string { return t.files }

// ReadBlob reads a file's content by blob id, from a path holding it. A file
// changed since Read fails: the merge would otherwise work from content
// other than what it classified.
func (t Tree) ReadBlob(_ context.Context, id string) ([]byte, error) {
	p, ok := t.byID[id]
	if !ok {
		return nil, fmt.Errorf("no file in %s has blob %s", t.dir, id)
	}
	content, err := os.ReadFile(filepath.Join(t.dir, filepath.FromSlash(p)))
	if err != nil {
		return nil, err
	}
	if repoformat.BlobID(content) != id {
		return nil, fmt.Errorf("%s changed while anfra was reading the folder. Run the command again", p)
	}
	return content, nil
}

// Unselected is those of paths that exist in the folder but aren't among its
// selected files: a symlink, or a file this anfra doesn't publish, such as one
// an older anfra did. A merge would read each as deleted, and the next
// publish would delete it from the org, so callers stop and name them.
func (t Tree) Unselected(paths []string) []string {
	var out []string
	for _, p := range paths {
		if _, ok := t.files[p]; ok {
			continue
		}
		if _, err := os.Lstat(filepath.Join(t.dir, filepath.FromSlash(p))); !errors.Is(err, fs.ErrNotExist) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}
