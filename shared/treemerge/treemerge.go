// Package treemerge merges three versions of a set of files: a base, and two
// sides ("ours" and "theirs") that each changed it. It answers what each path
// becomes, the merged files' content, and the conflicts it couldn't decide.
// docs/designs/treemerge.md is its design.
//
// It's pure: no file system, network, git or anfra state. Callers hand it the
// three versions as trees of blob ids, and a way to read a blob's content.
//
// git's merge is the baseline: for any input, the result is git's, or a
// conflict where git would have resolved. The text of a file both sides
// changed is merged by textmerge.
package treemerge

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/holistics/anfra/shared/repoformat"
	"github.com/holistics/anfra/shared/textmerge"
)

// Tree is a set of files: slash-separated path → blob id, git's blob id of the
// file's bytes (repoformat.BlobID).
type Tree map[string]string

// BlobReader reads a file's content by blob id.
type BlobReader interface {
	ReadBlob(ctx context.Context, id string) ([]byte, error)
}

// Outcome is what happened to a path.
type Outcome string

const (
	// Unchanged: both sides have the same version, the base's or a change
	// both made.
	Unchanged Outcome = "unchanged"
	// Ours: only ours changed it, so ours' version is kept.
	Ours Outcome = "ours"
	// Theirs: only theirs changed it, so theirs' version is taken.
	Theirs Outcome = "theirs"
	// Merged: both changed it, and the text merged cleanly.
	Merged Outcome = "merged"
	// Conflict: treemerge won't decide it. PathResult.Conflict says why.
	Conflict Outcome = "conflict"
	// NeedsMerge: both changed its content, so deciding needs the content.
	// Only Classify answers it: Merge answers Merged or Conflict instead.
	NeedsMerge Outcome = "needs_merge"
)

// Kind is why a conflict is one: a stable code callers branch on and show.
// The names follow the conflict types git's merge reports.
type Kind string

const (
	// Content: both sides changed a text file, and the changes overlap or touch.
	Content Kind = "content"
	// AddAdd: both sides added a text file at the same path, and their lines
	// conflict.
	AddAdd Kind = "add_add"
	// ModifyDelete: ours changed a file theirs deleted.
	ModifyDelete Kind = "modify_delete"
	// DeleteModify: ours deleted a file theirs changed.
	DeleteModify Kind = "delete_modify"
	// Binary: both sides changed a binary file differently.
	Binary Kind = "binary"
	// FileFolder: a path is a file in the merge, and a folder too.
	FileFolder Kind = "file_folder"
	// PathCollision: two paths in the merge would be one file on a system that
	// ignores case or Unicode form.
	PathCollision Kind = "path_collision"
)

// Version is one path's three versions, as blob ids. "" is absent.
type Version struct {
	Path               string
	Base, Ours, Theirs string
}

// ConflictInfo is a conflict as data: its kind, every path it covers with its
// three versions, and its marker size. Markers are rendered from it (Render),
// so a caller can keep it and show the conflict its own way.
type ConflictInfo struct {
	Kind  Kind
	Files []Version
	// MarkerSize is the size of the markers written into the file, for the
	// kinds a file shows with markers. 0: the conflict has no markers, and is
	// resolved only by taking a side.
	MarkerSize int
}

// PathResult is what the merge decided for one path, with its three versions.
type PathResult struct {
	Version
	Outcome Outcome
	// Conflict is set when Outcome is Conflict. Paths a conflict covers
	// together share it.
	Conflict *ConflictInfo
	// Result is the path's content after the merge, as a blob id: the
	// version kept or taken, the merged file, or the file with conflict
	// markers. "" means the path doesn't exist after the merge. A conflict
	// without markers keeps ours' version.
	Result string
}

// Result is a merge: every path in any of the three trees, sorted, and the
// content of the files the merge made (merged, or with markers), by blob id.
type Result struct {
	Paths    []PathResult
	NewBlobs map[string][]byte
	// Conflicts is how many conflicts there are. One conflict can cover
	// several paths.
	Conflicts int
}

// Labels name the sides in conflict markers: "<<<<<<< ours (local)". Empty
// labels leave the markers bare.
type Labels struct{ Ours, Base, Theirs string }

// Options are how Merge treats files and writes conflicts.
type Options struct {
	Labels Labels
	// MaxFileSize, when above 0, treats larger files as binary: never merged
	// by lines.
	MaxFileSize int
}

// binaryPrefix is how much of a file is searched for a NUL byte, as git does.
const binaryPrefix = 8000

// Classify decides each path from blob ids alone, reading no content. A path
// both sides changed differently is NeedsMerge: whether it merges cleanly
// needs its content. It answers whether a merge would change ours, cheaply
// enough to ask before every publish.
func Classify(base, ours, theirs Tree) []PathResult {
	paths := union(base, ours, theirs)
	res := make([]PathResult, 0, len(paths))
	for _, p := range paths {
		v := Version{Path: p, Base: base[p], Ours: ours[p], Theirs: theirs[p]}
		res = append(res, classify(v))
	}
	checkPaths(res)
	return res
}

