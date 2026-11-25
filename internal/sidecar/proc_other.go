//go:build !linux && !windows

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

// tieToHost has nothing to do: the sidecar's own watchdog stops it when anfra goes.
func tieToHost(*exec.Cmd) error { return nil }
