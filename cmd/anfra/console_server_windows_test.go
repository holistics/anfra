//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// The console's server, started as the console starts it, stops on stopServer
// by itself: not killed. The test binary stands in for anfra serve
// (TestMain's waitInterruptEnv).
func TestStopServer(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // the test binary itself
	cmd.Env = append(os.Environ(), waitInterruptEnv+"=1")
	configureServer(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	time.Sleep(500 * time.Millisecond) // let it reach its wait

	stopServer(cmd.Process)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("the server didn't stop within 3s")
	}
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("exit code %d, want 0 (a clean stop)", code)
	}
}
