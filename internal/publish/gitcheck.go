package publish

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/holistics/anfra/shared/repoformat"
)

// shownPaths is how many paths a warning names before "and N more".
const shownPaths = 5

// gitState is what a publish from a git folder may leave out of git: the
// branch, when it isn't the remote's default; commits not pushed; and the
// files a publish sends that aren't committed.
type gitState struct {
	branch, defaultBranch string // defaultBranch "" when unknown
	upstream              string
	ahead                 int
	uncommitted           []string // relative to the folder, sorted as git lists them
}

// readGit reads dir's git state, without the network. ok is false outside a
// git repo, or without git: then there's nothing to say.
func readGit(ctx context.Context, dir string) (s gitState, ok bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return s, false
	}
	prefix, err := gitOutput(ctx, dir, "rev-parse", "--show-prefix")
	if err != nil {
		return s, false
	}
	prefix = strings.TrimSpace(prefix)
	out, err := gitOutput(ctx, dir, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all", "--no-renames", "--", ".")
	if err != nil {
		return s, false
	}
	for rec := range strings.SplitSeq(out, "\x00") {
		var path string
		switch {
		case strings.HasPrefix(rec, "# branch.head "):
			s.branch = strings.TrimPrefix(rec, "# branch.head ")
		case strings.HasPrefix(rec, "# branch.upstream "):
			s.upstream = strings.TrimPrefix(rec, "# branch.upstream ")
		case strings.HasPrefix(rec, "# branch.ab "):
			ahead, _, _ := strings.Cut(strings.TrimPrefix(rec, "# branch.ab +"), " ")
			s.ahead, _ = strconv.Atoi(ahead)
		case strings.HasPrefix(rec, "1 "):
			if f := strings.SplitN(rec, " ", 9); len(f) == 9 {
				path = f[8]
			}
		case strings.HasPrefix(rec, "u "):
			if f := strings.SplitN(rec, " ", 11); len(f) == 11 {
				path = f[10]
			}
		case strings.HasPrefix(rec, "? "):
			path = rec[2:]
		}
		// git names paths from the repo's root; a publish, from its folder.
		if rel, ok := strings.CutPrefix(path, prefix); ok && path != "" && repoformat.Publishable(rel) {
			s.uncommitted = append(s.uncommitted, rel)
		}
	}
	remote := "origin"
	if r, _, ok := strings.Cut(s.upstream, "/"); ok {
		remote = r
	}
	if def, err := gitOutput(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/"+remote+"/HEAD"); err == nil {
		s.defaultBranch = strings.TrimPrefix(strings.TrimSpace(def), remote+"/")
	}
	return s, true
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output() //nolint:gosec // G204: git, with this package's own arguments, never a shell
	return string(out), err
}

// warnOfGit warns of what publishing dir leaves out of git, or left: the
// folder is on a branch other than the remote's default, has commits not
// pushed, or has files to publish not committed. Outside git, or without git,
// it says nothing. It never refuses: a team may share its work through the
// org, and use git only on its own.
func warnOfGit(ctx context.Context, t *told, dir string) {
	s, ok := readGit(ctx, dir)
	if !ok {
		return
	}
	// A detached HEAD is how CI checks a commit out: no branch to compare.
	if s.defaultBranch != "" && s.branch != "(detached)" && s.branch != s.defaultBranch {
		t.Warn("git_non_default_branch", "This folder is on the branch %s, not %s, the remote's default. Consider merging your changes into %s, and publishing from there.",
			s.branch, s.defaultBranch, s.defaultBranch)
	}
	if s.upstream != "" && s.ahead > 0 {
		t.Warn("git_unpushed", "This folder has %d %s not pushed to %s. Consider pushing %s.",
			s.ahead, pick(s.ahead, "commit", "commits"), s.upstream, itThem(s.ahead))
	}
	if n := len(s.uncommitted); n > 0 {
		names := s.uncommitted
		if n > shownPaths {
			names = append(names[:shownPaths:shownPaths], fmt.Sprintf("%d more", n-shownPaths))
		}
		t.Warn("git_uncommitted", "%d %s publish sends %s uncommitted changes: %s. Consider committing %s.",
			n, pick(n, "file", "files"), hasHave(n), joinPaths(names), itThem(n))
	}
}
