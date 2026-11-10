package textmerge_test

import (
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/textmerge"
)

var labels = textmerge.Options{Ours: "ours", Base: "base", Theirs: "theirs"}

func merge(t *testing.T, base, ours, theirs string) (string, int) {
	t.Helper()
	r := textmerge.Merge([]byte(base), []byte(ours), []byte(theirs))
	return string(r.Render(labels)), r.Conflicts
}

func TestMerge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		base, ours, theirs string
		want               string
		conflicts          int
	}{
		{
			name: "nothing changed",
			base: "a\nb\n", ours: "a\nb\n", theirs: "a\nb\n",
			want: "a\nb\n",
		},
		{
			name: "only ours changed",
			base: "a\nb\nc\n", ours: "a\nB\nc\n", theirs: "a\nb\nc\n",
			want: "a\nB\nc\n",
		},
		{
			name: "only theirs changed",
			base: "a\nb\nc\n", ours: "a\nb\nc\n", theirs: "a\nb\nC\n",
			want: "a\nb\nC\n",
		},
		{
			name: "changes apart, with an unchanged line between",
			base: "a\nb\nc\nd\ne\n", ours: "a\nB\nc\nd\ne\n", theirs: "a\nb\nc\nD\ne\n",
			want: "a\nB\nc\nD\ne\n",
		},
		{
			name: "the same change on both sides is taken once",
			base: "a\nb\nc\n", ours: "a\nB\nc\n", theirs: "a\nB\nc\n",
			want: "a\nB\nc\n",
		},
		{
			name: "changes that touch conflict",
			base: "a\nb\nc\nd\n", ours: "a\nB\nc\nd\n", theirs: "a\nb\nC\nd\n",
			want:      "a\n<<<<<<< ours\nB\nc\n||||||| base\nb\nc\n=======\nb\nC\n>>>>>>> theirs\nd\n",
			conflicts: 1,
		},
		{
			name: "the same line changed differently",
			base: "a\nb\nc\n", ours: "a\nB1\nc\n", theirs: "a\nB2\nc\n",
			want:      "a\n<<<<<<< ours\nB1\n||||||| base\nb\n=======\nB2\n>>>>>>> theirs\nc\n",
			conflicts: 1,
		},
		{
			name: "deleted on one side, changed on the other",
			base: "a\nb\nc\n", ours: "a\nc\n", theirs: "a\nB\nc\n",
			want:      "a\n<<<<<<< ours\n||||||| base\nb\n=======\nB\n>>>>>>> theirs\nc\n",
			conflicts: 1,
		},
		{
			name: "both sides inserting different lines at one place",
			base: "a\nc\n", ours: "a\nb1\nc\n", theirs: "a\nb2\nc\n",
			want:      "a\n<<<<<<< ours\nb1\n||||||| base\n=======\nb2\n>>>>>>> theirs\nc\n",
			conflicts: 1,
		},
		{
			name: "added on both sides, from an empty base",
			base: "", ours: "x\n", theirs: "y\n",
			want:      "<<<<<<< ours\nx\n||||||| base\n=======\ny\n>>>>>>> theirs\n",
			conflicts: 1,
		},
		{
			name: "two conflicts, kept apart by an unchanged line",
			base: "a\nb\nc\n", ours: "A1\nb\nC1\n", theirs: "A2\nb\nC2\n",
			want:      "<<<<<<< ours\nA1\n||||||| base\na\n=======\nA2\n>>>>>>> theirs\nb\n<<<<<<< ours\nC1\n||||||| base\nc\n=======\nC2\n>>>>>>> theirs\n",
			conflicts: 2,
		},
		{
			name: "a last line without an ending, changed on one side",
			base: "a\nb", ours: "a\nB", theirs: "a\nb",
			want: "a\nB",
		},
		{
			name: "a last line without an ending, in a conflict, gets one before the next marker",
			base: "a\nb", ours: "a\nB1", theirs: "a\nB2",
			want:      "a\n<<<<<<< ours\nB1\n||||||| base\nb\n=======\nB2\n>>>>>>> theirs\n",
			conflicts: 1,
		},
		{
			name: "CRLF files get CRLF markers",
			base: "a\r\nb\r\nc\r\n", ours: "a\r\nB1\r\nc\r\n", theirs: "a\r\nB2\r\nc\r\n",
			want:      "a\r\n<<<<<<< ours\r\nB1\r\n||||||| base\r\nb\r\n=======\r\nB2\r\n>>>>>>> theirs\r\nc\r\n",
			conflicts: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, conflicts := merge(t, tt.base, tt.ours, tt.theirs)
			if got != tt.want || conflicts != tt.conflicts {
				t.Errorf("got %d conflicts:\n%q\nwant %d:\n%q", conflicts, got, tt.conflicts, tt.want)
			}
		})
	}
}

// A merge that takes everything from one side gives back that side's bytes.
func TestMergeKeepsBytes(t *testing.T) {
	t.Parallel()
	for _, side := range []string{"", "a", "a\r\nb\n", "a\n\n\nb", "\x00\xff\n"} {
		if got, _ := merge(t, "", side, ""); got != side {
			t.Errorf("ours only: got %q, want %q", got, side)
		}
		if got, _ := merge(t, "", "", side); got != side {
			t.Errorf("theirs only: got %q, want %q", got, side)
		}
	}
}

func TestMarkerSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		versions []string
		want     int
	}{
		{[]string{"a\n", "b\n"}, 7},
		{[]string{"======\n"}, 7},          // shorter than a marker: no confusion
		{[]string{"a\n", "=======\n"}, 11}, // a marker-like line: longer markers
		{[]string{"<<<<<<<<<< x\n", "|||||||\n"}, 14},
	}
	for _, tt := range tests {
		var vs [][]byte
		for _, v := range tt.versions {
			vs = append(vs, []byte(v))
		}
		if got := textmerge.MarkerSize(vs...); got != tt.want {
			t.Errorf("MarkerSize(%q) = %d, want %d", tt.versions, got, tt.want)
		}
	}

	// A conflict rendered with that size parses back as unresolved, and the
	// content's own marker-like line doesn't.
	base, ours, theirs := "=======\na\n", "=======\nA1\n", "=======\nA2\n"
	size := textmerge.MarkerSize([]byte(base), []byte(ours), []byte(theirs))
	r := textmerge.Merge([]byte(base), []byte(ours), []byte(theirs))
	out := r.Render(textmerge.Options{MarkerSize: size})
	if !strings.Contains(string(out), strings.Repeat("<", size)+"\n") {
		t.Errorf("markers aren't %d long:\n%s", size, out)
	}
	if textmerge.Resolved(out, size) {
		t.Error("a rendered conflict counts as resolved")
	}
	if !textmerge.Resolved([]byte(ours), size) {
		t.Error("content's own marker-like line counts as unresolved")
	}
}

func TestResolved(t *testing.T) {
	t.Parallel()
	tests := []struct {
		content string
		want    bool
	}{
		{"a\nb\n", true},
		{"<<<<<<< ours\n", false},
		{"=======\n", false},
		{"=======", false},             // the last line, no ending
		{"=======\r\n", false},         // CRLF
		{">>>>>>>>>> longer\n", false}, // a longer run counts too
		{"||||||| base\n", false},
		{"<<<<<<<x\n", true},    // not followed by a space or the line's end
		{"<<<<<< six\n", true},  // shorter than the size
		{"a <<<<<<< b\n", true}, // not at the line's start
	}
	for _, tt := range tests {
		if got := textmerge.Resolved([]byte(tt.content), 7); got != tt.want {
			t.Errorf("Resolved(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}
