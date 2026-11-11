package publish

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gitWarnings is what a publish check of dir warns of git, a line each.
func gitWarnings(t *testing.T, dir string) []string {
	t.Helper()
	_, said, err := run(t, dir, Options{SetOrg: "https://bi.example.com/o/acme", Check: true}, never(t))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for line := range strings.SplitSeq(said, "\n") {
		if strings.HasPrefix(line, "git_") {
			out = append(out, line)
		}
	}
	return out
}

// sharedClone is a git repo in a folder of its own, with main pushed to a
// remote whose default branch is main.
func sharedClone(t *testing.T, files map[string]string) string {
	t.Helper()
	remote, dir := t.TempDir(), t.TempDir()
	git(t, remote, "init", "-q", "--bare", "-b", "main")
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, files)
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "start")
	git(t, dir, "remote", "add", "origin", remote)
	git(t, dir, "push", "-q", "-u", "origin", "main")
	git(t, dir, "remote", "set-head", "origin", "main")
	return dir
}

// Publishing what isn't in the remote's default branch is warned of: another
// branch, commits not pushed, and files to publish not committed. Files a
// publish doesn't send don't count.
func TestPublishWarnsOfGit(t *testing.T) {
	dir := sharedClone(t, map[string]string{"a.aml": "a", "b.aml": "b", "README.md": "r"})
	if got := gitWarnings(t, dir); len(got) != 0 {
		t.Fatalf("an up-to-date main: %q", got)
	}

	write(t, dir, map[string]string{"b.aml": "b2"})
	git(t, dir, "commit", "-qam", "b2")
	write(t, dir, map[string]string{"a.aml": "a2", "c.aml": "c", "README.md": "r2"})
	git(t, dir, "switch", "-q", "-c", "my-change")
	got := strings.Join(gitWarnings(t, dir), "\n")
	for _, want := range []string{
		"git_non_default_branch: This folder is on the branch my-change, not main, the remote's default. Consider merging your changes into main, and publishing from there.",
		"git_uncommitted: 2 files publish sends have uncommitted changes: a.aml and c.aml. Consider committing them.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "git_unpushed") {
		t.Errorf("a branch without an upstream has nothing to push to: %s", got)
	}

	git(t, dir, "switch", "-q", "main")
	if got := strings.Join(gitWarnings(t, dir), "\n"); !strings.Contains(got, "git_unpushed: This folder has 1 commit not pushed to origin/main. Consider pushing it.") || strings.Contains(got, "git_non_default_branch") {
		t.Errorf("unpushed on main: %s", got)
	}

	// A detached HEAD, as CI checks out, has no branch to compare.
	git(t, dir, "stash", "-q", "-u")
	git(t, dir, "checkout", "-q", "--detach")
	if got := gitWarnings(t, dir); len(got) != 0 {
		t.Errorf("detached: %q", got)
	}
}

// A repo in a folder of a git repo counts its own files, named from it.
func TestPublishWarnsOfGitInSubfolder(t *testing.T) {
	root := sharedClone(t, map[string]string{"bi/a.aml": "a", "other/x.aml": "x"})
	write(t, root, map[string]string{"bi/a.aml": "a2", "bi/apps/d.html": "d", "other/x.aml": "x2"})
	got := strings.Join(gitWarnings(t, filepath.Join(root, "bi")), "\n")
	if !strings.Contains(got, "git_uncommitted: 2 files publish sends have uncommitted changes: a.aml and apps/d.html.") {
		t.Errorf("got %s", got)
	}
}

// Outside git, or without git, nothing is said.
func TestPublishWarnsOfGitOnlyWithGit(t *testing.T) {
	plain := t.TempDir()
	write(t, plain, map[string]string{"a.aml": "a"})
	if got := gitWarnings(t, plain); len(got) != 0 {
		t.Errorf("outside git: %q", got)
	}
	dir := sharedClone(t, map[string]string{"a.aml": "a"})
	git(t, dir, "switch", "-q", "-c", "my-change")
	write(t, dir, map[string]string{"a.aml": "a2"})
	t.Setenv("PATH", t.TempDir())
	if got := gitWarnings(t, dir); len(got) != 0 {
		t.Errorf("without git: %q", got)
	}
}

// Many uncommitted files are counted past the first few.
func TestPublishWarnsOfGitManyFiles(t *testing.T) {
	dir := sharedClone(t, map[string]string{"a.aml": "a"})
	for _, n := range []string{"b", "c", "d", "e", "f", "g", "h"} {
		if err := os.WriteFile(filepath.Join(dir, n+".aml"), []byte(n), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := strings.Join(gitWarnings(t, dir), "\n")
	if !strings.Contains(got, "7 files publish sends have uncommitted changes: b.aml, c.aml, d.aml, e.aml, f.aml, and 2 more. Consider committing them.") {
		t.Errorf("got %s", got)
	}
}
