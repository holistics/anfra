//go:build windows

package main

import (
	"io"
	"os"

	"github.com/muesli/cancelreader"
	"golang.org/x/sys/windows"
)

// newLineReader is a reader of typed lines that ctx's end can stop
// (cancelreader). On Windows, cancelreader also puts the console in raw mode,
// for full-screen programs that echo and edit input themselves: a question's
// answer then wasn't shown, and Enter didn't end it. The console's own mode is
// put back, so it echoes, edits the line, and takes Ctrl-C as an interrupt;
// cancelreader still stops its wait for input.
func newLineReader(in io.Reader) (cancelreader.CancelReader, error) {
	var mode uint32
	console := in == io.Reader(os.Stdin) && windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode) == nil
	cr, err := cancelreader.NewReader(in)
	if err == nil && console {
		_ = windows.SetConsoleMode(windows.Handle(os.Stdin.Fd()), mode)
	}
	return cr, err
}
