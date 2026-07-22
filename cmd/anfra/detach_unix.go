//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detachProcess puts the child in its own process group so it isn't killed when
// the foreground anfra process (and its group) exits.
func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
