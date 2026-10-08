// Package repo resolves the identity and on-disk layout of a local AMQL
// repo, so multiple repos on one machine stay isolated (separate data
// dirs, logs, and later caches/runtimes).
package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/holistics/anfra/internal/home"
)

// ConfigDirName is a repo's config folder, committed with it: part of a repo's
// format, as .git is, so the same for everyone who clones it.
const ConfigDirName = ".anfra"

// Repo is the resolved identity and on-disk layout for one AMQL repo.
type Repo struct {
	Dir       string // the AML repo directory, as given
	ID        string // stable: <basename>-<sha8(realpath(dir))>
	DataDir   string // its state on this machine: <anfra home>/repos/<id> (internal/home)
	ConfigDir string // its config (data_sources.yml, ...): <repo>/.anfra
}

func (p Repo) LogsDir() string    { return filepath.Join(p.DataDir, "logs") }
func (p Repo) CacheDir() string   { return filepath.Join(p.DataDir, "cache") }
func (p Repo) RuntimeDir() string { return filepath.Join(p.DataDir, "runtime") }
func (p Repo) CatalogDir() string { return filepath.Join(p.DataDir, "catalog") }
func (p Repo) CatalogPath() string {
	return filepath.Join(p.CatalogDir(), "catalog.duckdb")
}

// ID is the stable per-repo identifier: <basename>-<sha8(realpath)>. Stable
// across restarts and disambiguates same-named directories in different paths.
func ID(repoDir string) string {
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		abs = repoDir
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	sum := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%s-%s", filepath.Base(abs), hex.EncodeToString(sum[:4]))
}

// Resolve computes a repo's identity and data-dir layout. It creates no
// directories; callers create the subdirs they use.
func Resolve(repoDir string) Repo {
	id := ID(repoDir)
	return Repo{
		Dir:       repoDir,
		ID:        id,
		DataDir:   filepath.Join(home.Dir(), "repos", id),
		ConfigDir: filepath.Join(repoDir, ConfigDirName),
	}
}
