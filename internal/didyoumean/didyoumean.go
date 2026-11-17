// Package didyoumean finds the name someone meant when they typed one that
// doesn't exist, for an error to suggest: "Did you mean week_start_day?".
package didyoumean

import "strings"

// Find is the name in names that typed was meant to be: the one equal to it
// with case, _ and - ignored, or else the nearest within two edits, or "".
func Find(typed string, names []string) string {
	t := fold(typed)
	for _, n := range names {
		if fold(n) == t {
			return n
		}
	}
	best, bestDist := "", 3
	for _, n := range names {
		if d := editDistance(t, fold(n)); d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

var separators = strings.NewReplacer("_", "", "-", "")

func fold(s string) string { return separators.Replace(strings.ToLower(s)) }

// editDistance is the Levenshtein distance between a and b, by bytes.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
