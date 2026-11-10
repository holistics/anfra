// Package workdir is a repo folder as publish and pull see it: the files a
// publish carries, hashed into a tree, and the safe writing of a merge into
// them, with a backup to undo it. docs/designs/publish.md is its design.
package workdir

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/holistics/anfra/shared/repoformat"
)

// NotRegularError names files of the repo that aren't regular files: a symlink
// or anything else. They're refused by name, never skipped, which would read
// as deleted, nor followed out of the repo.
type NotRegularError struct {
	// Files are the paths, each with what it is: "models/a.aml (a symlink)".
	Files []string
}

func (e *NotRegularError) Error() string {
	return "these files aren't regular files: " + strings.Join(e.Files, ", ")
}

// Files is the files a publish carries, slash-separated, relative to dir,
// sorted: the repo's own, as repoformat.Publishable says, the same on every
// machine, with or without git. A symlink, or anything but a regular file,
// among them fails with a NotRegularError.
func Files(dir string) (files []string, err error) {
	var refused []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case rel == ".":
			return nil
		case d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules"):
			return filepath.SkipDir // nothing under it is publishable: not worth walking
		case d.IsDir():
			return nil
		case !repoformat.Publishable(rel):
		case d.Type()&fs.ModeSymlink != 0:
			refused = append(refused, rel+" (a symlink)")
		case !d.Type().IsRegular():
			refused = append(refused, rel+" (not a regular file)")
		default:
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(refused) > 0 {
		return nil, &NotRegularError{Files: refused}
	}
	slices.Sort(files)
	return files, nil
}
