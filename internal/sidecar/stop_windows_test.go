//go:build windows

package sidecar

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
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
	case "no-console": // an anfra started with no console, stopping a sidecar like canal
		os.Exit(stopWithoutConsole())
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

// An anfra with no console (started by an agent, or detached) still stops a
// sidecar with Ctrl-Break, and the sidecar exits by itself: not killed.
func TestCloseSendsCtrlBreakWithoutConsole(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$") //nolint:gosec // the test binary itself
	cmd.Env = append(os.Environ(), helperEnv+"=no-console")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
	err := cmd.Run()
	switch code := cmd.ProcessState.ExitCode(); code {
	case 0:
	case 2:
		t.Fatal("the detached anfra still had a console")
	case 3:
		t.Fatal("killed after the wait, not stopped by Ctrl-Break")
	default:
		t.Fatalf("exit code %d: %v", code, err)
	}
}

// stopWithoutConsole answers 0 when Close stopped the sidecar cleanly.
func stopWithoutConsole() int {
	if hasConsole() {
		return 2
	}
	p, err := StartProcess(context.Background(), ProcSpec{
		Name:     "helper-ctrl-break",
		Path:     os.Args[0],
		Args:     []string{"-test.run=^$"},
		ExtraEnv: []string{helperEnv + "=ctrl-break"},
	})
	if err != nil {
		return 1
	}
	time.Sleep(500 * time.Millisecond)
	start := time.Now()
	p.Close(context.Background())
	<-p.Done()
	if time.Since(start) >= 3*time.Second || p.cmd.ProcessState.ExitCode() != 0 {
		return 3
	}
	return 0
}