func classify(v Version) PathResult {
	r := PathResult{Version: v}
	switch {
	case v.Ours == v.Theirs:
		r.Outcome, r.Result = Unchanged, v.Ours
	case v.Ours == v.Base:
		r.Outcome, r.Result = Theirs, v.Theirs
	case v.Theirs == v.Base:
		r.Outcome, r.Result = Ours, v.Ours
	case v.Ours == "":
		r.Outcome, r.Conflict = Conflict, &ConflictInfo{Kind: DeleteModify, Files: []Version{v}}
	case v.Theirs == "":
		r.Outcome, r.Result = Conflict, v.Ours
		r.Conflict = &ConflictInfo{Kind: ModifyDelete, Files: []Version{v}}
	default:
		r.Outcome, r.Result = NeedsMerge, v.Ours
	}
	return r
}

// Merge merges the three trees. Content is read only for paths both sides
// changed.
func Merge(ctx context.Context, base, ours, theirs Tree, blobs BlobReader, o Options) (Result, error) {
	res := Result{Paths: Classify(base, ours, theirs), NewBlobs: map[string][]byte{}}
	for i := range res.Paths {
		r := &res.Paths[i]
		switch {
		case r.Outcome == NeedsMerge:
			if err := mergeContent(ctx, r, blobs, o, res.NewBlobs); err != nil {
				return Result{}, err
			}
		case r.Conflict != nil && (r.Conflict.Kind == ModifyDelete || r.Conflict.Kind == DeleteModify):
			if err := markWhole(ctx, r, blobs, o, res.NewBlobs); err != nil {
				return Result{}, err
			}
		}
	}
	seen := map[*ConflictInfo]bool{}
	for _, r := range res.Paths {
		if r.Conflict != nil && !seen[r.Conflict] {
			seen[r.Conflict] = true
			res.Conflicts++
		}
	}
	return res, nil
}

// mergeContent merges a file both sides changed: binary files conflict, and
// text is merged by lines.
func mergeContent(ctx context.Context, r *PathResult, blobs BlobReader, o Options, newBlobs map[string][]byte) error {
	b, ou, th, err := readVersions(ctx, blobs, r.Version)
	if err != nil {
		return err
	}
	r.Outcome = Conflict
	if isBinary(o, b, ou, th) {
		r.Conflict = &ConflictInfo{Kind: Binary, Files: []Version{r.Version}}
		return nil // ours' version stays
	}
	m := textmerge.Merge(b, ou, th)
	if m.Conflicts == 0 {
		r.Outcome = Merged
		r.Result = put(newBlobs, m.Render(textmerge.Options{}))
		return nil
	}
	kind := Content
	if r.Base == "" {
		kind = AddAdd
	}
	size := textmerge.MarkerSize(b, ou, th)
	r.Conflict = &ConflictInfo{Kind: kind, Files: []Version{r.Version}, MarkerSize: size}
	r.Result = put(newBlobs, m.Render(markerOptions(o.Labels, size)))
	return nil
}

// markWhole writes a modify/delete conflict into the file: the whole file
// between markers, the deleted side empty, so the member sees what the other
// side changed. Binary files get no markers: ours' version stays, a file or
// none.
func markWhole(ctx context.Context, r *PathResult, blobs BlobReader, o Options, newBlobs map[string][]byte) error {
	b, ou, th, err := readVersions(ctx, blobs, r.Version)
	if err != nil {
		return err
	}
	if isBinary(o, b, ou, th) {
		r.Result = r.Ours
		return nil
	}
	size := textmerge.MarkerSize(b, ou, th)
	r.Conflict.MarkerSize = size
	r.Result = put(newBlobs, textmerge.Whole(b, ou, th).Render(markerOptions(o.Labels, size)))
	return nil
}

// Render writes a conflict's file with markers, from its blob ids alone: the
// same bytes Merge wrote. Only conflicts with markers have a rendering.
func Render(ctx context.Context, c ConflictInfo, blobs BlobReader, o Options) ([]byte, error) {
	if c.MarkerSize == 0 || len(c.Files) != 1 {
		return nil, fmt.Errorf("a %s conflict has no markers to render", c.Kind)
	}
	b, ou, th, err := readVersions(ctx, blobs, c.Files[0])
	if err != nil {
		return nil, err
	}
	opts := markerOptions(o.Labels, c.MarkerSize)
	switch c.Kind {
	case ModifyDelete, DeleteModify:
		return textmerge.Whole(b, ou, th).Render(opts), nil
	default:
		return textmerge.Merge(b, ou, th).Render(opts), nil
	}
}

// Resolved says whether a conflict is resolved, from its file's current
// content. A file deleted is resolved, as a deletion. A file still holding a
// marker-like line of the conflict's marker size isn't. A conflict without
// markers is resolved only by taking a side.
func Resolved(c ConflictInfo, current []byte, exists bool) bool {
	if c.MarkerSize == 0 {
		return false
	}
	if !exists {
		return true
	}
	return textmerge.Resolved(current, c.MarkerSize)
}

