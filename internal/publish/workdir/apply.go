package workdir

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/holistics/anfra/shared/repoformat"
)

// Change is a path's content after a merge: Content written, or the path
// deleted.
type Change struct {
	Path    string
	Content []byte
	Delete  bool
}

// UnsafePathError is a path a merge would write that anfra refuses: one that
// isn't a repo file publish would send, or that would reach outside the
// folder, through a symlink or otherwise. Nothing is written when there's one.
type UnsafePathError struct {
	Path   string
	Reason string
}

func (e *UnsafePathError) Error() string {
	return fmt.Sprintf("refused to write %q: %s", e.Path, e.Reason)
}

// Apply writes changes into the folder at dir, as safely as it can:
//
//  1. Every path is checked first (CheckPath). One unsafe path writes nothing.
//  2. The current version of each path that will change is copied into
//     backupDir, named by its blob id, never by its path: a backup named
//     models/orders.aml would be read as part of the repo by anything that
//     scans for .aml files.
//  3. begin is called with each changed path's blob id before the merge, ""
//     for one that didn't exist, for the caller to record before anything is
//     written: a write cut short can then be undone (Restore).
//  4. Each file is written through a temporary file and a rename, keeping an
//     existing file's mode; deleted paths are removed.
//
// Every file operation goes through an os.Root opened at dir or backupDir,
// so none can leave them, by ".." or a symlink, even one made after the check.
func Apply(dir, backupDir string, changes []Change, begin func(before map[string]string) error) error {
	repo, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer repo.Close()
	for _, c := range changes {
		if err := checkPath(repo, c.Path); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil { //nolint:gosec // G301: inside the repo's .anfra/publish
		return err
	}
	backups, err := os.OpenRoot(backupDir)
	if err != nil {
		return err
	}
	defer backups.Close()
	before := make(map[string]string, len(changes))
	for _, c := range changes {
		id, err := backup(repo, backups, c.Path)
		if err != nil {
			return err
		}
		before[c.Path] = id
	}
	if err := begin(before); err != nil {
		return err
	}
	for _, c := range changes {
		name := filepath.FromSlash(c.Path)
		if c.Delete {
			if err := repo.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			continue
		}
		if err := writeFile(repo, name, c.Content); err != nil {
			return err
		}
	}
	return nil
}

// backup copies a path's current content into backups, by blob id, and
// answers the id; "" when the path doesn't exist.
func backup(repo, backups *os.Root, p string) (string, error) {
	content, err := repo.ReadFile(filepath.FromSlash(p))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	id := repoformat.BlobID(content)
	if _, err := backups.Stat(id); err == nil {
		return id, nil // the same content, already kept
	}
	return id, writeFile(backups, id, content)
}

// Restore puts each path in before back as it was: its content from the
// backup, or deleted when it didn't exist (""). It undoes an Apply, whole or
// cut short, and takes a single path back for `--take local`.
func Restore(dir, backupDir string, before map[string]string) error {
	repo, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer repo.Close()
	backups, err := os.OpenRoot(backupDir)
	if err != nil {
		return fmt.Errorf("the pull's backup is missing (%s), so nothing can be put back: %w", backupDir, err)
	}
	defer backups.Close()
	for _, p := range slices.Sorted(maps.Keys(before)) {
		if err := checkPath(repo, p); err != nil {
			return err
		}
		name := filepath.FromSlash(p)
		id := before[p]
		if id == "" {
			if err := repo.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			continue
		}
		content, err := backups.ReadFile(id)
		if err != nil {
			return fmt.Errorf("the backup of %s is missing (%s), so it can't be put back: %w", p, id, err)
		}
		if err := writeFile(repo, name, content); err != nil {
			return err
		}
	}
	return nil
}

// CheckPath refuses a path anfra won't write in the folder at dir: one that
// isn't a clean, relative, slash-separated path publish would send, or that
// leads through a symlink, or onto something other than a regular file.
func CheckPath(dir, p string) error {
	repo, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer repo.Close()
	return checkPath(repo, p)
}

func checkPath(repo *os.Root, p string) error {
	switch {
	case p == "" || strings.HasPrefix(p, "/") || filepath.IsAbs(p) || filepath.VolumeName(p) != "":
		return &UnsafePathError{p, "it isn't a path relative to the repo"}
	case strings.Contains(p, "\\"):
		return &UnsafePathError{p, "it holds a backslash"}
	case path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../"):
		return &UnsafePathError{p, "it isn't a plain path inside the repo"}
	case !repoformat.Publishable(p):
		return &UnsafePathError{p, "it isn't a file publish sends"}
	}
	parts := strings.Split(p, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		info, err := repo.Lstat(filepath.FromSlash(prefix))
		if errors.Is(err, fs.ErrNotExist) {
			return nil // nothing further down exists either
		}
		if err != nil {
			return err
		}
		last := i == len(parts)-1
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			return &UnsafePathError{p, prefix + " is a symlink"}
		case !last && !info.IsDir():
			return &UnsafePathError{p, prefix + " is a file, not a folder"}
		case last && !info.Mode().IsRegular():
			return &UnsafePathError{p, "it isn't a regular file here"}
		}
	}
	return nil
}

// writeFile writes content to name in root through a temporary file and a
// rename, so a reader never sees half a file, keeping an existing file's mode.
// The temporary file is a dotfile beside it, which nothing publishes.
func writeFile(root *os.Root, name string, content []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := root.Stat(name); err == nil {
		mode = info.Mode().Perm()
	}
	if dir := filepath.Dir(name); dir != "." {
		if err := root.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := filepath.Join(filepath.Dir(name), "."+filepath.Base(name)+"."+strconv.FormatInt(rand.Int64(), 36)) //nolint:gosec // G404: a unique name, not a secret
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(tmp) }()
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Rename(tmp, name)
}
