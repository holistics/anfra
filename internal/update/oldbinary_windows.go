//go:build windows

package update

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// installCommand installs anfra by hand, for when an update can't.
const installCommand = "irm https://anfra.ai/install.ps1 | iex"

// oldSavePath is where selfupdate moves the running anfra.exe aside, as Windows
// can't overwrite a running program: its default, .anfra.exe.old, unless an
// anfra from an earlier update still runs from there (an anfra serve), when
// Windows won't delete it; then a name of its own. Leftovers of earlier
// updates that nothing runs go first. target is the binary updated, "" for the
// running one.
func oldSavePath(target string) string {
	if target == "" {
		exe, err := os.Executable()
		if err != nil {
			return ""
		}
		target = exe
	}
	dir, base := filepath.Split(target)
	usual := filepath.Join(dir, "."+base+".old")
	leftovers, _ := filepath.Glob(usual + "*")
	inUse := false
	for _, l := range leftovers {
		if os.Remove(l) != nil && l == usual {
			inUse = true
		}
	}
	if !inUse {
		return ""
	}
	return fmt.Sprintf("%s-%d", usual, time.Now().UnixNano())
}
