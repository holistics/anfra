package sidecar

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/holistics/anfra/shared/logkit"
)

// A sidecar's log events, as the host sees it (docs/designs/logs.md).
var (
	eventSpawned = logkit.DefineEvent("sidecar.spawned", "A sidecar process was started.", "name", "child_pid")
	eventClosing = logkit.DefineEvent("sidecar.closing", "A sidecar process is being stopped.", "name", "child_pid")
	// EventReady is a sidecar answering, started here (socket) or already
	// running at a URL (owned false).
	EventReady = logkit.DefineEvent("sidecar.ready", "A sidecar is ready for calls.", "name", "url", "socket", "owned")
)

// Process is the shared lifecycle of any spawned sidecar: orphan-prevention
// (process group + Pdeathsig) and stdio forwarded into the host log sink.
// Protocol/readiness live in each sidecar's own package.
type Process struct {
	name   string
	cmd    *exec.Cmd
	stdin  *os.File // held open; closing on host exit trips a stdin-EOF watchdog if the child has one
	logger *slog.Logger
}

// ProcSpec is how to spawn a sidecar.
type ProcSpec struct {
	Name string
	Path string
	Args []string
	// ExtraEnv is what the supervisor sets, over anything inherited or
	// forwarded (childEnv).
	ExtraEnv []string
	// DefaultEnv is what the supervisor sets unless anfra's environment sets it
	// for this sidecar, through EnvPrefix.
	DefaultEnv []string
	// EnvPrefix forwards <EnvPrefix><NAME>=value from anfra's environment as
	// NAME=value, to this sidecar only: e.g. ANFRA_CANAL_QUERY_ENV_.
	EnvPrefix string
	Stdout    io.Writer // child stdout sink; defaults to io.Discard
	Stderr    io.Writer // child stderr sink (the log stream); defaults to os.Stderr
	Logger    *slog.Logger
	PipeStdin bool // wire a stdin pipe so the child can use stdin-EOF as a parent-death signal (Node does; canal doesn't)
}

// StartProcess spawns the sidecar spec describes. It does not wait for it to
// be ready: that is the sidecar's protocol, its own package's to check.
//
// ctx carries the trace its log record joins, as slog.Handler.Handle's does: it
// is not for cancellation, and does not bound the sidecar's life, which lasts
// until Close (exec.Command, not exec.CommandContext).
func StartProcess(ctx context.Context, spec ProcSpec) (*Process, error) {
	// No sink supplied → discard. Callers (the sidecar supervisors / host) wire the
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

	cmd := exec.Command(spec.Path, spec.Args...) //nolint:gosec // G204: spawns our own bundled/configured sidecar, not untrusted input
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = childEnv(os.Environ(), spec.EnvPrefix, spec.DefaultEnv, spec.ExtraEnv)

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
	logger.InfoContext(ctx, "Sidecar started", logkit.EventKey, eventSpawned, "name", spec.Name, "child_pid", cmd.Process.Pid)
	return &Process{name: spec.Name, cmd: cmd, stdin: stdin, logger: logger}, nil
}

// Logger is the supervisor's logger (defaulted in StartProcess), so supervisors
// can log their own events without re-defaulting.
func (p *Process) Logger() *slog.Logger { return p.logger }

// Close stops the sidecar: SIGTERM, then SIGKILL to its process group if it has
// not exited within 3s.
//
// ctx carries the trace its log record joins, as slog.Handler.Handle's does: it
// is not for cancellation, so a sidecar closed after Ctrl-C still gets its 3s.
func (p *Process) Close(ctx context.Context) {
	if p.cmd == nil || p.cmd.Process == nil {
		return
	}
	p.logger.InfoContext(ctx, "Closing sidecar", logkit.EventKey, eventClosing, "name", p.name, "child_pid", p.cmd.Process.Pid)
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
