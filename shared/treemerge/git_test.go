package treemerge_test

import (
	"context"
	"errors"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/treemerge"
)

// TestAgainstGit merges generated trees with treemerge and with
// `git merge-tree --write-tree`, the reference, configured as treemerge
// works: Myers and diff3 markers. git can't be told to detect no renames at
// all, only exact ones (find-renames=100%), and treemerge detects none yet, so
// paths caught in an exact rename, the same content deleted at one path and
// added at another on one side, are skipped. For each other path, the result
// must be git's, or a conflict where git resolved. Conflicts both report are compared without their
// markers' labels, and differences only counted: they come from the diffs
// aligning repeated lines differently.
//
// git runs only here, never in what's shipped. Skipped without git, or with
// one older than 2.38, which has no merge-tree --write-tree.
func TestAgainstGit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	g := newGitRepo(t)
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // G404: a fixed seed, so a failing case can be replayed
	const cases = 200
	var same, cautious, placedDifferently, renamed int
	cautiousKinds := map[treemerge.Kind]int{}
	for i := range cases {
		base := genTree(rng, nil)
		ours, theirs := genTree(rng, base), genTree(rng, base)

		s := store{}
		r, err := treemerge.Merge(context.Background(), s.tree(base), s.tree(ours), s.tree(theirs), s, labels)
		if err != nil {
			t.Fatal(err)
		}
		gitFiles, gitConflicted := g.merge(t, base, ours, theirs)

		skip := renameInvolved(base, ours)
		maps.Copy(skip, renameInvolved(base, theirs))
		for _, p := range r.Paths {
			if skip[p.Path] {
				renamed++
				continue
			}
			ourContent, ourConflict := content(t, s, r, p), p.Conflict != nil
			gitContent, ok := gitFiles[p.Path]
			if !ok {
				gitContent = "<absent>"
			}
			switch {
			case !ourConflict && !gitConflicted[p.Path] && ourContent == gitContent:
				same++
			case ourConflict && !gitConflicted[p.Path]:
				cautious++
				cautiousKinds[p.Conflict.Kind]++
			case ourConflict && gitConflicted[p.Path]:
				if p.Conflict.MarkerSize > 0 && unlabelled(ourContent) != unlabelled(gitContent) &&
					(p.Conflict.Kind == treemerge.Content || p.Conflict.Kind == treemerge.AddAdd) {
					placedDifferently++
					t.Logf("case %d, %s: conflicts elsewhere than git\ngit:\n%q\ntreemerge:\n%q", i, p.Path, gitContent, ourContent)
				} else {
					same++
				}
			default:
				t.Errorf("case %d, %s: resolved differently from git (git conflicted: %v)\nbase:   %q\nours:   %q\ntheirs: %q\ngit:       %q\ntreemerge: %q",
					i, p.Path, gitConflicted[p.Path], base, ours, theirs, gitContent, ourContent)
			}
		}
	}
	t.Logf("%d cases: paths as git %d, conflicting where git resolved %d, conflicting elsewhere than git %d, skipped for an exact rename %d",
		cases, same, cautious, placedDifferently, renamed)
	t.Logf("conflicting where git resolved, by kind: %v", cautiousKinds)
}

// renameInvolved is the paths of exact renames from base to side: a path side
// deleted, and one it added with the same content.
func renameInvolved(base, side map[string]string) map[string]bool {
	out := map[string]bool{}
	for from, content := range base {
		if _, kept := side[from]; kept {
			continue
		}
		for to, c := range side {
			if _, existed := base[to]; !existed && c == content {
				out[from], out[to] = true, true
			}
		}
	}
	return out
}

var markerLabel = regexp.MustCompile(`(?m)^([<|=>]{7,}) .*$`)

// unlabelled is content with its marker lines' labels removed.
func unlabelled(s string) string { return markerLabel.ReplaceAllString(s, "$1") }

// genTree is a tree of a few files: from scratch, or changed from base by
// keeping, changing, deleting and adding files. Paths include a pair that
// differ only in case, and a file that's a folder in other trees.
func genTree(rng *rand.Rand, base map[string]string) map[string]string {
	paths := []string{"a.aml", "b.aml", "c.aml", "models/d.aml", "models/e.aml", "apps/f.html", "apps/g", "apps/g/index.html", "A.aml"}
	t := map[string]string{}
	if base == nil {
		for _, p := range paths {
			if rng.IntN(3) == 0 {
				t[p] = genContent(rng)
			}
		}
	} else {
		for _, p := range slices.Sorted(maps.Keys(base)) {
			switch rng.IntN(5) {
			case 0: // deleted
			case 1, 2:
				t[p] = editContent(rng, base[p])
			default:
				t[p] = base[p]
			}
		}
		for _, p := range paths {
			if _, ok := base[p]; !ok && rng.IntN(5) == 0 {
				t[p] = genContent(rng)
			}
		}
	}
	// One tree can't hold a file and a folder at one path.
	if _, ok := t["apps/g"]; ok {
		if _, ok := t["apps/g/index.html"]; ok {
			delete(t, "apps/g")
		}
	}
	if rng.IntN(4) != 0 {
		delete(t, "A.aml") // a case-only pair, but rarely
	}
	return t
}

