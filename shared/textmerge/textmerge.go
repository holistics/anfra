// Package textmerge merges three versions of one file's text: a base, and two
// sides ("ours" and "theirs") that each changed it. It's the text merger behind
// treemerge (docs/designs/treemerge.md), and usable alone, by anything merging
// a single file.
//
// Its behaviour follows git's: `git merge-file --diff3`, at what git calls its
// "eager" level, is the reference. A change only one side made is taken. The
// same change on both sides is taken once. Changes that overlap, or touch with
// no unchanged line between them, are a conflict. git's code is not used: the
// rules are written here from its documented behaviour, and checked against
// git by the tests.
//
// Text is handled as bytes, split into lines after each "\n", each line
// keeping its ending. Nothing is decoded or normalised, so a merge that takes
// every line from one side gives back that side's bytes exactly.
package textmerge

import (
	"bytes"
	"strings"
)

// Hunk is a stretch of a merged file: clean lines, or a conflict.
type Hunk struct {
	// Conflict says which kind of hunk this is.
	Conflict bool
	// Lines are a clean hunk's lines, each with its line ending.
	Lines []string
	// Base, Ours and Theirs are a conflict's three versions of the stretch.
	// Any may be empty: a side that deleted it, or a base that had nothing
	// where both sides added lines.
	Base, Ours, Theirs []string
	// CRLF says whether a conflict's markers end their lines with "\r\n",
	// decided from the lines around it, as git decides.
	CRLF bool
}

// Result is a merge: its hunks, in order, and how many of them conflict.
type Result struct {
	Hunks     []Hunk
	Conflicts int
}

// Merge merges ours and theirs, two changed versions of base.
func Merge(base, ours, theirs []byte) Result {
	b, o, t := splitLines(base), splitLines(ours), splitLines(theirs)
	recs := combine(b, changes(b, o), changes(b, t), o, t)
	return assemble(b, o, t, recs)
}

// A change is one side's replacement of base[b0:b1] by side[s0:s1]: a
// stretch the side changed, deleted (s0 == s1), or inserted at (b0 == b1).
type change struct{ b0, b1, s0, s1 int }

func (c change) delta() int { return (c.s1 - c.s0) - (c.b1 - c.b0) }

// What a record takes, as git's merge records do.
type mode int

const (
	conflict   mode = iota // both sides, between markers
	takeOurs               // ours' lines
	takeTheirs             // theirs' lines
)

// A record is a stretch of the base, and the matching stretches of ours and
// theirs, that the merge decided on. Stretches between records are unchanged
// on both sides.
type record struct {
	mode           mode
	b0, b1, o0, o1 int
	t0, t1         int
}

