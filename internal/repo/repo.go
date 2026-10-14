// Package repo is a command's AML repo: where its files are read from, and the
// identity anfra-node keys its compile cache by.
//
// A repo is one of two kinds, and the types keep them apart:
//
//   - Local: a directory on this machine, the CLI's. It has a directory, a config
//     dir beside its files, and state under the anfra home (logs, caches, the
//     catalog). Resolve builds one.
//   - Managed: a repo its platform manages (a server, through its git service),
//     which the engine reads at one revision, "<repo>@<sha>". It has no directory
//     at all, so no config dir and no local state. Managed builds one.
//
// A command receives a Repo, which holds either kind and exposes no directory: one
// that needs a directory asks for the Local with Repo.Local, which a managed repo
// does not have, so the managed case is handled where the directory is used,
// never discovered as an empty path. Code that only ever has a local repo (the
// CLI) holds a Local, and needs no such check.
package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/holistics/anfra/internal/home"
)

// ConfigDirName is a repo's config folder, committed with it: part of a repo's
// format, as .git is, so the same for everyone who clones it.
const ConfigDirName = ".anfra"

// Local is a repo in a directory on this machine: its directory, its identity,
// and where its state lives.
type Local struct {
	Dir       string // the AML repo directory, as given
	ID        string // stable: <basename>-<sha8(realpath(dir))>
	DataDir   string // its state on this machine: <anfra home>/repos/<id> (internal/home)
	ConfigDir string // its config (data_sources.yml, ...): <repo>/.anfra
}

func (p Local) LogsDir() string    { return filepath.Join(p.DataDir, "logs") }
func (p Local) CacheDir() string   { return filepath.Join(p.DataDir, "cache") }
func (p Local) RuntimeDir() string { return filepath.Join(p.DataDir, "runtime") }
func (p Local) CatalogDir() string { return filepath.Join(p.DataDir, "catalog") }
func (p Local) CatalogPath() string {
	return filepath.Join(p.CatalogDir(), "catalog.duckdb")
}

// Repo is this local repo, as a command receives it.
func (p Local) Repo() Repo { return Repo{id: p.ID, local: &p} }

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

// Resolve computes a local repo's identity and data-dir layout. It creates no
// directories; callers create the subdirs they use.
func Resolve(repoDir string) Local {
	id := ID(repoDir)
	return Local{
		Dir:       repoDir,
		ID:        id,
		DataDir:   filepath.Join(home.Dir(), "repos", id),
		ConfigDir: filepath.Join(repoDir, ConfigDirName),
	}
}

// Repo is a command's repo: a Local, or a managed repo. Its fields are
// unexported, so it is only ever built by Local.Repo or Managed, and holds
// exactly one kind. The zero Repo holds neither and is refused where a repo is
// needed.
type Repo struct {
	id       string
	local    *Local // set for a local repo
	revision string // set for a managed repo: the revision it is read at, "<repo>@<sha>"
}

// revisionPattern is "<repo>@<sha>": a repo name, then a commit's full hex sha
// (SHA-1 or SHA-256), never an abbreviation or a ref.
var revisionPattern = regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}([0-9a-f]{24})?$`)

// Managed is a repo its platform manages, read at one revision: id is its stable
// identity, which anfra-node keys its incremental compile cache by, and must not
// change from one revision to the next; revision, "<repo>@<sha>", is what to read. Keying the
// cache by revision would make every commit a cold compile; correctness comes
// from the content anfra-node reads, never the key.
func Managed(id, revision string) (Repo, error) {
	if id == "" {
		return Repo{}, fmt.Errorf("a managed repo needs its stable id")
	}
	if !revisionPattern.MatchString(revision) {
		return Repo{}, fmt.Errorf("revision %q must be <repo>@<sha>, with the commit's full hex sha", revision)
	}
	return Repo{id: id, revision: revision}, nil
}

// ID is the repo's stable identity: anfra-node's compile-cache key, and how logs
// name it. Empty for the zero Repo.
func (r Repo) ID() string { return r.id }

// Local is the repo's directory form, for code that needs a directory; false for
// a managed repo, which has none.
func (r Repo) Local() (Local, bool) {
	if r.local == nil {
		return Local{}, false
	}
	return *r.local, true
}

// Revision is the revision a managed repo is read at, "<repo>@<sha>"; false for a
// local repo.
func (r Repo) Revision() (string, bool) { return r.revision, r.revision != "" }

// IsZero reports the zero Repo: neither kind.
func (r Repo) IsZero() bool { return r.local == nil && r.revision == "" }

// String names the repo for messages: its directory, or its revision.
func (r Repo) String() string {
	switch {
	case r.local != nil:
		return r.local.Dir
	case r.revision != "":
		return r.revision
	}
	return "(no repo)"
}
