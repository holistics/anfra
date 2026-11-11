package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/publish/syncstate"
	"github.com/holistics/anfra/internal/publish/workdir"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/treemerge"
)

// PullOptions are a pull's choices: the command's flags. At most one of Take
// and Abort.
type PullOptions struct {
	// SetOrg is an org's address to record as the repo's, and pull from.
	SetOrg string
	// Take is "org" or "local": resolve the conflicts on Paths by keeping that
	// side's version of the whole files.
	Take  string
	Paths []string
	// Abort undoes the last pull while it isn't finished.
	Abort bool
}

// PullResult is a pull's result: what it merged into the folder, and what's
// left to resolve. --take and --abort answer it too, with what they did.
type PullResult struct {
	Message      string                    `json:"message" doc:"the outcome, in a sentence"`
	Org          *OrgRef                   `json:"org,omitempty" doc:"the org pulled from"`
	Revision     string                    `json:"revision,omitempty" doc:"the org's revision this pull merged into the folder"`
	PublishedBy  string                    `json:"published_by,omitempty" doc:"who published that revision. Absent for the org's starting files."`
	PublishedAt  *time.Time                `json:"published_at,omitempty" doc:"when that revision was published" pretty:",moment"`
	Base         string                    `json:"base,omitempty" doc:"the org's revision this folder matched before the pull. Absent when anfra had no record of one."`
	Changes      []FileChange              `json:"changes,omitempty" doc:"the files the pull wrote or deleted, other than those with conflicts"`
	Conflicts    map[string][]PullConflict `json:"conflicts,omitempty" doc:"the conflicts left to resolve, by kind. Kinds: \"content\", \"add_add\", \"modify_delete\", \"delete_modify\", \"binary\", \"file_folder\", \"path_collision\"."`
	HowToResolve map[string]string         `json:"how_to_resolve,omitempty" doc:"how to resolve each kind of conflict left, by kind"`
	Taken        []TakenSide               `json:"taken,omitempty" doc:"with --take: the conflicts resolved, and the side kept"`
	Restored     []string                  `json:"restored,omitempty" doc:"with --abort: the files put back as they were before the pull"`
	Hints        []string                  `json:"hints,omitempty" doc:"what to do next, if any"`
}

// FileChange is a file a pull changed, or one a pull or a publish would
// change.
type FileChange struct {
	Path   string `json:"path" doc:"the file's path in the repo"`
	Change string `json:"change" doc:"what happens to it. \"added\", \"updated\", \"deleted\", \"merged\" (both the folder and the org changed it, and the changes combine), or \"conflict\" (they changed it in ways that can't combine)."`
}

// PullConflict is one conflict: the paths it covers, and what happened.
type PullConflict struct {
	Paths   []string `json:"paths" doc:"the files the conflict covers. Most cover one; a file here and a folder on the org, or paths differing only in case, cover several." pretty:"-"`
	Message string   `json:"message" doc:"what happened, naming its files"`
}

// TakenSide is a conflict resolved by keeping one side's version.
type TakenSide struct {
	Paths []string `json:"paths" doc:"the files the conflict covers"`
	Kept  string   `json:"kept" doc:"whose version was kept. \"org\" or \"local\"."`
}

// Pull merges the org's published files into the folder at dir, or, with
// o.Take or o.Abort, resolves or undoes the last pull
// (docs/designs/publish.md).
func Pull(ctx context.Context, dir string, o PullOptions, env Env) (PullResult, error) {
	if _, err := os.Stat(filepath.Join(dir, repo.ConfigDirName)); err != nil {
		return PullResult{}, clierr.Precondition("Not pulled, because this folder isn't an anfra repo. Run `anfra init` first, then `anfra pull`.")
	}
	org, configured, err := readOrg(dir)
	if o.SetOrg != "" {
		org, err = cloudapi.ParseOrgAddress(o.SetOrg)
		configured = true
	}
	if err != nil {
		return PullResult{}, err
	}
	sync := syncstate.For(dir)
	release, err := sync.Lock()
	if err != nil {
		return PullResult{}, err
	}
	defer release()
	state, err := sync.Load()
	if err != nil {
		return PullResult{}, err
	}
	p := cliui.NewPrinter(env.ErrOut)
	pl := puller{dir: dir, sync: sync, state: state, env: env, p: p}
	switch {
	case o.Abort:
		return pl.abort(org, configured)
	case o.Take != "":
		return pl.take(ctx, org, configured, o)
	}
	return pl.pull(ctx, org, configured, o.SetOrg != "")
}

