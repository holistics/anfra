//go:build !windows

package pretty

import "testing"

// On Unix the locale says whether the terminal is UTF-8: the first of LC_ALL,
// LC_CTYPE and LANG that's set.
func TestUTF8TerminalFromLocale(t *testing.T) {
	for _, c := range []struct {
		lcAll, lang string
		want        bool
	}{{"", "en_US.UTF-8", true}, {"", "C", false}, {"C", "en_US.UTF-8", false}, {"", "", false}} {
		t.Setenv("LC_ALL", c.lcAll)
		t.Setenv("LC_CTYPE", "")
		t.Setenv("LANG", c.lang)
		if got := utf8Terminal(); got != c.want {
			t.Errorf("LC_ALL=%q LANG=%q: %v, want %v", c.lcAll, c.lang, got, c.want)
		}
	}
}
