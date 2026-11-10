// Package syncstate keeps a repo folder's sync state with its orgs: the org
// revision its files last matched (the base), and a pull that isn't finished
// (the pending record). docs/designs/publish.md#the-sync-state is its design.
//
// The state lives in the repo, in .anfra/publish/, not in ANFRA_HOME with
// anfra's other state: it stays true with the folder's files wherever they are,
// so it moves with them, and a copy carries the base its files match. The
// folder ignores itself (its own .gitignore holds "*"), so nothing in it is
// committed, whatever the repo's .gitignore says.
package syncstate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/filelock"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/jsonkit"
)

// DirName is the sync state's folder, in a repo's .anfra/.
const DirName = "publish"

const (
	stateFile  = "state.json"
	lockFile   = "lock"
	backupDir  = "backup"
	ignoreFile = ".gitignore"
	ignore     = "# anfra's sync state for this folder, never committed.\n*\n"
	// legacyFile is where anfra kept the revision last published, per org,
	// before the sync state, in the repo's folder in ANFRA_HOME.
	legacyFile = "published.json"
)

// State is a folder's sync state, per org, by the org's address.
type State map[string]*Org

// Org is a folder's sync state with one org.
type Org struct {
	// Base is the org's revision these files last matched: the one last
	// published, or pulled with no conflicts left.
	Base string `json:"base,omitempty"`
	// Pending is a pull that isn't finished; nil when there's none.
	Pending *Pending `json:"pending,omitempty"`
}

// Stage is where an unfinished pull stands.
type Stage string

const (
	// Writing: the pull is writing files. Found later, the pull was stopped
	// partway, and only undoing it gets out.
	Writing Stage = "writing"
	// Conflicted: the pull's files are written, and conflicts are left to
	// resolve.
	Conflicted Stage = "conflicted"
)

// Pending is a pull that isn't finished.
type Pending struct {
	// Revision is the org's revision the pull merged: the base once every
	// conflict is resolved.
	Revision  string     `json:"revision"`
	Stage     Stage      `json:"state"`
	Conflicts []Conflict `json:"conflicts,omitempty"`
	// Added is the files the pull created, which undoing it deletes. The
	// files it changed or deleted are in the backup.
	Added []string `json:"added,omitempty"`
}

// Conflict is one of a pull's conflicts, as treemerge reported it.
type Conflict struct {
	Kind  string `json:"kind"`
	Files []File `json:"files"`
	// MarkerSize is the size of the markers written into the file; 0 when it
	// has none, and is resolved only by taking a side.
	MarkerSize int `json:"marker_size,omitempty"`
	// Resolved is set when a side was taken. A conflict with markers is
	// resolved by its file's content, which is checked each time.
	Resolved bool `json:"resolved,omitempty"`
}

// File is one path a conflict covers, with its three versions as blob ids:
// the base's, this folder's (local) and the org's. "" is absent.
type File struct {
	Path  string `json:"path"`
	Base  string `json:"base,omitempty"`
	Local string `json:"local,omitempty"`
	Org   string `json:"org,omitempty"`
}

// Folder is a repo folder's sync state, on disk.
type Folder struct {
	dir    string // the repo's .anfra/publish
	legacy string // published.json, in the repo's folder in ANFRA_HOME
}

// For is the sync state of the repo at repoDir.
func For(repoDir string) Folder {
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		abs = repoDir
	}
	r := repo.Resolve(abs)
	return Folder{
		dir:    filepath.Join(r.ConfigDir, DirName),
		legacy: filepath.Join(r.DataDir, legacyFile),
	}
}

// BackupDir is where an unfinished pull keeps the files it changed, for
// undoing it.
func (f Folder) BackupDir() string { return filepath.Join(f.dir, backupDir) }

// Load reads the sync state. A folder with none has an empty one, or, the first
// time, the revisions ANFRA_HOME's published.json recorded, as each org's base.
// Save moves them in.
func (f Folder) Load() (State, error) {
	raw, err := os.ReadFile(filepath.Join(f.dir, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return f.loadLegacy(), nil
	}
	if err != nil {
		return nil, err
	}
	s := State{}
	if err := jsonkit.Unmarshal(raw, &s); err != nil {
		return nil, clierr.Precondition("can't read %s: %v. Delete it to start over: the next pull or publish works without it, perhaps with more to resolve",
			filepath.Join(f.dir, stateFile), err)
	}
	return s, nil
}

// loadLegacy is the bases published.json recorded, or none.
func (f Folder) loadLegacy() State {
	s := State{}
	raw, err := os.ReadFile(f.legacy)
	if err != nil {
		return s
	}
	var published map[string]string
	if jsonkit.Unmarshal(raw, &published) != nil {
		return s
	}
	for org, revision := range published {
		s[org] = &Org{Base: revision}
	}
	return s
}

// Save writes the sync state, replacing what's there in one step, and removes
// ANFRA_HOME's published.json, whose revisions it now holds.
func (f Folder) Save(s State) error {
	if err := f.ensure(); err != nil {
		return err
	}
	raw, err := jsonkit.Marshal(s)
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(f.dir, stateFile), raw); err != nil {
		return err
	}
	_ = os.Remove(f.legacy)
	return nil
}

// Lock takes the folder's lock, so two pulls, or a pull and a publish, in one
// folder don't interleave. It doesn't wait: another one running fails it. The
// lock goes with the process, so a crash never leaves it held.
func (f Folder) Lock() (release func(), err error) {
	if err := f.ensure(); err != nil {
		return nil, err
	}
	path := filepath.Join(f.dir, lockFile)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := filelock.TryLock(file); err != nil {
		_ = file.Close()
		if errors.Is(err, filelock.ErrHeld) {
			return nil, clierr.Precondition("another anfra pull or publish is running in this folder. Try again when it's done")
		}
		return nil, clierr.Precondition("can't lock %s: %v. The folder may be on a network drive that doesn't support file locks", path, err)
	}
	return func() {
		filelock.Unlock(file)
		_ = file.Close()
	}, nil
}

// ensure creates the folder, ignoring itself, before anything is written in it.
func (f Folder) ensure() error {
	if err := os.MkdirAll(f.dir, 0o755); err != nil { //nolint:gosec // G301: an ordinary folder of the repo
		return err
	}
	path := filepath.Join(f.dir, ignoreFile)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return writeAtomic(path, []byte(ignore))
}

// writeAtomic writes data to path through a temporary file and a rename, so a
// reader never sees half of it.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