// combine walks both sides' changes in the base's order, and decides each.
// dOurs and dTheirs are how far each side's positions have moved from the
// base's, from the changes passed so far: a position p of the base that
// neither side changed is p+dOurs in ours, and p+dTheirs in theirs.
func combine(base []string, co, ct []change, ours, theirs []string) []record {
	var recs []record
	add := func(r record) {
		// A record that overlaps or touches the previous one joins it. Joining
		// different modes, such as ours' change next to theirs', is a conflict:
		// two changes with no unchanged line between them.
		if n := len(recs); n > 0 && r.b0 <= recs[n-1].b1 {
			p := &recs[n-1]
			if p.mode != r.mode {
				p.mode = conflict
			}
			// The joined record ends where the later one does. The earlier one's
			// end can overshoot: a conflict over a stretch one side changed
			// assumes the other side's lines beyond its own change are the
			// base's, and the later record is that other change, ending right.
			p.b1, p.o1, p.t1 = r.b1, r.o1, r.t1
			return
		}
		recs = append(recs, r)
	}
	dOurs, dTheirs := 0, 0
	i, j := 0, 0
	for i < len(co) || j < len(ct) {
		switch {
		case j == len(ct) || (i < len(co) && co[i].b1 < ct[j].b0):
			// Only ours changed this stretch.
			c := co[i]
			add(record{takeOurs, c.b0, c.b1, c.s0, c.s1, c.b0 + dTheirs, c.b1 + dTheirs})
			dOurs += c.delta()
			i++
		case i == len(co) || ct[j].b1 < co[i].b0:
			// Only theirs changed this stretch.
			c := ct[j]
			add(record{takeTheirs, c.b0, c.b1, c.b0 + dOurs, c.b1 + dOurs, c.s0, c.s1})
			dTheirs += c.delta()
			j++
		default:
			// The changes overlap or touch.
			x, y := co[i], ct[j]
			same := x.b0 == y.b0 && x.b1 == y.b1 && equalLines(ours[x.s0:x.s1], theirs[y.s0:y.s1])
			if !same {
				// A conflict over both stretches. Where one change reaches
				// beyond the other, the other side has the base's lines there.
				b0, b1 := min(x.b0, y.b0), max(x.b1, y.b1)
				add(record{
					mode: conflict, b0: b0, b1: b1,
					o0: x.s0 - (x.b0 - b0), o1: x.s1 + (b1 - x.b1),
					t0: y.s0 - (y.b0 - b0), t1: y.s1 + (b1 - y.b1),
				})
			}
			// The same change on both sides adds no record: the merge takes
			// it from ours, as it takes every stretch without a record.
			if x.b1 >= y.b1 {
				dTheirs += y.delta()
				j++
			}
			if y.b1 >= x.b1 {
				dOurs += x.delta()
				i++
			}
		}
	}
	return recs
}

// assemble turns the records into hunks. The merged file is ours, with each
// record's stretch replaced: by theirs' lines, or by a conflict.
func assemble(base, ours, theirs []string, recs []record) Result {
	var res Result
	var clean []string
	flush := func() {
		if len(clean) > 0 {
			res.Hunks = append(res.Hunks, Hunk{Lines: clean})
			clean = nil
		}
	}
	at := 0 // the next line of ours to copy
	for _, r := range recs {
		clean = append(clean, ours[at:r.o0]...)
		switch r.mode {
		case takeOurs:
			clean = append(clean, ours[r.o0:r.o1]...)
		case takeTheirs:
			clean = append(clean, theirs[r.t0:r.t1]...)
		case conflict:
			flush()
			res.Hunks = append(res.Hunks, Hunk{
				Conflict: true,
				Base:     base[r.b0:r.b1],
				Ours:     ours[r.o0:r.o1],
				Theirs:   theirs[r.t0:r.t1],
				CRLF:     crlfNeeded(base, ours, theirs, r),
			})
			res.Conflicts++
		}
		at = r.o1
	}
	clean = append(clean, ours[at:]...)
	flush()
	return res
}

// crlfNeeded decides whether a conflict's markers end in "\r\n", as git does:
// when the line before the conflict (or the first line) uses CRLF in ours and
// in theirs, and the base's first line does too. Undecided, it's "\n".
func crlfNeeded(base, ours, theirs []string, r record) bool {
	before := func(n int) int {
		if n > 0 {
			return n - 1
		}
		return 0
	}
	// Each check that finds "\n" settles it. Otherwise the next one decides,
	// and the base's first line decides last.
	if eolOf(ours, before(r.o0)) == lf || eolOf(theirs, before(r.t0)) == lf {
		return false
	}
	return eolOf(base, 0) == crlf
}

type eol int

const (
	undecided eol = iota
	lf
	crlf
)

// eolOf is the line ending of lines[i]. The last line, without one, takes the
// line before it. A file with no lines, or one line without an ending, is
// undecided.
func eolOf(lines []string, i int) eol {
	switch {
	case len(lines) == 0:
		return undecided
	case i < len(lines)-1:
		return endingOf(lines[i])
	case strings.HasSuffix(lines[i], "\n"):
		return endingOf(lines[i])
	case i == 0:
		return undecided
	default:
		return endingOf(lines[i-1])
	}
}

