//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// configureServer starts the console's server in its own process group, so
// Ctrl-C at the console doesn't reach it.
func configureServer(cmd *exec.Cmd) { detachProcess(cmd) }

// stopServer asks the console's server to stop.
func stopServer(p *os.Process) { _ = p.Signal(syscall.SIGTERM) }
