//go:build windows

package sidecar

import (
	"context"
	"io"
	"os"
	"os/signal"
	"testing"
	"time"
)

// helperEnv makes the test binary act as a sidecar instead of running tests.
const helperEnv = "ANFRA_SIDECAR_TEST_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "ctrl-break": // a sidecar like canal: stops on os.Interrupt, which Ctrl-Break is in Go
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		os.Exit(0)
	case "stdin-eof": // a sidecar like anfra-node: stops when its stdin closes
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func startHelper(t *testing.T, mode string, pipeStdin bool) *Process {
	t.Helper()
	p, err := StartProcess(context.Background(), ProcSpec{
		Name:      "helper-" + mode,
		Path:      os.Args[0],
		Args:      []string{"-test.run=^$"},
		ExtraEnv:  []string{helperEnv + "=" + mode},
		PipeStdin: pipeStdin,
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // let it reach its wait
	return p
}

// A sidecar without a stdin pipe is stopped by Ctrl-Break, and exits by itself: not killed.
func TestCloseSendsCtrlBreak(t *testing.T) {
	p := startHelper(t, "ctrl-break", false)
	start := time.Now()
	p.Close(context.Background())
	<-p.Done()
	if took := time.Since(start); took >= 3*time.Second {
		t.Errorf("stopped after %s: killed after the wait, not stopped by Ctrl-Break", took)
	}
	if code := p.cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("exit code %d, want 0 (a clean stop)", code)
	}
}

// A sidecar with a stdin pipe is stopped by closing it, and exits by itself.
func TestCloseClosesStdin(t *testing.T) {
	p := startHelper(t, "stdin-eof", true)
	start := time.Now()
	p.Close(context.Background())
	<-p.Done()
	if took := time.Since(start); took >= 3*time.Second {
		t.Errorf("stopped after %s: killed after the wait, not stopped by stdin EOF", took)
	}
	if code := p.cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("exit code %d, want 0 (a clean stop)", code)
	}
}
