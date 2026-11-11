package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/publish/syncstate"
	"github.com/holistics/anfra/internal/publish/workdir"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/treemerge"
)

// publishSync is a publish's view of the folder's sync state: what it last
// pulled or published, and the base the publish names.
type publishSync struct {
	dir   string
	sync  syncstate.Folder
	state syncstate.State
	p     *cliui.Printer
	// base is the revision this publish is built on, once catchUp has
	// checked it: the server refuses the publish if it's no longer published.
	base string
}

// openSync takes the folder's lock and reads its sync state. A folder anfra
// can't write to, such as a read-only checkout, publishes without the lock,
// and its sync state isn't saved.
func openSync(dir string, p *cliui.Printer) (*publishSync, func(), error) {
	sc := &publishSync{dir: dir, sync: syncstate.For(dir), p: p}
	release, err := sc.sync.Lock()
	if err != nil {
		if _, ok := errors.AsType[*fs.PathError](err); !ok {
			return nil, nil, err // another pull or publish holds it
		}
		release = func() {}
	}
	if sc.state, err = sc.sync.Load(); err != nil {
		release()
		return nil, nil, err
	}
	return sc, release, nil
}

// catchUp checks that publishing replaces nothing this folder hasn't seen.
// When the org's published revision is the one last pulled or published here,
// it carries on. Otherwise it asks whether a pull would change the folder: if
// it would, the publish stops with pull_required; if not, the org's changes
// are already here, and the publish carries on from the published revision.
// local is the folder's selected files: path → blob id.
func (sc *publishSync) catchUp(ctx context.Context, client *cloudapi.Client, org cloudapi.Org, status cloudapi.StatusOut, local map[string]string) error {
	live := cloudapi.LiveRevision{Revision: status.Revision, PublishedBy: status.PublishedBy, PublishedAt: status.PublishedAt}
	if status.Revision == "" {
		return nil
	}
	var base string
	if s := sc.state[org.String()]; s != nil {
		base = s.Base
	}
	if base != status.Revision {
		changes, err := sc.pullWouldChange(ctx, client, base, status.Revision, local)
		if err != nil {
			return err
		}
		if changes {
			return pullRequired(org, live, base != "")
		}
		if status.PublishedBy != nil {
			sc.p.Success("%s published to %s on %s, and their changes are already in this folder.",
				who(status.PublishedBy), org, status.PublishedAt.Local().Format("2006-01-02 15:04"))
		}
	}
	sc.base = status.Revision
	return nil
}

// pullWouldChange says whether pulling revision would change the folder:
// treemerge's classification of the base's files, the folder's and the
// published revision's, by blob id only. A base the org no longer has is
// none, as a pull reads it.
func (sc *publishSync) pullWouldChange(ctx context.Context, client *cloudapi.Client, base, revision string, local map[string]string) (bool, error) {
	published, err := listTree(ctx, client, revision)
	if err != nil {
		return false, err
	}
	baseTree := treemerge.Tree{}
	if base != "" {
		if baseTree, err = listTree(ctx, client, base); err != nil {
			if !cloudapi.IsCode(err, "not_found") {
				return false, err
			}
			baseTree = treemerge.Tree{}
		}
	}
	tree, err := workdir.Read(sc.dir)
	if err != nil {
		return false, err
	}
	if odd := tree.Unselected(slices.Sorted(maps.Keys(union(baseTree, published)))); len(odd) > 0 {
		return false, clierr.Precondition("Not published, because %s %s in this folder, but anfra doesn't publish %s (a symlink, or a file anfra no longer publishes), and the org has a file at that path. Replace %s with a regular file, or remove %s, then publish again.",
			joinPaths(odd), isAre(len(odd)), itThem(len(odd)), itThem(len(odd)), itThem(len(odd)))
	}
	for _, r := range treemerge.Classify(baseTree, local, published) {
		if r.Outcome != treemerge.Unchanged && r.Outcome != treemerge.Ours {
			return true, nil
		}
	}
	return false, nil
}

// record makes revision the folder's base for org. Best-effort: without it,
// the next publish asks the org whether a pull would change anything.
func (sc *publishSync) record(org cloudapi.Org, revision string) {
	s := sc.state[org.String()]
	if s == nil {
		s = &syncstate.Org{}
		sc.state[org.String()] = s
	}
	s.Base = revision
	_ = sc.sync.Save(sc.state)
}

// pullRequired is the refusal of a publish that would replace what the org
// has and this folder hasn't seen. hadBase says whether the folder had
// pulled or published before.
func pullRequired(org cloudapi.Org, live cloudapi.LiveRevision, hadBase bool) error {
	details := clierr.Published{Revision: live.Revision, PublishedAt: live.PublishedAt}
	if live.PublishedBy != nil {
		details.PublishedBy = live.PublishedBy.Email
	}
	var msg string
	switch {
	case hadBase && live.PublishedBy != nil:
		msg = fmt.Sprintf("Not published, because %s published to %s on %s, after this folder last pulled or published. Run `anfra pull` to merge their changes, then publish again.",
			who(live.PublishedBy), org, live.PublishedAt.Local().Format("2006-01-02 15:04"))
	default:
		msg = fmt.Sprintf("Not published, because %s has files this folder hasn't pulled, and anfra has no record of this folder pulling from or publishing to it. Run `anfra pull` to merge them, then publish again.", org)
	}
	return apperr.NewWith(clierr.PullRequired, msg, details)
}
