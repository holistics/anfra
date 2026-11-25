//go:build !windows

package sidecar

import (
	"syscall"
	"time"
)

// stop sends SIGTERM, then SIGKILL to the sidecar's process group if it has
// not exited within wait.
func (p *Process) stop(wait time.Duration) {
	_ = p.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case <-p.done:
	case <-time.After(wait):
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
	if p.stdin != nil {
		p.stdin.Close()
	}
}
