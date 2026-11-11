package publish

import (
	"context"
	"maps"
	"slices"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/publish/workdir"
	"github.com/holistics/anfra/shared/treemerge"
)

// comparison is the three versions pull, publish and publish status compare,
// by blob id: the base (what this folder last pulled or published), the
// folder, and the org's published revision (docs/designs/publish.md).
type comparison struct {
	status cloudapi.StatusOut
	// base is the revision compared from: "" when there's none, or the org
	// no longer has it, so everything is compared from nothing.
	base     string
	baseTree treemerge.Tree
	live     treemerge.Tree
	local    workdir.Tree
	paths    []treemerge.PathResult // the classification, by path
}

// compare lists the published revision's files, and the base's when it
// differs, and classifies every path.
func compare(ctx context.Context, client *cloudapi.Client, status cloudapi.StatusOut, base string, local workdir.Tree) (comparison, error) {
	c := comparison{status: status, base: base, local: local}
	var err error
	if c.live, err = listTree(ctx, client, status.Revision); err != nil {
		return comparison{}, err
	}
	switch {
	case base == status.Revision:
		c.baseTree = c.live
	case base != "":
		if c.baseTree, err = listTree(ctx, client, base); err != nil {
			if !cloudapi.IsCode(err, "not_found") {
				return comparison{}, err
			}
			c.base = ""
		}
	}
	if c.base == "" {
		c.baseTree = treemerge.Tree{}
	}
	c.paths = treemerge.Classify(c.baseTree, local.Files(), c.live)
	return c, nil
}

// unselected refuses a file on disk at a path the base or the org has, that
// the folder's selection leaves out: a merge would read it as deleted, and a
// publish would delete it from the org (workdir.Tree.Unselected).
func (c comparison) unselected(v verb) error {
	odd := c.local.Unselected(slices.Sorted(maps.Keys(union(c.baseTree, c.live))))
	if len(odd) == 0 {
		return nil
	}
	n := len(odd)
	return clierr.Precondition("%s, because %s %s in this folder, but anfra doesn't publish %s (a symlink, or a file anfra no longer publishes), and the org has a file at that path. Replace %s with a regular file, or remove %s, then %s.",
		v.not, joinPaths(odd), isAre(n), itThem(n), itThem(n), itThem(n), v.again)
}

// pullWouldChange says whether a pull would change the folder: any path it
// would take, merge, or mark as a conflict.
func (c comparison) pullWouldChange() bool {
	return slices.ContainsFunc(c.paths, func(r treemerge.PathResult) bool {
		return r.Outcome != treemerge.Unchanged && r.Outcome != treemerge.Ours
	})
}

// incoming is what the org changed since the base, as a pull would bring it:
// each path's change to the folder.
func (c comparison) incoming() []FileChange {
	var out []FileChange
	for _, r := range c.paths {
		switch r.Outcome {
		case treemerge.Theirs:
			out = append(out, FileChange{Path: r.Path, Change: sideChange(r.Ours, r.Theirs)})
		case treemerge.NeedsMerge:
			out = append(out, FileChange{Path: r.Path, Change: "merged"})
		case treemerge.Conflict:
			out = append(out, FileChange{Path: r.Path, Change: "conflict"})
		}
	}
	return out
}

// outgoing is what the folder changed since the base, that the org doesn't
// have: what publishing would send.
func (c comparison) outgoing() []FileChange {
	var out []FileChange
	for _, r := range c.paths {
		if r.Ours != r.Base && r.Ours != r.Theirs {
			out = append(out, FileChange{Path: r.Path, Change: sideChange(r.Base, r.Ours)})
		}
	}
	return out
}

// sideChange names the change from one blob id to another: "added",
// "deleted", or "updated".
func sideChange(from, to string) string {
	switch {
	case from == "":
		return "added"
	case to == "":
		return "deleted"
	}
	return "updated"
}