func endingOf(line string) eol {
	if strings.HasSuffix(line, "\r\n") {
		return crlf
	}
	return lf
}

// Options are how conflicts are written.
type Options struct {
	// Labels follow each marker's characters: "<<<<<<< ours (local)". Empty,
	// the marker stands alone.
	Ours, Base, Theirs string
	// MarkerSize is how many characters each marker has: 7 when 0. Use
	// MarkerSize to pick one that can't be confused with the content.
	MarkerSize int
}

// Render writes a merge as text: clean hunks as they are, and conflicts
// between git's diff3 markers.
func (r Result) Render(o Options) []byte {
	size := o.MarkerSize
	if size <= 0 {
		size = DefaultMarkerSize
	}
	var buf bytes.Buffer
	for _, h := range r.Hunks {
		if !h.Conflict {
			for _, l := range h.Lines {
				buf.WriteString(l)
			}
			continue
		}
		nl := "\n"
		if h.CRLF {
			nl = "\r\n"
		}
		marker := func(c byte, label string) {
			buf.WriteString(strings.Repeat(string(c), size))
			if label != "" {
				buf.WriteByte(' ')
				buf.WriteString(label)
			}
			buf.WriteString(nl)
		}
		section := func(lines []string) {
			for _, l := range lines {
				buf.WriteString(l)
			}
			// A section whose last line has no ending gets one, so the next
			// marker starts its own line. That the side had none is lost, as
			// in git.
			if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
				buf.WriteString(nl)
			}
		}
		marker('<', o.Ours)
		section(h.Ours)
		marker('|', o.Base)
		section(h.Base)
		marker('=', "")
		section(h.Theirs)
		marker('>', o.Theirs)
	}
	return buf.Bytes()
}

// DefaultMarkerSize is git's marker size.
const DefaultMarkerSize = 7

// MarkerSize is the marker size for a conflict in these versions: 7, unless a
// line in one of them already starts with 7 or more of the same marker
// character, in which case it's that run's length + 4 (jj's rule), so no
// line of content can be mistaken for a marker. git takes the same size per
// path, as its conflict-marker-size attribute.
func MarkerSize(versions ...[]byte) int {
	longest := 0
	for _, v := range versions {
		for _, line := range splitLines(v) {
			if n := markerRun(line); n >= DefaultMarkerSize {
				longest = max(longest, n)
			}
		}
	}
	if longest == 0 {
		return DefaultMarkerSize
	}
	return longest + 4
}

// Resolved says whether content is free of conflict markers of size: no line
// starts with a marker character repeated at least size times, followed by a
// space or the line's end. It's deliberately strict: a malformed marker line
// still counts, because what's resolved may be published, and a stray marker
// line never should be.
func Resolved(content []byte, size int) bool {
	if size <= 0 {
		size = DefaultMarkerSize
	}
	for _, line := range splitLines(content) {
		n := markerRun(line)
		if n < size {
			continue
		}
		rest := line[n:]
		if rest == "" || rest[0] == ' ' || rest[0] == '\n' || strings.HasPrefix(rest, "\r\n") || rest == "\r" {
			return false
		}
	}
	return true
}

// markerRun is how many times line's first character repeats at its start,
// when that character is one markers use, and 0 otherwise.
func markerRun(line string) int {
	if line == "" || !strings.ContainsRune("<|=>", rune(line[0])) {
		return 0
	}
	n := 1
	for n < len(line) && line[n] == line[0] {
		n++
	}
	return n
}

// splitLines splits text after each "\n", each line keeping its ending. A
// last line without one is a line too.
func splitLines(text []byte) []string {
	if len(text) == 0 {
		return nil
	}
	s := string(text)
	lines := make([]string, 0, strings.Count(s, "\n")+1)
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:i+1])
		s = s[i+1:]
	}
	return lines
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
