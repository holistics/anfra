//go:build windows

package sidecar

import (
	"time"

	"golang.org/x/sys/windows"
)

// stop asks the sidecar to stop the one way it listens for, since Windows has
// no SIGTERM: a sidecar with a stdin pipe (anfra-node) stops when it closes; one
// without (canal) on Ctrl-Break, which Go delivers as os.Interrupt. Not both:
// Ctrl-Break's default handler ends a process at once, cutting short the
// shutdown stdin EOF started. It is killed if it has not exited within wait.
func (p *Process) stop(wait time.Duration) {
	if p.stdin != nil {
		p.stdin.Close()
	} else {
		_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(p.cmd.Process.Pid)) //nolint:gosec // G115: a pid fits a DWORD
	}

	select {
	case <-p.done:
	case <-time.After(wait):
		_ = p.cmd.Process.Kill()
	}
}