// Side is a side of a merge, to take.
type Side int

const (
	TakeOurs Side = iota + 1
	TakeTheirs
)

// Change is a path's content after taking a side: a blob id, or "" to delete
// the path.
type Change struct {
	Path string
	Blob string
}

// Take resolves a conflict by taking one side's version of every path it
// covers, as `git checkout --ours` and `--theirs` do. An absent version means
// the path is deleted.
func Take(c ConflictInfo, side Side) []Change {
	out := make([]Change, 0, len(c.Files))
	for _, f := range c.Files {
		id := f.Ours
		if side == TakeTheirs {
			id = f.Theirs
		}
		out = append(out, Change{Path: f.Path, Blob: id})
	}
	return out
}

func markerOptions(l Labels, size int) textmerge.Options {
	return textmerge.Options{Ours: l.Ours, Base: l.Base, Theirs: l.Theirs, MarkerSize: size}
}

// isBinary says whether a file is never to be merged by lines: a NUL byte in
// a version's first 8000 bytes, as git checks, a version that isn't UTF-8,
// since the repo's files are text, or one larger than MaxFileSize.
func isBinary(o Options, versions ...[]byte) bool {
	for _, v := range versions {
		if o.MaxFileSize > 0 && len(v) > o.MaxFileSize {
			return true
		}
		if bytes.IndexByte(v[:min(len(v), binaryPrefix)], 0) >= 0 || !utf8.Valid(v) {
			return true
		}
	}
	return false
}

func readVersions(ctx context.Context, blobs BlobReader, v Version) (base, ours, theirs []byte, err error) {
	read := func(id string) ([]byte, error) {
		if id == "" {
			return nil, nil
		}
		b, err := blobs.ReadBlob(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("read %s's blob %s: %w", v.Path, id, err)
		}
		return b, nil
	}
	if base, err = read(v.Base); err != nil {
		return nil, nil, nil, err
	}
	if ours, err = read(v.Ours); err != nil {
		return nil, nil, nil, err
	}
	if theirs, err = read(v.Theirs); err != nil {
		return nil, nil, nil, err
	}
	return base, ours, theirs, nil
}

// put keeps content the merge made, and answers its blob id.
func put(newBlobs map[string][]byte, content []byte) string {
	id := repoformat.BlobID(content)
	newBlobs[id] = content
	return id
}

func union(trees ...Tree) []string {
	set := map[string]bool{}
	for _, t := range trees {
		for p := range t {
			set[p] = true
		}
	}
	paths := make([]string, 0, len(set))
	for p := range set {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	return paths
}

// checkPaths finds paths that can't all be written after the merge: a path
// that's a file and also a folder of other paths, and paths that are one file
// on a system ignoring case or Unicode form. Each group becomes one conflict
// over all its paths, keeping ours' versions. It looks only at paths that
// exist after the merge, so a clash one side already resolved, by deleting,
// isn't one.
func checkPaths(res []PathResult) {
	// A path exists after the merge if it has content, will once merged, or
	// will hold markers: a file ours deleted and theirs changed is written
	// back, between markers.
	exists := func(r PathResult) bool {
		return r.Result != "" || r.Outcome == NeedsMerge || (r.Conflict != nil && r.Conflict.Kind == DeleteModify)
	}
	var live []int // indexes into res, of paths that exist after the merge
	for i, r := range res {
		if exists(r) {
			live = append(live, i)
		}
	}
	mark := func(kind Kind, group []int) {
		c := &ConflictInfo{Kind: kind}
		for _, i := range group {
			c.Files = append(c.Files, res[i].Version)
		}
		for _, i := range group {
			res[i].Outcome, res[i].Conflict, res[i].Result = Conflict, c, res[i].Ours
		}
	}

	// A file that's also a folder: res is sorted, so a path's descendants
	// follow it, after any siblings sorting between ("a.aml" < "a/").
	taken := map[int]bool{}
	for k, i := range live {
		prefix := res[i].Path + "/"
		group := []int{i}
		for _, j := range live[k+1:] {
			if strings.HasPrefix(res[j].Path, prefix) {
				group = append(group, j)
			}
		}
		if len(group) > 1 && !taken[i] {
			for _, g := range group {
				taken[g] = true
			}
			mark(FileFolder, group)
		}
	}

	// Paths that are one file ignoring case and Unicode form.
	fold := cases.Fold()
	byKey := map[string][]int{}
	var keys []string
	for _, i := range live {
		key := fold.String(norm.NFC.String(res[i].Path))
		if byKey[key] == nil {
			keys = append(keys, key)
		}
		byKey[key] = append(byKey[key], i)
	}
	for _, key := range keys {
		if group := byKey[key]; len(group) > 1 {
			mark(PathCollision, group)
		}
	}
}
