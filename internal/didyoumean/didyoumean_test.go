package didyoumean

import "testing"

// A name differing only in case or separators wins over a nearer spelling; a
// typo within two edits is found; anything further suggests nothing.
func TestFind(t *testing.T) {
	names := []string{"timezone", "week_start_day", "enabled", "max_duration"}
	for typed, want := range map[string]string{
		"weekStartDay":   "week_start_day",
		"week-start-day": "week_start_day",
		"enabeld":        "enabled",
		"timezon":        "timezone",
		"max_duraton":    "max_duration",
		"pagination":     "",
		"":               "",
	} {
		if got := Find(typed, names); got != want {
			t.Errorf("Find(%q) = %q, want %q", typed, got, want)
		}
	}
}
