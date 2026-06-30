// Package project resolves the identity and on-disk layout of a local AMQL
// project, so multiple projects on one machine stay isolated (separate data
// dirs, logs, and later caches/runtimes).
package project

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDirName is the anfra namespace directory name, used for both the global
// state dir (~/<name>) and the per-project config dir (<project>/<name>).
// Override with ANFRA_DIR_NAME (a bare name, e.g. ".anfra").
const DefaultDirName = ".anfra"

func dirName() string {
	if n := os.Getenv("ANFRA_DIR_NAME"); n != "" {
		return n
	}
	return DefaultDirName
}

// Project is the resolved identity and on-disk layout for one AMQL project.
type Project struct {
	Dir       string // the AML project directory, as given
	ID        string // stable: <basename>-<sha8(realpath(dir))>
	DataDir   string // global state: ~/<dirName>/projects/<id>
	ConfigDir string // project config (data_sources.yml, ...): <project>/<dirName>
}

func (p Project) LogsDir() string    { return filepath.Join(p.DataDir, "logs") }
func (p Project) CacheDir() string   { return filepath.Join(p.DataDir, "cache") }
func (p Project) RuntimeDir() string { return filepath.Join(p.DataDir, "runtime") }

// ID is the stable per-project identifier: <basename>-<sha8(realpath)>. Stable
// across restarts and disambiguates same-named directories in different paths.
func ID(projectDir string) string {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		abs = projectDir
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	sum := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%s-%s", filepath.Base(abs), hex.EncodeToString(sum[:4]))
}

// Resolve computes a project's identity and data-dir layout. It creates no
// directories; callers create the subdirs they use.
func Resolve(projectDir string) Project {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	id := ID(projectDir)
	dir := dirName()
	return Project{
		Dir:       projectDir,
		ID:        id,
		DataDir:   filepath.Join(home, dir, "projects", id),
		ConfigDir: filepath.Join(projectDir, dir),
	}
}
