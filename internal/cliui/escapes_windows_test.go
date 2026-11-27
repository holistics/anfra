//go:build windows

package cliui

import (
	"os"
	"testing"
)

// A file that isn't a console is left as it is: nothing to turn on, and its
// styles are decided as before.
func TestEnableEscapeCodesLeavesAPipeAlone(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	EnableEscapeCodes(w)
	if escapesOff(w) {
		t.Error("a pipe counted as a console without escape codes")
	}
}
