//go:build windows

package main

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detachProcess starts the child in its own process group, without anfra's
// console, so it isn't stopped by Ctrl-C at the terminal nor when the terminal
// closes, as Setpgid does on Unix.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
}
