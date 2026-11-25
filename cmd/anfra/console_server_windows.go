//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// configureServer starts the console's server in its own process group, so
// Ctrl-C at the console doesn't reach it, on the console's own console: Windows
// has no SIGTERM, and the Ctrl-Break stopServer sends reaches only processes on
// the sender's console. Closing the terminal ends it too, as leaving the
// console does.
func configureServer(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// stopServer asks the console's server to stop: Go delivers Ctrl-Break as
// os.Interrupt, which anfra serve stops on.
func stopServer(p *os.Process) {
	_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(p.Pid)) //nolint:gosec // G115: a pid fits a DWORD
}
