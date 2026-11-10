package textmerge_test

import (
	"errors"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/textmerge"
)

// TestAgainstGit merges generated edits with textmerge and with
// `git merge-file --diff3`, the reference. Each result must be git's, or a
// conflict where git resolved: textmerge may be more cautious than git, never
// different. Where both conflict but place the conflict differently, the
// diffs aligned lines differently; that's counted and reported, not failed.
//
// git runs only here, never in what's shipped. Skipped without git.
func TestAgainstGit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	dir := t.TempDir()
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // G404: a fixed seed, so a failing case can be replayed
	const cases = 1500
	var same, cautious, placedDifferently int
	for i := range cases {
		base := genFile(rng)
		ours, theirs := edit(rng, base), edit(rng, base)
		want, wantConflicts := gitMerge(t, dir, base, ours, theirs)
		r := textmerge.Merge([]byte(base), []byte(ours), []byte(theirs))
		got := string(r.Render(labels))
		switch {
		case got == want && (r.Conflicts > 0) == (wantConflicts > 0):
			same++
		case r.Conflicts > 0 && wantConflicts == 0:
			cautious++
			t.Logf("case %d conflicts where git resolved\nbase:   %q\nours:   %q\ntheirs: %q\ngit:\n%q\ntextmerge:\n%q", i, base, ours, theirs, want, got)
		case r.Conflicts > 0 && wantConflicts > 0:
			placedDifferently++
			t.Logf("case %d conflicts elsewhere than git\nbase:   %q\nours:   %q\ntheirs: %q\ngit:\n%q\ntextmerge:\n%q", i, base, ours, theirs, want, got)
		default:
			t.Errorf("case %d resolved differently from git\nbase:   %q\nours:   %q\ntheirs: %q\ngit (%d conflicts):\n%q\ntextmerge (%d):\n%q",
				i, base, ours, theirs, wantConflicts, want, r.Conflicts, got)
		}
	}
	t.Logf("%d cases: %d as git, %d conflicting where git resolved, %d conflicting elsewhere than git", cases, same, cautious, placedDifferently)
}

// gitMerge runs `git merge-file --diff3` with Myers, textmerge's diff, and
// the same labels, and answers its output and its number of conflicts.
func gitMerge(t *testing.T, dir, base, ours, theirs string) (string, int) {
	t.Helper()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	o, b, th := write("ours", ours), write("base", base), write("theirs", theirs)
	cmd := exec.Command("git", "merge-file", "-p", "--diff3", "-L", "ours", "-L", "base", "-L", "theirs", o, b, th)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.Output()
	conflicts := 0
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() > 0 && exit.ExitCode() < 128 {
		conflicts = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("git merge-file: %v", err)
	}
	return string(out), conflicts
}

// genFile is a file of a few lines from a small alphabet, so lines repeat, as
// braces and blank lines do in AML.
func genFile(rng *rand.Rand) string {
	words := []string{"a", "b", "c", "}", "", "  x", "y {"}
	var b strings.Builder
	for range rng.IntN(12) {
		b.WriteString(words[rng.IntN(len(words))])
		b.WriteByte('\n')
	}
	return b.String()
}

// edit changes a few lines of text: replacing, deleting or inserting.
func edit(rng *rand.Rand, text string) string {
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	fresh := []string{"P\n", "Q\n", "}\n", "\n", "R\n"}
	for range rng.IntN(4) {
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
		case 2:
			lines = append(lines[:i], append([]string{fresh[rng.IntN(len(fresh))]}, lines[i:]...)...)
		}
	}
	return strings.Join(lines, "")
}
