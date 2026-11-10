package treemerge_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/holistics/anfra/shared/repoformat"
	"github.com/holistics/anfra/shared/treemerge"
)

// store is blobs in memory, by id: what a caller's BlobReader would read.
type store map[string][]byte

func (s store) ReadBlob(_ context.Context, id string) ([]byte, error) {
	b, ok := s[id]
	if !ok {
		return nil, fmt.Errorf("no blob %s", id)
	}
	return b, nil
}

// tree adds files to s, and answers them as a tree.
func (s store) tree(files map[string]string) treemerge.Tree {
	t := treemerge.Tree{}
	for p, content := range files {
		id := repoformat.BlobID([]byte(content))
		s[id] = []byte(content)
		t[p] = id
	}
	return t
}

var labels = treemerge.Options{Labels: treemerge.Labels{Ours: "ours", Base: "base", Theirs: "theirs"}}

func merge(t *testing.T, s store, base, ours, theirs map[string]string) treemerge.Result {
	t.Helper()
	r, err := treemerge.Merge(context.Background(), s.tree(base), s.tree(ours), s.tree(theirs), s, labels)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// content is a path's content after the merge: what Result names.
func content(t *testing.T, s store, r treemerge.Result, p treemerge.PathResult) string {
	t.Helper()
	if p.Result == "" {
		return "<absent>"
	}
	if b, ok := r.NewBlobs[p.Result]; ok {
		return string(b)
	}
	return string(s[p.Result])
}

func TestClassify(t *testing.T) {
	t.Parallel()
	const x, y, z = "x\n", "y\n", "z\n"
	tests := []struct {
		name               string
		base, ours, theirs map[string]string
		outcome            treemerge.Outcome
		kind               treemerge.Kind
		want               string
	}{
		{"unchanged", m(x), m(x), m(x), treemerge.Unchanged, "", x},
		{"both made the same change", m(x), m(y), m(y), treemerge.Unchanged, "", y},
		{"both deleted", m(x), nil, nil, treemerge.Unchanged, "", "<absent>"},
		{"only theirs changed", m(x), m(x), m(y), treemerge.Theirs, "", y},
		{"only theirs deleted", m(x), m(x), nil, treemerge.Theirs, "", "<absent>"},
		{"only theirs added", nil, nil, m(y), treemerge.Theirs, "", y},
		{"only ours changed", m(x), m(y), m(x), treemerge.Ours, "", y},
		{"only ours deleted", m(x), nil, m(x), treemerge.Ours, "", "<absent>"},
		{"only ours added", nil, m(y), nil, treemerge.Ours, "", y},
		{"ours deleted, theirs changed", m(x), nil, m(y), treemerge.Conflict, treemerge.DeleteModify,
			"<<<<<<< ours\n||||||| base\nx\n=======\ny\n>>>>>>> theirs\n"},
		{"ours changed, theirs deleted", m(x), m(y), nil, treemerge.Conflict, treemerge.ModifyDelete,
			"<<<<<<< ours\ny\n||||||| base\nx\n=======\n>>>>>>> theirs\n"},
		{"both changed, cleanly", m("a\nb\nc\n"), m("A\nb\nc\n"), m("a\nb\nC\n"), treemerge.Merged, "", "A\nb\nC\n"},
		{"both changed the same line", m(x), m(y), m(z), treemerge.Conflict, treemerge.Content,
			"<<<<<<< ours\ny\n||||||| base\nx\n=======\nz\n>>>>>>> theirs\n"},
		{"both added, differently", nil, m(y), m(z), treemerge.Conflict, treemerge.AddAdd,
			"<<<<<<< ours\ny\n||||||| base\n=======\nz\n>>>>>>> theirs\n"},
		{"both changed a binary file", m("\x00a"), m("\x00b"), m("\x00c"), treemerge.Conflict, treemerge.Binary, "\x00b"},
		{"invalid UTF-8 is binary", m("\xffa"), m("\xffb"), m("\xffc"), treemerge.Conflict, treemerge.Binary, "\xffb"},
		{"binary, ours deleted: stays deleted", m("\x00a"), nil, m("\x00b"), treemerge.Conflict, treemerge.DeleteModify, "<absent>"},
		{"binary, theirs deleted: ours stays", m("\x00a"), m("\x00b"), nil, treemerge.Conflict, treemerge.ModifyDelete, "\x00b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := store{}
			r := merge(t, s, tt.base, tt.ours, tt.theirs)
			if len(r.Paths) != 1 {
				t.Fatalf("got %d paths, want 1", len(r.Paths))
			}
			p := r.Paths[0]
			if p.Outcome != tt.outcome {
				t.Errorf("outcome %q, want %q", p.Outcome, tt.outcome)
			}
			if kind := kindOf(p); kind != tt.kind {
				t.Errorf("kind %q, want %q", kind, tt.kind)
			}
			if got := content(t, s, r, p); got != tt.want {
				t.Errorf("content %q, want %q", got, tt.want)
			}
		})
	}
}

