// Package repo resolves the identity and on-disk layout of a local AMQL
// repo, so multiple repos on one machine stay isolated (separate data
// dirs, logs, and later caches/runtimes).
package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDirName is the anfra namespace directory name, used for both the global
// state dir (~/<name>) and the per-repo config dir (<repo>/<name>).
// Override with ANFRA_DIR_NAME (a bare name, e.g. ".anfra").
const DefaultDirName = ".anfra"

func dirName() string {
	if n := os.Getenv("ANFRA_DIR_NAME"); n != "" {
		return n
	}
	return DefaultDirName
}

// Repo is the resolved identity and on-disk layout for one AMQL repo.
type Repo struct {
	Dir       string // the AML repo directory, as given
	ID        string // stable: <basename>-<sha8(realpath(dir))>
	DataDir   string // global state: ~/<dirName>/repos/<id>
	ConfigDir string // repo config (data_sources.yml, ...): <repo>/<dirName>
}

func (p Repo) LogsDir() string    { return filepath.Join(p.DataDir, "logs") }
func (p Repo) CacheDir() string   { return filepath.Join(p.DataDir, "cache") }
func (p Repo) RuntimeDir() string { return filepath.Join(p.DataDir, "runtime") }

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
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	id := ID(repoDir)
	dir := dirName()
	return Repo{
		Dir:       repoDir,
		ID:        id,
		DataDir:   filepath.Join(home, dir, "repos", id),
		ConfigDir: filepath.Join(repoDir, dir),
	}
}