// puller is one run of pull, holding the folder's lock.
type puller struct {
	dir   string
	sync  syncstate.Folder
	state syncstate.State
	env   Env
	p     *cliui.Printer
}

func (pl *puller) org(org cloudapi.Org) *syncstate.Org {
	s := pl.state[org.String()]
	if s == nil {
		s = &syncstate.Org{}
		pl.state[org.String()] = s
	}
	return s
}

// settle checks the sync's pending record before a pull or a publish: a pull
// stopped partway refuses, unresolved conflicts refuse, naming their files,
// and conflicts all resolved make the pull's revision the base.
func settle(dir string, sync syncstate.Folder, state syncstate.State, org cloudapi.Org, v verb) error {
	s := state[org.String()]
	settled, err := pendingRefusal(dir, s, v)
	if err != nil || !settled {
		return err
	}
	s.Base, s.Pending = s.Pending.Revision, nil
	if err := sync.Save(state); err != nil {
		return err
	}
	_ = os.RemoveAll(sync.BackupDir())
	return nil
}

// pendingRefusal is settle's check alone, changing nothing: the refusal of a
// pull stopped partway, or of unresolved conflicts. settled says the pending
// pull's conflicts are all resolved, so its revision can become the base.
func pendingRefusal(dir string, s *syncstate.Org, v verb) (settled bool, err error) {
	if s == nil || s.Pending == nil {
		return false, nil
	}
	switch s.Pending.Stage {
	case syncstate.Writing:
		return false, apperr.New(clierr.PullInterrupted, fmt.Sprintf("%s, because the last pull stopped partway through writing files. Run `anfra pull --abort` to put them back as they were, then try again.", v.not))
	case syncstate.Conflicted:
		if open := openConflicts(dir, s); len(open) > 0 {
			files, details := listConflicts(open)
			return false, apperr.NewWith(clierr.ConflictsUnresolved,
				fmt.Sprintf("%s, because %s still %s conflicts from the last pull. Resolve them (`anfra publish status` says how), or undo that pull with `anfra pull --abort`, then try again.", v.not, files, hasHave(len(details.Paths))),
				details)
		}
		return true, nil
	}
	return false, nil
}

// listConflicts names conflicts' files, each with its kind ("a.aml
// (content) and logo.png (binary)"), and is them as conflicts_unresolved's
// details.
func listConflicts(cs []syncstate.Conflict) (string, clierr.UnresolvedFiles) {
	details := clierr.UnresolvedFiles{}
	var named []string
	for _, c := range cs {
		var paths []string
		for _, f := range c.Files {
			paths = append(paths, f.Path)
		}
		details.Paths = append(details.Paths, paths...)
		details.Conflicts = append(details.Conflicts, clierr.UnresolvedConflict{Kind: c.Kind, Paths: paths})
		named = append(named, strings.Join(paths, ", ")+" ("+c.Kind+")")
	}
	return joinPaths(named), details
}

// openConflicts is the pending pull's conflicts not resolved yet.
func openConflicts(dir string, s *syncstate.Org) []syncstate.Conflict {
	if s == nil || s.Pending == nil {
		return nil
	}
	var open []syncstate.Conflict
	for _, c := range s.Pending.Conflicts {
		if !resolved(dir, c) {
			open = append(open, c)
		}
	}
	return open
}

// resolved says whether a conflict is resolved, from its file's content now.
func resolved(dir string, c syncstate.Conflict) bool {
	if c.Resolved {
		return true
	}
	info := toConflictInfo(c)
	if info.MarkerSize == 0 || len(c.Files) != 1 {
		return false
	}
	content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c.Files[0].Path)))
	exists := !errors.Is(err, fs.ErrNotExist)
	if err != nil && exists {
		return false
	}
	return treemerge.Resolved(info, content, exists)
}

