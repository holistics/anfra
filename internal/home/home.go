// Package home is anfra's own folder on this machine: everything anfra keeps
// between runs, in one place to find, mount or delete.
//
//	<home>/
//	├── bin/anfra              the installer's binary (install.sh)
//	├── sidecars/<name>-<hash> the embedded sidecars, unpacked to run (internal/sidecar)
//	├── update-check.json      the last update check (internal/update)
//	└── repos/<repo id>/       each repo's state: logs, caches, the running server (internal/repo)
//
// It is ~/.anfra, or ANFRA_HOME. A repo's own config is not here: it is the
// repo's .anfra/ folder, committed with it.
package home

import (
	"os"
	"path/filepath"
)

// Dir is anfra's folder: ANFRA_HOME, else ~/.anfra (in the temp folder when
// there is no home folder).
func Dir() string {
	if dir := os.Getenv("ANFRA_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".anfra")
}

// RemoveLegacy removes what anfra kept in the user's cache folder before it
// kept everything in Dir: the unpacked sidecars and the last update check,
// which are made again on demand. Best-effort, and cheap once it is gone.
func RemoveLegacy() {
	if cache, err := os.UserCacheDir(); err == nil {
		_ = os.RemoveAll(filepath.Join(cache, "anfra"))
	}
}
