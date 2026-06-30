//go:build !linux

package sidecar

import (
	"os/exec"
	"syscall"
)

// On non-Linux platforms there is no Pdeathsig; we still isolate the process
// group, and rely on the sidecar's own stdin-EOF + ppid watchdog to avoid
// orphaning.
func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