// Classify reads nothing: paths both sides changed are left for the merge.
func TestClassifyReadsNoContent(t *testing.T) {
	t.Parallel()
	got := treemerge.Classify(
		treemerge.Tree{"a.aml": "1", "b.aml": "1"},
		treemerge.Tree{"a.aml": "2", "b.aml": "1"},
		treemerge.Tree{"a.aml": "3", "b.aml": "2"},
	)
	want := map[string]treemerge.Outcome{"a.aml": treemerge.NeedsMerge, "b.aml": treemerge.Theirs}
	for _, p := range got {
		if p.Outcome != want[p.Path] {
			t.Errorf("%s: %q, want %q", p.Path, p.Outcome, want[p.Path])
		}
	}
}

func TestFileFolder(t *testing.T) {
	t.Parallel()
	s := store{}
	r := merge(t, s, nil, m2("apps/report", "a\n"), m2("apps/report/index.html", "b\n"))
	if r.Conflicts != 1 {
		t.Fatalf("%d conflicts, want 1", r.Conflicts)
	}
	for _, p := range r.Paths {
		if kindOf(p) != treemerge.FileFolder {
			t.Errorf("%s: kind %q, want file_folder", p.Path, kindOf(p))
		}
	}
	// Ours' version stays: the file, and not theirs' file inside the folder.
	if got := content(t, s, r, r.Paths[0]); got != "a\n" {
		t.Errorf("%s: %q", r.Paths[0].Path, got)
	}
	if r.Paths[1].Result != "" {
		t.Errorf("%s written", r.Paths[1].Path)
	}
	// A clash one side resolved, by deleting the file, isn't one.
	r = merge(t, store{}, m2("apps/report", "a\n"), m2("apps/report", "a\n"), m2("apps/report/index.html", "b\n"))
	if r.Conflicts != 0 {
		t.Errorf("%d conflicts, want 0", r.Conflicts)
	}
}

func TestPathCollision(t *testing.T) {
	t.Parallel()
	for _, pair := range [][2]string{
		{"models/Orders.aml", "models/orders.aml"},
		{"models/café.aml", "models/café.aml"}, // NFC and NFD
	} {
		r := merge(t, store{}, nil, m2(pair[0], "a\n"), m2(pair[1], "b\n"))
		if r.Conflicts != 1 || kindOf(r.Paths[0]) != treemerge.PathCollision || r.Paths[0].Conflict != r.Paths[1].Conflict {
			t.Errorf("%q: %d conflicts, kinds %q", pair, r.Conflicts, []treemerge.Kind{kindOf(r.Paths[0]), kindOf(r.Paths[1])})
		}
	}
}

// What Merge wrote, Render writes again from the conflict alone, and the
// file's state decides resolution.
func TestRenderResolvedTake(t *testing.T) {
	t.Parallel()
	s := store{}
	r := merge(t, s, m("=======\nx\n"), m("=======\ny\n"), m("=======\nz\n"))
	p := r.Paths[0]
	c := *p.Conflict
	if c.MarkerSize != 11 {
		t.Errorf("marker size %d, want 11: the file has a marker-like line", c.MarkerSize)
	}
	written := r.NewBlobs[p.Result]
	again, err := treemerge.Render(context.Background(), c, s, labels)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(written) {
		t.Errorf("Render gave\n%q\nMerge wrote\n%q", again, written)
	}
	if treemerge.Resolved(c, written, true) {
		t.Error("the file as written counts as resolved")
	}
	if !treemerge.Resolved(c, []byte("=======\ny\n"), true) {
		t.Error("markers removed, the content's own marker-like line still counts")
	}
	if !treemerge.Resolved(c, nil, false) {
		t.Error("a deleted file isn't resolved")
	}
	if got := treemerge.Take(c, treemerge.TakeTheirs); len(got) != 1 || got[0].Blob != c.Files[0].Theirs {
		t.Errorf("Take theirs: %+v", got)
	}

	// Without markers, only taking a side resolves.
	r = merge(t, s, m("\x00a"), m("\x00b"), m("\x00c"))
	bin := *r.Paths[0].Conflict
	if treemerge.Resolved(bin, nil, false) || treemerge.Resolved(bin, []byte("x"), true) {
		t.Error("a binary conflict resolved without taking a side")
	}
	if _, err := treemerge.Render(context.Background(), bin, s, labels); err == nil {
		t.Error("a binary conflict rendered")
	}
}

// The same merge gives the same result, whatever the order maps iterate in.
func TestDeterministic(t *testing.T) {
	t.Parallel()
	files := map[string]string{}
	for i := range 50 {
		files[fmt.Sprintf("models/m%02d.aml", i)] = fmt.Sprintf("model %d\n", i)
	}
	first := merge(t, store{}, nil, files, nil)
	for range 5 {
		again := merge(t, store{}, nil, files, nil)
		if !slices.EqualFunc(first.Paths, again.Paths, func(a, b treemerge.PathResult) bool {
			return a.Path == b.Path && a.Result == b.Result && a.Outcome == b.Outcome
		}) {
			t.Fatal("results differ between runs")
		}
	}
}

func m(content string) map[string]string { return map[string]string{"f.aml": content} }

func m2(path, content string) map[string]string { return map[string]string{path: content} }

func kindOf(p treemerge.PathResult) treemerge.Kind {
	if p.Conflict == nil {
		return ""
	}
	return p.Conflict.Kind
}