// pull merges the org's published revision into the folder.
func (pl *puller) pull(ctx context.Context, org cloudapi.Org, configured, setOrgGiven bool) (PullResult, error) {
	org, client, err := connect(ctx, pl.dir, destination{org, configured, setOrgGiven}, pl.env, pl.p, verbPull)
	if err != nil {
		return PullResult{}, err
	}
	if err := settle(pl.dir, pl.sync, pl.state, org, verbPull); err != nil {
		return PullResult{}, err
	}
	pl.p.Step("Pulling from %s…", org)
	status, err := client.Status(ctx)
	if err != nil {
		return PullResult{}, err
	}
	ref := pl.orgRef(org)
	res := PullResult{Org: &ref, Revision: status.Revision}
	if status.PublishedBy != nil {
		res.PublishedBy = status.PublishedBy.Email
	}
	if !status.PublishedAt.IsZero() {
		at := status.PublishedAt
		res.PublishedAt = &at
	}
	s := pl.org(org)
	if s.Base == status.Revision {
		res.Message = fmt.Sprintf("This folder already has everything published to %s.", ref.shown())
		return res, nil
	}

	local, err := readFolder(pl.dir, verbPull)
	if err != nil {
		return PullResult{}, err
	}
	c, err := compare(ctx, client, status, s.Base, local)
	if err != nil {
		return PullResult{}, err
	}
	if err := c.unselected(verbPull); err != nil {
		return PullResult{}, err
	}
	if !c.pullWouldChange() {
		// The org's changes since the base, if any, are all here already:
		// the published revision is the base.
		s.Base = status.Revision
		if err := pl.sync.Save(pl.state); err != nil {
			return PullResult{}, err
		}
		res.Message = fmt.Sprintf("This folder already has everything published to %s.", ref.shown())
		return res, nil
	}
	res.Base = c.base
	noBase := c.base == ""
	dirty := gitDirty(ctx, pl.dir) // before the merge's own changes
	merged, err := pl.merge(ctx, client, org, c)
	if err != nil {
		return PullResult{}, err
	}
	res.Changes = merged.changes
	conflicts := merged.conflicts

	res.Message = fmt.Sprintf("Pulled the changes published to %s.", ref.shown())
	if res.PublishedBy != "" {
		res.Message = fmt.Sprintf("Pulled the changes %s published on %s.", res.PublishedBy, res.PublishedAt.Local().Format("2006-01-02 at 15:04"))
	}
	// Only a folder that had files could have one that differs, or one
	// deleted on purpose: a folder fresh from `anfra init` has neither.
	if noBase && len(c.local.Files()) > 0 {
		res.Hints = append(res.Hints, fmt.Sprintf("anfra has no record of this folder pulling from or publishing to %s, so where a file differs, it can't tell whose version is newer. In a git repo, `git log -- <path>` shows which is newer.", ref.shown()))
		if slices.ContainsFunc(res.Changes, func(c FileChange) bool { return c.Change == "added" }) {
			res.Hints = append(res.Hints, "The org had the files added, and this folder didn't. If you deleted any of them on purpose, delete them again.")
		}
	}
	if dirty {
		res.Hints = append(res.Hints, uncommittedHint())
	}
	if h := missingDataSources(ctx, pl.dir, client); h != "" {
		res.Hints = append(res.Hints, h)
	}
	if len(conflicts) == 0 {
		res.Hints = append(res.Hints, "Run `anfra validate` to check the merged files, then `anfra publish`.")
		return res, nil
	}
	res.Message += fmt.Sprintf(" %s to resolve.", countConflicts(conflicts))
	res.Conflicts, res.HowToResolve = describe(conflicts, noBase)
	res.Hints = append(res.Hints, conflictHints("this pull")...)
	return res, nil
}

// merged is what a merge wrote: its changes, other than conflicts, and the
// conflicts left to resolve.
type merged struct {
	changes   []FileChange
	conflicts []syncstate.Conflict
}

