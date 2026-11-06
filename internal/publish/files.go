package publish

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/shared/repoformat"
)

// selectFiles is the files a publish sends, slash-separated, relative to dir,
// sorted: the repo's own, as repoformat.Publishable says, the same on every
// machine, with or without git. A symlink, or anything but a regular file, among
// the repo's own is refused by name: never skipped, which would delete it on the
// server, nor followed out of the repo.
func selectFiles(dir string) (files []string, err error) {
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
		return nil, clierr.Precondition("Not published, because these files aren't regular files: %s. Replace each with a regular file (for a symlink, a copy of the file it points to), and publish again.", strings.Join(refused, ", "))
	}
	slices.Sort(files)
	return files, nil
}