func genContent(rng *rand.Rand) string {
	if rng.IntN(15) == 0 {
		return "\x00binary" + string(rune('a'+rng.IntN(3)))
	}
	words := []string{"a", "b", "}", "", "  x", "y {"}
	var b strings.Builder
	for range 1 + rng.IntN(8) {
		b.WriteString(words[rng.IntN(len(words))])
		b.WriteByte('\n')
	}
	return b.String()
}

func editContent(rng *rand.Rand, text string) string {
	if strings.HasPrefix(text, "\x00") {
		return "\x00binary" + string(rune('a'+rng.IntN(3)))
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	fresh := []string{"P\n", "Q\n", "}\n", "\n"}
	for range 1 + rng.IntN(3) {
		i := rng.IntN(len(lines) + 1)
		switch rng.IntN(3) {
		case 0:
			if i < len(lines) {
				lines[i] = fresh[rng.IntN(len(fresh))]
			}
		case 1:
			if i < len(lines) {
				lines = append(lines[:i], lines[i+1:]...)
			}
		default:
			lines = append(lines[:i], append([]string{fresh[rng.IntN(len(fresh))]}, lines[i:]...)...)
		}
	}
	return strings.Join(lines, "")
}

// gitRepo is a scratch repo to merge trees in.
type gitRepo struct{ dir string }

func newGitRepo(t *testing.T) gitRepo {
	t.Helper()
	g := gitRepo{dir: t.TempDir()}
	g.run(t, "", "init", "-q")
	if _, err := g.try("", "merge-tree", "--write-tree", "-h"); strings.Contains(err, "unknown option") {
		t.Skip("this git has no merge-tree --write-tree (2.38 or later)")
	}
	return g
}

// merge commits base, then ours and theirs on it, merges them, and answers
// the merged tree's files and the paths git reported as conflicted.
func (g gitRepo) merge(t *testing.T, base, ours, theirs map[string]string) (map[string]string, map[string]bool) {
	t.Helper()
	b := g.commit(t, base, "")
	o, th := g.commit(t, ours, b), g.commit(t, theirs, b)
	out, _ := g.try("", "-c", "merge.conflictStyle=diff3",
		"merge-tree", "--write-tree", "--no-messages", "-X", "diff-algorithm=myers", "-X", "find-renames=100%", o, th)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	tree := lines[0]
	conflicted := map[string]bool{}
	for _, l := range lines[1:] {
		if _, path, ok := strings.Cut(l, "\t"); ok {
			conflicted[path] = true
		}
	}
	files := map[string]string{}
	for _, l := range strings.Split(strings.TrimRight(g.run(t, "", "ls-tree", "-r", tree), "\n"), "\n") {
		meta, path, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		files[path] = g.run(t, "", "cat-file", "blob", fields[2])
	}
	return files, conflicted
}

// commit writes files as a commit on parent, through an index of its own.
func (g gitRepo) commit(t *testing.T, files map[string]string, parent string) string {
	t.Helper()
	var info strings.Builder
	for _, p := range slices.Sorted(maps.Keys(files)) {
		id := strings.TrimSpace(g.run(t, files[p], "hash-object", "-w", "--stdin"))
		info.WriteString("100644 blob " + id + "\t" + p + "\n")
	}
	index := filepath.Join(g.dir, "index-"+strings.Repeat("x", 4))
	_ = os.Remove(index)
	env := []string{"GIT_INDEX_FILE=" + index}
	g.runEnv(t, env, info.String(), "update-index", "--index-info")
	tree := strings.TrimSpace(g.runEnv(t, env, "", "write-tree"))
	args := []string{"commit-tree", tree, "-m", "c"}
	if parent != "" {
		args = append(args, "-p", parent)
	}
	return strings.TrimSpace(g.run(t, "", args...))
}

func (g gitRepo) run(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	return g.runEnv(t, nil, stdin, args...)
}

func (g gitRepo) runEnv(t *testing.T, env []string, stdin string, args ...string) string {
	t.Helper()
	out, err := g.exec(env, stdin, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// try runs git and answers its output and error text, for commands whose
// failure is an answer: a merge with conflicts exits 1.
func (g gitRepo) try(stdin string, args ...string) (string, string) {
	out, err := g.exec(nil, stdin, args...)
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return out, string(exit.Stderr) + out
	}
	return out, ""
}

func (g gitRepo) exec(env []string, stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	return string(out), err
}