// merge merges the published revision of c into the folder, keeping a copy of
// every file it changes, and records the pull in the sync state: the base, or
// its conflicts left to resolve. The caller holds the folder's lock.
func (pl *puller) merge(ctx context.Context, client *cloudapi.Client, org cloudapi.Org, c comparison) (merged, error) {
	s := pl.org(org)
	s.Base = c.base // "" when the org no longer has it: merged from nothing
	status, local := c.status, c.local
	var out merged

	// Read only the content the merge needs, that the folder doesn't have.
	need := map[string]bool{}
	for _, r := range c.paths {
		switch {
		case r.Outcome == treemerge.Theirs, r.Outcome == treemerge.NeedsMerge,
			r.Conflict != nil && (r.Conflict.Kind == treemerge.ModifyDelete || r.Conflict.Kind == treemerge.DeleteModify):
			need[r.Base], need[r.Theirs] = true, true
		}
	}
	delete(need, "")
	blobs, err := client.ReadBlobs(ctx, slices.Sorted(maps.Keys(need)), status.Limits)
	if err != nil {
		return merged{}, err
	}
	reader := blobReader{local: local, fetched: blobs}

	// Markers name the sides as people know them: who published the org's
	// version, and when. Revisions are long and opaque, so they stay in the
	// result's fields.
	labels := treemerge.Labels{Ours: "local", Base: "base", Theirs: "org"}
	var by string
	if status.PublishedBy != nil {
		by = status.PublishedBy.Email
	}
	var at *time.Time
	if !status.PublishedAt.IsZero() {
		at = &status.PublishedAt
	}
	if who := publishedText(by, at); who != "" {
		labels.Theirs = "org (" + who + ")"
	}
	m, err := treemerge.Merge(ctx, c.baseTree, local.Files(), c.live, reader, treemerge.Options{Labels: labels})
	if err != nil {
		return merged{}, err
	}

	var changes []workdir.Change
	var conflicts []syncstate.Conflict
	seen := map[*treemerge.ConflictInfo]bool{}
	for _, r := range m.Paths {
		if r.Conflict != nil && !seen[r.Conflict] {
			seen[r.Conflict] = true
			conflicts = append(conflicts, fromConflictInfo(*r.Conflict))
		}
		if r.Result == r.Ours {
			continue
		}
		ch := workdir.Change{Path: r.Path, Delete: r.Result == ""}
		if !ch.Delete {
			if ch.Content, err = content(ctx, m, reader, r.Result); err != nil {
				return merged{}, err
			}
		}
		changes = append(changes, ch)
		if r.Conflict == nil {
			out.changes = append(out.changes, FileChange{Path: r.Path, Change: changeOf(r)})
		}
	}

	if len(changes) > 0 {
		err := workdir.Apply(pl.dir, pl.sync.BackupDir(), changes, func(before map[string]string) error {
			s.Pending = &syncstate.Pending{Revision: status.Revision, Stage: syncstate.Writing, Conflicts: conflicts, Before: before}
			return pl.sync.Save(pl.state)
		})
		if err != nil {
			return merged{}, err
		}
	}
	if len(conflicts) == 0 {
		s.Base, s.Pending = status.Revision, nil
		_ = os.RemoveAll(pl.sync.BackupDir())
	} else {
		if s.Pending == nil { // conflicts that wrote nothing: kept by the folder's versions
			s.Pending = &syncstate.Pending{Revision: status.Revision, Conflicts: conflicts, Before: map[string]string{}}
		}
		s.Pending.Stage = syncstate.Conflicted
	}
	if err := pl.sync.Save(pl.state); err != nil {
		return merged{}, err
	}
	out.conflicts = conflicts
	return out, nil
}

