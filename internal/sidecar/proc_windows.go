//go:build windows

package sidecar

import (
	"os/exec"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureProc starts the sidecar in its own process group, as Setpgid does on
// Unix: Ctrl-C at the terminal then reaches anfra, not the sidecar, and stop
// can send Ctrl-Break to the sidecar alone. An anfra with no console (started
// by an agent, or detached) gives it a hidden one, or Windows would open a
// window for it.
func configureProc(cmd *exec.Cmd) {
	flags := uint32(syscall.CREATE_NEW_PROCESS_GROUP)
	if !hasConsole() {
		flags |= windows.CREATE_NO_WINDOW
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
}

// hasConsole reports whether anfra was started with a console. It is read once:
// stop attaches anfra to a sidecar's console for a moment, and must not change
// the answer.
var hasConsole = sync.OnceValue(func() bool {
	_, err := windows.GetConsoleCP()
	return err == nil
})
