package publish

import (
	"errors"
	"strings"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/publish/workdir"
)

// readFolder is the folder's selected files, hashed (workdir.Read), refusing
// any that aren't regular files by name.
func readFolder(dir string, v verb) (workdir.Tree, error) {
	tree, err := workdir.Read(dir)
	if e, ok := errors.AsType[*workdir.NotRegularError](err); ok {
		return workdir.Tree{}, clierr.Precondition("%s, because these files aren't regular files: %s. Replace each with a regular file (for a symlink, a copy of the file it points to), and %s.", v.not, strings.Join(e.Files, ", "), v.again)
	}
	return tree, err
}