// take resolves the conflicts on o.Paths by keeping one side's version of
// their whole files.
func (pl *puller) take(ctx context.Context, org cloudapi.Org, configured bool, o PullOptions) (PullResult, error) {
	if o.Take != "org" && o.Take != "local" {
		return PullResult{}, invalid("take", "invalid", fmt.Sprintf(`--take must be "org" or "local", not %q.`, o.Take))
	}
	if len(o.Paths) == 0 {
		return PullResult{}, invalid("paths", "required", "Name the files to keep a version of, after --take.")
	}
	if !configured {
		return PullResult{}, apperr.New(clierr.OrgNotSet, "This repo has no org, so there's no pull to resolve.")
	}
	s := pl.state[org.String()]
	if s == nil || s.Pending == nil {
		return PullResult{}, clierr.Precondition("There's no pull with conflicts to resolve. Run `anfra pull` first.")
	}
	if s.Pending.Stage == syncstate.Writing {
		return PullResult{}, apperr.New(clierr.PullInterrupted, "The last pull stopped partway through writing files. Run `anfra pull --abort` to put them back as they were, then pull again.")
	}
	// The conflicts the paths name, each once.
	var picked []int
	var unknown []string
	for _, path := range o.Paths {
		i := slices.IndexFunc(s.Pending.Conflicts, func(c syncstate.Conflict) bool {
			return !resolved(pl.dir, c) && slices.ContainsFunc(c.Files, func(f syncstate.File) bool { return f.Path == path })
		})
		switch {
		case i < 0:
			unknown = append(unknown, path)
		case !slices.Contains(picked, i):
			picked = append(picked, i)
		}
	}
	if len(unknown) > 0 {
		return PullResult{}, invalid("paths", "invalid",
			fmt.Sprintf("%s %s no conflict to resolve. %s", joinPaths(unknown), hasHave(len(unknown)), stillOpen(pl.dir, s.Pending.Conflicts)))
	}

	var taken []TakenSide
	if o.Take == "local" {
		before := map[string]string{}
		for _, i := range picked {
			for _, f := range s.Pending.Conflicts[i].Files {
				if id, ok := s.Pending.Before[f.Path]; ok {
					before[f.Path] = id // the pull changed it: put it back
				}
			}
		}
		if err := workdir.Restore(pl.dir, pl.sync.BackupDir(), before); err != nil {
			return PullResult{}, err
		}
	} else {
		_, client, err := connect(ctx, pl.dir, destination{org: org, configured: true}, pl.env, pl.p, verbPull)
		if err != nil {
			return PullResult{}, err
		}
		status, err := client.Status(ctx)
		if err != nil {
			return PullResult{}, err
		}
		var ids []string
		for _, i := range picked {
			for _, f := range s.Pending.Conflicts[i].Files {
				if f.Org != "" {
					ids = append(ids, f.Org)
				}
			}
		}
		blobs, err := client.ReadBlobs(ctx, ids, status.Limits)
		if err != nil {
			return PullResult{}, err
		}
		var changes []workdir.Change
		for _, i := range picked {
			for _, f := range s.Pending.Conflicts[i].Files {
				changes = append(changes, workdir.Change{Path: f.Path, Content: blobs[f.Org], Delete: f.Org == ""})
			}
		}
		err = workdir.Apply(pl.dir, pl.sync.BackupDir(), changes, func(before map[string]string) error {
			for path, id := range before {
				if _, ok := s.Pending.Before[path]; !ok { // the version before the pull is the one to keep
					s.Pending.Before[path] = id
				}
			}
			return pl.sync.Save(pl.state)
		})
		if err != nil {
			return PullResult{}, err
		}
	}
	var paths []string
	for _, i := range picked {
		c := &s.Pending.Conflicts[i]
		c.Resolved = true
		var cp []string
		for _, f := range c.Files {
			cp = append(cp, f.Path)
		}
		paths = append(paths, cp...)
		taken = append(taken, TakenSide{Paths: cp, Kept: o.Take})
	}
	if err := pl.sync.Save(pl.state); err != nil {
		return PullResult{}, err
	}

	whose := "the org's"
	if o.Take == "local" {
		whose = "this folder's"
	}
	res := PullResult{Taken: taken, Message: fmt.Sprintf("Kept %s version of %s.", whose, joinPaths(paths))}
	var left []syncstate.Conflict
	for _, c := range s.Pending.Conflicts {
		if !resolved(pl.dir, c) {
			left = append(left, c)
		}
	}
	if len(left) == 0 {
		if err := settle(pl.dir, pl.sync, pl.state, org, verbPull); err != nil {
			return PullResult{}, err
		}
		res.Message += " Every conflict is resolved."
		res.Hints = []string{"Run `anfra validate` to check the merged files, then `anfra publish`."}
		return res, nil
	}
	res.Message += fmt.Sprintf(" %s left to resolve.", countConflicts(left))
	res.Conflicts, res.HowToResolve = describe(left, false)
	res.Hints = conflictHints("the last pull")
	return res, nil
}

// abort undoes the last pull while it isn't finished: every file it changed
// put back.
func (pl *puller) abort(org cloudapi.Org, configured bool) (PullResult, error) {
	if !configured {
		return PullResult{}, apperr.New(clierr.OrgNotSet, "This repo has no org, so there's no pull to undo.")
	}
	s := pl.state[org.String()]
	if s == nil || s.Pending == nil {
		return PullResult{Message: "There's no pull to undo: the last one finished."}, nil
	}
	if err := workdir.Restore(pl.dir, pl.sync.BackupDir(), s.Pending.Before); err != nil {
		return PullResult{}, err
	}
	restored := slices.Sorted(maps.Keys(s.Pending.Before))
	s.Pending = nil
	if err := pl.sync.Save(pl.state); err != nil {
		return PullResult{}, err
	}
	_ = os.RemoveAll(pl.sync.BackupDir())
	return PullResult{
		Message:  undoneMessage(len(restored)),
		Restored: restored,
	}, nil
}

