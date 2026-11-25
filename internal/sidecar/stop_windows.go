//go:build windows

package sidecar

import (
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// stop asks the sidecar to stop with Ctrl-Break, since Windows has no SIGTERM:
// Go delivers it to canal as os.Interrupt, Bun to anfra-node as SIGBREAK. It
// is killed if it has not exited within wait. Its stdin pipe, if any, closes
// last: anfra-node's watchdog reads stdin closing as anfra dying.
func (p *Process) stop(wait time.Duration) {
	_ = sendCtrlBreak(uint32(p.cmd.Process.Pid)) //nolint:gosec // G115: a pid fits a DWORD

	select {
	case <-p.done:
	case <-time.After(wait):
		_ = p.cmd.Process.Kill()
	}
	if p.stdin != nil {
		p.stdin.Close()
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
