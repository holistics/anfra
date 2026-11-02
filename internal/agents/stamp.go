package agents

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// stamp is the end of each SKILL.md the skills repo's build writes: a rule, then
// the skills' version. Nothing but blank lines may follow it.
var stamp = regexp.MustCompile(`(?:^|\n)---\r?\nanfra skills (\S+)\s*$`)

// ParseStamp is the skills version a SKILL.md states, from its stamp; false
// when it has none, as a copy from before the stamp existed has none.
func ParseStamp(skill []byte) (string, bool) {
	m := stamp.FindSubmatch(skill)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

// prefix starts every anfra skill's name, so a skills folder's anfra-*
// folders are ours.
const prefix = "anfra-"

// oldMarker is a skill of the names from before the prefix: a name only anfra's
// skills used, unlike aql or write-aql, which other plugins have too. Its text
// names anfra, which tells it from another vendor's skill of the same name.
const oldMarker = "setup-repo"

// Copy is a copy of the skills in a folder anfra knows.
type Copy struct {
	Dir  string // the skills folder, as shown: ~/… or relative to the repo
	path string // the skills folder
	// Versions are its skills by the version they state: "" for those with no
	// stamp. A copy installed whole has one.
	Versions map[string][]string
	// OldNames: it has skills of the names from before the prefix, which an
	// agent loads next to the new ones until they are removed.
	OldNames bool
}

// Oldest is the copy's oldest skills version: "" when a skill has no stamp,
// which is older than any.
func (c Copy) Oldest() string {
	oldest, first := "", true
	for v := range c.Versions {
		if first || v == "" || Newer(oldest, v) {
			oldest, first = v, false
		}
	}
	return oldest
}

// Copies are the copies of the skills in the folders the known agents read,
// under the user's home and under repoDir ("" for none), in that order. A copy
// in any other folder, or in an agent's plugin cache, isn't found.
func Copies(env Env, repoDir string) []Copy {
	var out []Copy
	seen := map[string]bool{}
	add := func(dir, shown string) {
		abs, err := filepath.Abs(dir)
		if err != nil || seen[abs] {
			return
		}
		seen[abs] = true
		c := Copy{Dir: shown, path: abs, Versions: map[string][]string{}}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), prefix) {
				continue
			}
			skill, err := os.ReadFile(filepath.Join(dir, e.Name(), "SKILL.md")) //nolint:gosec // G304: a skills folder anfra knows
			if err != nil {
				continue
			}
			v, _ := ParseStamp(skill)
			c.Versions[v] = append(c.Versions[v], e.Name())
		}
		if old, err := os.ReadFile(filepath.Join(dir, oldMarker, "SKILL.md")); err == nil && bytes.Contains(old, []byte("anfra")) { //nolint:gosec // G304: as above
			c.OldNames = true
		}
		if len(c.Versions) > 0 || c.OldNames {
			out = append(out, c)
		}
	}
	for _, a := range Known {
		for _, d := range a.reads {
			if dir := env.path(d); dir != "" {
				add(dir, tilde(env.Home, dir))
			}
		}
	}
	if repoDir != "" {
		for _, a := range Known {
			for _, d := range a.repoReads {
				add(filepath.Join(repoDir, d), d)
			}
		}
	}
	return out
}

// readsCopies are the copies, of those found, in the folders the agent reads,
// in the order it lists them.
func (a Agent) readsCopies(env Env, repoDir string, copies []Copy) []Copy {
	byPath := map[string]Copy{}
	for _, c := range copies {
		byPath[c.path] = c
	}
	var paths []string
	for _, d := range a.reads {
		if dir := env.path(d); dir != "" {
			paths = append(paths, dir)
		}
	}
	if repoDir != "" {
		for _, d := range a.repoReads {
			paths = append(paths, filepath.Join(repoDir, d))
		}
	}
	var out []Copy
	seen := map[string]bool{}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if c, ok := byPath[abs]; err == nil && ok && !seen[abs] {
			seen[abs] = true
			out = append(out, c)
		}
	}
	return out
}

// tilde shows a path under the home as ~/….
func tilde(home, path string) string {
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filepath.Join("~", rel))
	}
	return path
}

// Newer reports whether version a is newer than b; false when either isn't a
// full version (1.2.3, without a prefix).
func Newer(a, b string) bool {
	return validVersion(a) && validVersion(b) && semver.Compare("v"+a, "v"+b) > 0
}

// validVersion reports whether v is a full version: semver also takes 1.2,
// which Canonical fills in, and so tells apart.
func validVersion(v string) bool { return semver.Canonical("v"+v) == "v"+v }
