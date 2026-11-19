// Package home is anfra's own folder on this machine: everything anfra keeps
// between runs, in one place to find, mount or delete.
//
//	<home>/
//	├── bin/anfra              the installer's binary (install.sh)
//	├── sidecars/<name>-<hash> the embedded sidecars, unpacked to run (internal/sidecar)
//	├── update-check.json      the last update check (internal/update)
//	├── console_history        anfra console's entries, kept between sessions (cmd/anfra)
//	└── repos/<repo id>/       each repo's state: logs, caches, the running server (internal/repo),
//	                           and telemetry/compose.yaml, its telemetry stack (anfra experimental telemetry init)
//
// It is ~/.anfra, or ANFRA_HOME. A repo's own config is not here: it is the
// repo's .anfra/ folder, committed with it, and each person's settings for a
// repo are its root .env (internal/repoenv).
package home

import (
	"os"
	"path/filepath"
	"strings"
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

// Shown is path p as a person reads it: ~/… when it is under the user's home
// folder.
func Shown(p string) string {
	if h, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(h, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(filepath.Join("~", rel))
		}
	}
	return p
}
