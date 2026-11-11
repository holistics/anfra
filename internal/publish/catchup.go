package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/publish/syncstate"
	"github.com/holistics/anfra/internal/publish/workdir"
	"github.com/holistics/anfra/shared/apperr"
)

// publishSync is a publish's view of the folder's sync state: what it last
// pulled or published, and the base the publish names.
type publishSync struct {
	dir   string
	sync  syncstate.Folder
	state syncstate.State
	p     *cliui.Printer
	env   Env
	// base is the revision this publish is built on, once catchUp has
	// checked it: the server refuses the publish if it's no longer published.
	base string
}

// openSync takes the folder's lock and reads its sync state. A folder anfra
// can't write to, such as a read-only checkout, publishes without the lock,
// and its sync state isn't saved.
func openSync(dir string, p *cliui.Printer, env Env) (*publishSync, func(), error) {
	sc := &publishSync{dir: dir, sync: syncstate.For(dir), p: p, env: env}
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
// it carries on. Otherwise it asks whether a pull would change the folder:
//   - it wouldn't: the org's changes are already here, and the publish
//     carries on from the published revision;
//   - it would, and pull is set: the org's changes are merged into the folder
//     (pulled), and the publish carries on, unless they conflict;
//   - it would: the publish stops with pull_required.
func (sc *publishSync) catchUp(ctx context.Context, client *cloudapi.Client, org cloudapi.Org, status cloudapi.StatusOut, local workdir.Tree, pull bool) (*Pulled, error) {
	if status.Revision == "" {
		return nil, nil
	}
	base := sc.lastBase(org)
	var pulled *Pulled
	if base != status.Revision {
		c, err := compare(ctx, client, status, base, local)
		if err != nil {
			return nil, err
		}
		if err := c.unselected(verbPublish); err != nil {
			return nil, err
		}
		switch {
		case c.pullWouldChange() && pull:
			if pulled, err = sc.pull(ctx, client, org, c); err != nil {
				return nil, err
			}
		case c.pullWouldChange():
			return nil, pullRequired(org, liveOf(status), base != "")
		case status.PublishedBy != nil:
			sc.p.Success("%s published to %s on %s, and their changes are already in this folder.",
				who(status.PublishedBy), org, status.PublishedAt.Local().Format("2006-01-02 15:04"))
		}
	}
	sc.base = status.Revision
	return pulled, nil
}

// pull merges the published revision of c into the folder, as anfra pull
// does, and stops the publish when the merge leaves conflicts.
func (sc *publishSync) pull(ctx context.Context, client *cloudapi.Client, org cloudapi.Org, c comparison) (*Pulled, error) {
	whose := "the org's changes"
	if c.status.PublishedBy != nil {
		whose = who(c.status.PublishedBy) + "'s changes"
	}
	sc.p.Step("Pulling %s…", whose)
	pl := puller{dir: sc.dir, sync: sc.sync, state: sc.state, env: sc.env, p: sc.p}
	m, err := pl.merge(ctx, client, org, c)
	if err != nil {
		return nil, err
	}
	if len(m.conflicts) > 0 {
		files, details := listConflicts(m.conflicts)
		return nil, apperr.NewWith(clierr.ConflictsUnresolved,
			fmt.Sprintf("Not published, because pulling %s left %s, in %s. Resolve %s (`anfra publish status` says how), then run `anfra publish` again.", whose, countConflicts(m.conflicts), files, itThem(len(m.conflicts))),
			details)
	}
	out := &Pulled{Revision: c.status.Revision, Changes: append([]FileChange{}, m.changes...)}
	if c.status.PublishedBy != nil {
		out.PublishedBy = c.status.PublishedBy.Email
	}
	if !c.status.PublishedAt.IsZero() {
		at := c.status.PublishedAt
		out.PublishedAt = &at
	}
	sc.p.Success("Pulled %s: %s.", whose, countChanges(m.changes))
	return out, nil
}

// countChanges is changes counted by kind: "1 file added, 2 updated".
func countChanges(changes []FileChange) string {
	counts := map[string]int{}
	for _, c := range changes {
		counts[c.Change]++
	}
	var parts []string
	for _, kind := range []string{"added", "updated", "merged", "deleted"} {
		switch n := counts[kind]; {
		case n == 0:
		case len(parts) == 0:
			parts = append(parts, fmt.Sprintf("%d %s %s", n, pick(n, "file", "files"), kind))
		default:
			parts = append(parts, fmt.Sprintf("%d %s", n, kind))
		}
	}
	return strings.Join(parts, ", ")
}

// lastBase is the revision this folder last pulled or published to org, ""
// when there's none.
func (sc *publishSync) lastBase(org cloudapi.Org) string {
	if s := sc.state[org.String()]; s != nil {
		return s.Base
	}
	return ""
}

func liveOf(status cloudapi.StatusOut) cloudapi.LiveRevision {
	return cloudapi.LiveRevision{Revision: status.Revision, PublishedBy: status.PublishedBy, PublishedAt: status.PublishedAt}
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
		msg = fmt.Sprintf("Not published, because %s published to %s on %s, after this folder last pulled or published. Run `anfra publish --pull` to merge their changes and publish, or `anfra pull` to review them first.",
			who(live.PublishedBy), org, live.PublishedAt.Local().Format("2006-01-02 15:04"))
	default:
		msg = fmt.Sprintf("Not published, because %s has files this folder hasn't pulled, and anfra has no record of this folder pulling from or publishing to it. Run `anfra publish --pull` to merge them and publish, or `anfra pull` to review them first.", org)
	}
	return apperr.NewWith(clierr.PullRequired, msg, details)
}
