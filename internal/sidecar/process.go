package sidecar

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// process is the shared lifecycle of any spawned sidecar: orphan-prevention
// (process group + Pdeathsig) and stdio forwarded into the host log sink.
// Protocol/readiness live in the per-sidecar manage/client files.
type process struct {
	name   string
	cmd    *exec.Cmd
	stdin  *os.File // held open; closing on host exit trips a stdin-EOF watchdog if the child has one
	logger *slog.Logger
}

type procSpec struct {
	Name      string
	Path      string
	Args      []string
	ExtraEnv  []string
	Stdout    io.Writer // child stdout sink; defaults to io.Discard
	Stderr    io.Writer // child stderr sink (the log stream); defaults to os.Stderr
	Logger    *slog.Logger
	PipeStdin bool // wire a stdin pipe so the child can use stdin-EOF as a parent-death signal (Node does; canal doesn't)
}

func startProcess(spec procSpec) (*process, error) {
	// No sink supplied → discard. Callers (the sidecar managers / host) wire the
	// real destinations; the supervisor takes no opinion on where output goes.
	stdout := spec.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := spec.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	logger := spec.Logger
	if logger == nil {
		logger = slog.Default()
	}

	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), spec.ExtraEnv...)

	var stdin *os.File
	if spec.PipeStdin {
		r, wr, err := os.Pipe()
		if err != nil {
			return nil, fmt.Errorf("stdin pipe: %w", err)
		}
		cmd.Stdin = r
		stdin = wr
		defer r.Close() // the child keeps its own copy after Start
	}

	configureProc(cmd) // process group + Pdeathsig (platform-specific)

	if err := cmd.Start(); err != nil {
		if stdin != nil {
			stdin.Close()
		}
		return nil, fmt.Errorf("start %s: %w", spec.Name, err)
	}
	logger.Info("sidecar.spawned", "name", spec.Name, "childPid", cmd.Process.Pid)
	return &process{name: spec.Name, cmd: cmd, stdin: stdin, logger: logger}, nil
}

// Logger is the supervisor's logger (defaulted in startProcess), so managers can
// log their own events without re-defaulting.
func (p *process) Logger() *slog.Logger { return p.logger }

func (p *process) pid() int {
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *process) close() {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	p.logger.Info("sidecar.closing", "name", p.name, "childPid", p.cmd.Process.Pid)
	_ = p.cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan struct{})
	go func() { _ = p.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	}
	if p.stdin != nil {
		p.stdin.Close()
	}
}
