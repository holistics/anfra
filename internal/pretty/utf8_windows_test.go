//go:build windows

package pretty

import "testing"

// On Windows trees are Unicode whatever the locale: Windows sets none, and Go
// writes to a console in UTF-16 whatever its code page.
func TestUTF8TerminalOnWindows(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "")
	if !utf8Terminal() {
		t.Error("ASCII trees on Windows")
	}
}
