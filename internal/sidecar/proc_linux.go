//go:build linux

package sidecar

import (
	"os/exec"
	"syscall"
)

// configureProc puts the sidecar in its own process group (so we can signal the
// whole group) and asks the kernel to send it SIGTERM if this host process dies
// — the strongest orphan guard, surviving even a host SIGKILL.
func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
}

// tieToHost has nothing to do: Pdeathsig, set before the start, ties the
// sidecar to anfra.
func tieToHost(*exec.Cmd) error { return nil }
