package repo

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
)

// File is the full content to write at a path relative to the repo.
type File struct {
	Path    string
	Content string
}

// WriteResult reports whether writing one file succeeded.
type WriteResult struct {
	Path string
	Err  error
}

// WriteFailed means the repo could not store a file.
var WriteFailed = apperr.DefinePublicCode(errcode.NS, "repo_write_failed", apperr.Server, "The file could not be written to the repo.")

// WriteFiles creates or replaces each file independently, returning its result
// in input order. Only local repos support writes.
func (r Repo) WriteFiles(files []File) []WriteResult {
	results := make([]WriteResult, len(files))
	local, ok := r.Local()
	if !ok {
		for i, file := range files {
			results[i] = WriteResult{file.Path, apperr.New(WriteFailed, fmt.Sprintf("Writing %q requires a local repo.", file.Path))}
		}
		return results
	}

	root, openErr := os.OpenRoot(local.Dir)
	if openErr == nil {
		defer root.Close()
	}
	for i, file := range files {
		err := openErr
		if err == nil {
			err = writeFile(root, file)
		}
		if err != nil {
			err = apperr.Encapsulate(err, WriteFailed, fmt.Sprintf("Could not write %q to the repo.", file.Path))
		}
		results[i] = WriteResult{file.Path, err}
	}
	return results
}

func writeFile(root *os.Root, file File) error {
	if !filepath.IsLocal(file.Path) {
		return errors.New("the file path must be relative to the repo")
	}
	info, err := root.Lstat(file.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := root.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return err
		}
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return errors.New("the destination must be a regular file")
	}

	// Stage beside the destination so rename is atomic. New files respect the
	// process umask. Replacements inherit the destination's exact permissions.
	mode := fs.FileMode(0o644)
	if info != nil {
		mode = info.Mode().Perm()
	}
	tmp := filepath.Join(filepath.Dir(file.Path), ".anfra-"+rand.Text())
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tmp) }()
	if _, err := f.WriteString(file.Content); err != nil {
		_ = f.Close()
		return err
	}
	if info != nil {
		if err := f.Chmod(mode); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Rename(tmp, file.Path)
}
