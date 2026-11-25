//go:build windows

package sidecar

import (
	"sync"
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
		_ = sendCtrlBreak(uint32(p.cmd.Process.Pid)) //nolint:gosec // G115: a pid fits a DWORD
	}

	select {
	case <-p.done:
	case <-time.After(wait):
		_ = p.cmd.Process.Kill()
	}
}

var (
	kernel32      = windows.NewLazySystemDLL("kernel32.dll")
	attachConsole = kernel32.NewProc("AttachConsole")
	freeConsole   = kernel32.NewProc("FreeConsole")

	// consoleMu keeps two stops from attaching to sidecars' consoles at once:
	// a process has one console at a time.
	consoleMu sync.Mutex
)

// sendCtrlBreak sends Ctrl-Break to the process group pid leads. Ctrl-Break
// only reaches processes on the sender's console, so an anfra without one
// attaches to the sidecar's hidden console to send it, then leaves.
func sendCtrlBreak(pid uint32) error {
	if hasConsole() {
		return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, pid)
	}
	consoleMu.Lock()
	defer consoleMu.Unlock()
	if r, _, err := attachConsole.Call(uintptr(pid)); r == 0 {
		return err
	}
	defer freeConsole.Call() //nolint:errcheck // nothing to do if it fails
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, pid)
}
