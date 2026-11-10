package textmerge

import "znkr.io/diff"

// changes is what side changed in base, line by line, as the stretches it
// replaced, in order. Myers' algorithm, git's default for `git merge-file`,
// then each changed stretch slid into place as git does (compact).
//
// The diff is the one place the merge depends on an outside algorithm, and the
// one most likely to change: histogram may replace it, if it merges AML better
// (docs/designs/treemerge.md).
func changes(base, side []string) []change {
	bChanged := make([]bool, len(base))
	sChanged := make([]bool, len(side))
	bi, si := 0, 0
	for _, e := range diff.Edits(base, side) {
		switch e.Op {
		case diff.Match:
			bi++
			si++
		case diff.Delete:
			bChanged[bi] = true
			bi++
		case diff.Insert:
			sChanged[si] = true
			si++
		}
	}
	compact(base, bChanged, sChanged)
	compact(side, sChanged, bChanged)
	return pair(bChanged, sChanged)
}

// compact slides each stretch of changed lines in one file into place, the way
// git does before merging, so that the merge sees the same changes git sees.
//
// A diff can mark different lines as changed and still be right: deleting one
// of two identical lines can delete either. git settles it: each stretch moves
// as far down as it can, unless a higher position lines it up with a change in
// the other file, where the two make one change. A stretch can move down by a
// line when its first line equals the line after it, and up when its last
// line equals the line before it.
//
// The two files' stretches correspond by the unchanged lines between them: the
// k-th gap between unchanged lines in one file pairs with the k-th in the
// other. other's stretches don't move while this file's do.
func compact(lines []string, changed, other []bool) {
	// otherFilled[k] says whether the other file has changed lines in its k-th
	// gap, the one after its k-th unchanged line.
	var otherFilled []bool
	filled := false
	for _, c := range other {
		if c {
			filled = true
			continue
		}
		otherFilled = append(otherFilled, filled)
		filled = false
	}
	otherFilled = append(otherFilled, filled)

	n := len(lines)
	unchangedBefore := 0 // unchanged lines before start: start's gap is this
	for start := 0; start < n; {
		if !changed[start] {
			unchangedBefore++
			start++
			continue
		}
		end := start
		for end < n && changed[end] {
			end++
		}
		var earliestEnd, matchedEnd int
		for {
			size := end - start
			// Up, as far as it goes, joining any stretch it reaches.
			for start > 0 && !changed[start-1] && lines[start-1] == lines[end-1] {
				start--
				end--
				changed[start], changed[end] = true, false
				unchangedBefore--
				for start > 0 && changed[start-1] {
					start--
				}
			}
			earliestEnd = end
			matchedEnd = -1
			if otherFilled[unchangedBefore] {
				matchedEnd = end
			}
			// Then down, as far as it goes, noting where it lines up with a
			// change in the other file.
			for end < n && !changed[end] && lines[start] == lines[end] {
				changed[start], changed[end] = false, true
				start++
				end++
				unchangedBefore++
				for end < n && changed[end] {
					end++
				}
				if otherFilled[unchangedBefore] {
					matchedEnd = end
				}
			}
			if end-start == size {
				break // it joined nothing: settled
			}
		}
		// Lined up with a change in the other file higher up: back up to there.
		if end != earliestEnd && matchedEnd >= 0 {
			for end > matchedEnd {
				start--
				end--
				changed[start], changed[end] = true, false
				unchangedBefore--
			}
		}
		start = end
	}
}

// pair turns the two files' changed lines into changes: unchanged lines pair
// in order, and the changed lines between two pairs are one change.
func pair(bChanged, sChanged []bool) []change {
	var out []change
	bi, si := 0, 0
	for bi < len(bChanged) || si < len(sChanged) {
		if (bi < len(bChanged) && bChanged[bi]) || (si < len(sChanged) && sChanged[si]) {
			c := change{b0: bi, s0: si}
			for bi < len(bChanged) && bChanged[bi] {
				bi++
			}
			for si < len(sChanged) && sChanged[si] {
				si++
			}
			c.b1, c.s1 = bi, si
			out = append(out, c)
			continue
		}
		bi++
		si++
	}
	return out
}