func (pl *puller) orgRef(org cloudapi.Org) OrgRef { return orgRef(pl.env, org) }

// orgRef is org as a result names it: its address, and its name when the
// login knows it.
func orgRef(env Env, org cloudapi.Org) OrgRef {
	ref := OrgRef{URL: org.String()}
	if env.OrgName != nil {
		ref.Name = env.OrgName(org)
	}
	return ref
}

// publishedText is who published a revision, and when, as people read it:
// "bob@acme.com, 2026-11-08 16:20". "" when neither is known.
func publishedText(by string, at *time.Time) string {
	var parts []string
	if by != "" {
		parts = append(parts, by)
	}
	if at != nil {
		parts = append(parts, at.Local().Format("2006-01-02 15:04"))
	}
	return strings.Join(parts, ", ")
}

func (r OrgRef) shown() string {
	if r.Name != "" {
		return r.Name
	}
	return r.URL
}

// blobReader reads content the merge needs: the folder's own files, or what
// was read from the org.
type blobReader struct {
	local   workdir.Tree
	fetched map[string][]byte
}

func (b blobReader) ReadBlob(ctx context.Context, id string) ([]byte, error) {
	if c, ok := b.fetched[id]; ok {
		return c, nil
	}
	return b.local.ReadBlob(ctx, id)
}

// content is a merged path's content, by blob id: one the merge made, or one
// it read.
func content(ctx context.Context, m treemerge.Result, r blobReader, id string) ([]byte, error) {
	if c, ok := m.NewBlobs[id]; ok {
		return c, nil
	}
	return r.ReadBlob(ctx, id)
}

func listTree(ctx context.Context, c *cloudapi.Client, revision string) (treemerge.Tree, error) {
	files, err := c.ListFiles(ctx, revision)
	if err != nil {
		return nil, err
	}
	t := make(treemerge.Tree, len(files))
	for _, f := range files {
		t[f.Path] = f.OID
	}
	return t, nil
}

func union(trees ...treemerge.Tree) treemerge.Tree {
	out := treemerge.Tree{}
	for _, t := range trees {
		maps.Copy(out, t)
	}
	return out
}

func changeOf(r treemerge.PathResult) string {
	if r.Outcome == treemerge.Merged {
		return "merged"
	}
	return sideChange(r.Ours, r.Result)
}

func toConflictInfo(c syncstate.Conflict) treemerge.ConflictInfo {
	info := treemerge.ConflictInfo{Kind: treemerge.Kind(c.Kind), MarkerSize: c.MarkerSize}
	for _, f := range c.Files {
		info.Files = append(info.Files, treemerge.Version{Path: f.Path, Base: f.Base, Ours: f.Local, Theirs: f.Org})
	}
	return info
}

func fromConflictInfo(info treemerge.ConflictInfo) syncstate.Conflict {
	c := syncstate.Conflict{Kind: string(info.Kind), MarkerSize: info.MarkerSize}
	for _, v := range info.Files {
		c.Files = append(c.Files, syncstate.File{Path: v.Path, Base: v.Base, Local: v.Ours, Org: v.Theirs})
	}
	return c
}

// describe is conflicts as a result shows them: each one's message, by kind,
// and how to resolve each kind present.
func describe(conflicts []syncstate.Conflict, noBase bool) (map[string][]PullConflict, map[string]string) {
	byKind := map[string][]PullConflict{}
	how := map[string]string{}
	for _, c := range conflicts {
		var paths []string
		for _, f := range c.Files {
			paths = append(paths, f.Path)
		}
		byKind[c.Kind] = append(byKind[c.Kind], PullConflict{Paths: paths, Message: conflictMessage(c, noBase)})
		how[c.Kind] = howToResolve[c.Kind]
	}
	return byKind, how
}

