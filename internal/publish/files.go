package publish

import (
	"errors"
	"strings"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/publish/workdir"
)

// selectFiles is the files a publish sends (workdir.Files), refusing any that
// aren't regular files by name.
func selectFiles(dir string) ([]string, error) {
	files, err := workdir.Files(dir)
	if e, ok := errors.AsType[*workdir.NotRegularError](err); ok {
		return nil, clierr.Precondition("Not published, because these files aren't regular files: %s. Replace each with a regular file (for a symlink, a copy of the file it points to), and publish again.", strings.Join(e.Files, ", "))
	}
	return files, err
}
