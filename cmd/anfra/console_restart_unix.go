//go:build !windows

package main

import (
	"os"
	"syscall"
)

// restartConsole runs the console again on the anfra now at exe, the path
// it was started from, in this process: the terminal, the folder and the
// history carry on.
func restartConsole(exe string) error {
	return syscall.Exec(exe, os.Args, append(os.Environ(), consoleRestarted+"=1")) //nolint:gosec // our own binary, our own args
}
