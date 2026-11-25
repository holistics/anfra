//go:build windows

package sidecar

import (
	"os/exec"
	"syscall"
)

// configureProc starts the sidecar in its own process group, as Setpgid does on
// Unix: Ctrl-C at the terminal then reaches anfra, not the sidecar, and stop
// can send Ctrl-Break to the sidecar alone.
func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