func conflictMessage(c syncstate.Conflict, noBase bool) string {
	p := c.Files[0].Path
	switch treemerge.Kind(c.Kind) {
	case treemerge.Content:
		return fmt.Sprintf("You and the org changed the same lines of %s.", p)
	case treemerge.AddAdd:
		if noBase {
			return fmt.Sprintf("%s differs here and on the org.", p)
		}
		return fmt.Sprintf("You and the org both added %s, with different content.", p)
	case treemerge.ModifyDelete:
		return fmt.Sprintf("You changed %s, and the org deleted it.", p)
	case treemerge.DeleteModify:
		return fmt.Sprintf("You deleted %s, and the org changed it.", p)
	case treemerge.Binary:
		return fmt.Sprintf("You and the org both changed %s, a binary file.", p)
	case treemerge.FileFolder:
		var inside []string
		for _, f := range c.Files[1:] {
			inside = append(inside, strings.TrimPrefix(f.Path, p+"/"))
		}
		here, there := "this folder", "the org"
		if c.Files[0].Local == "" {
			here, there = there, here
		}
		return fmt.Sprintf("%s is a file in %s, and a folder in %s, holding %s.", p, here, there, joinPaths(inside))
	case treemerge.PathCollision:
		var all []string
		for _, f := range c.Files {
			all = append(all, f.Path)
		}
		return fmt.Sprintf("%s would be one file on systems that ignore case, such as macOS and Windows.", joinPaths(all))
	}
	return fmt.Sprintf("%s has a conflict.", p)
}

// howToResolve is how to resolve a conflict of each kind.
var howToResolve = map[string]string{
	string(treemerge.Content):       "Edit each file to keep the right changes, and remove the conflict markers.",
	string(treemerge.AddAdd):        "Edit each file to keep the right content, and remove the conflict markers.",
	string(treemerge.ModifyDelete):  "Delete the file to keep the deletion, or remove the conflict markers to keep your version.",
	string(treemerge.DeleteModify):  "Delete the file to keep your deletion, or remove the conflict markers to keep the org's version.",
	string(treemerge.Binary):        "These files can't hold conflict markers. Keep one version with --take.",
	string(treemerge.FileFolder):    "Keep one side with --take. Only --take resolves these.",
	string(treemerge.PathCollision): "Keep one side with --take. Only --take resolves these.",
}

// conflictHints are the hints for a pull's conflicts; which names the pull:
// "this pull", or "the last pull".
func conflictHints(which string) []string {
	return []string{
		"To keep one version of whole files, run `anfra pull --take local <path>...` for yours, or `--take org <path>...` for the org's. It can resolve any conflict.",
		"Once every conflict is resolved, run `anfra validate`, then `anfra publish`.",
		"To undo " + which + ", run `anfra pull --abort`.",
	}
}

// stillOpen names the files that still have conflicts.
func stillOpen(dir string, conflicts []syncstate.Conflict) string {
	var open []string
	for _, c := range conflicts {
		if !resolved(dir, c) {
			for _, f := range c.Files {
				open = append(open, f.Path)
			}
		}
	}
	if len(open) == 0 {
		return "No file has conflicts left."
	}
	return fmt.Sprintf("%s still %s conflicts.", joinPaths(open), hasHave(len(open)))
}

func countConflicts(cs []syncstate.Conflict) string {
	kinds := map[string]bool{}
	for _, c := range cs {
		kinds[c.Kind] = true
	}
	n := fmt.Sprintf("%d conflict", len(cs))
	if len(cs) != 1 {
		n += "s"
	}
	if len(kinds) > 1 {
		n += fmt.Sprintf(", of %d kinds,", len(kinds))
	}
	return n
}

func undoneMessage(n int) string {
	switch n {
	case 0:
		return "Undid the last pull. It hadn't changed any file."
	case 1:
		return "Undid the last pull. The file it changed is back as it was."
	}
	return fmt.Sprintf("Undid the last pull. The %d files it changed are back as they were.", n)
}

// joinPaths lists paths in a sentence: "a", "a and b", "a, b, and c".
func joinPaths(paths []string) string {
	switch len(paths) {
	case 0:
		return ""
	case 1:
		return paths[0]
	case 2:
		return paths[0] + " and " + paths[1]
	}
	return strings.Join(paths[:len(paths)-1], ", ") + ", and " + paths[len(paths)-1]
}

// invalid is an input refused, its message the one sentence a reader needs.
func invalid(field, code, message string) error {
	return apperr.NewWith(apperr.ValidationFailed, message, apperr.Violate(apperr.Violation{Field: field, Code: code, Message: message}))
}

func hasHave(n int) string { return pick(n, "has", "have") }
func isAre(n int) string   { return pick(n, "is", "are") }
func itThem(n int) string  { return pick(n, "it", "them") }

func pick(n int, one, more string) string {
	if n == 1 {
		return one
	}
	return more
}
