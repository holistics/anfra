//go:build windows

package cliui

import (
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

// noEscapes holds the file descriptors of consoles that can't show escape
// codes: EnableEscapeCodes failed for them, so they get no styles.
var noEscapes sync.Map

// EnableEscapeCodes turns on escape-code processing for each file that is a
// console. Windows Terminal and VS Code have it on; a console window of the
// old host (cmd, Windows PowerShell 5) has it off for each program until the
// program turns it on, and would otherwise print styles as raw codes (←[1m).
// main calls it, not run: the tests mustn't change the console they run in.
func EnableEscapeCodes(files ...*os.File) {
	for _, f := range files {
		h := windows.Handle(f.Fd())
		var mode uint32
		if windows.GetConsoleMode(h, &mode) != nil {
			continue // not a console
		}
		if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
			continue
		}
		if windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) != nil {
			noEscapes.Store(f.Fd(), true)
		}
	}
}

// escapesOff reports whether out is a console that can't show escape codes.
func escapesOff(out any) bool {
	f, ok := out.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	_, off := noEscapes.Load(f.Fd())
	return off
}
