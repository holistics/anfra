// Package logging provides the host's structured, aggregated logger.
//
// The host is the only component that touches log files: it writes its own JSON
// records and forwards each sidecar's stderr (via Writer) into one per-repo
// log file plus its own stderr. Services only write to stderr.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Logging is the host's logging setup:
//   - Logger:       the host's own structured logger (writes to anfra.log).
//   - StderrWriter: sink for forwarding sidecar stderr — the log stream, same
//     destination as Logger (anfra.log; also host stderr if ANFRA_SIDECAR_STDERR).
//   - StdoutWriter: sink for forwarding sidecar stdout — discarded by default
//     (banners / incidental noise), or host stdout if ANFRA_SIDECAR_STDOUT.
type Logging struct {
	Logger       *slog.Logger
	StderrWriter io.Writer
	StdoutWriter io.Writer
	LogPath      string
	closeFn      func() error
}

func (l *Logging) Close() error {
	if l.closeFn != nil {
		return l.closeFn()
	}
	return nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Setup ensures logsDir exists, opens anfra.log there, and returns a host logger
// (and the same Writer the sidecars' output is forwarded into). The envelope
// ({ts, level, msg, service, repo, pid}) matches the Node sidecar's. Level
// comes from LOG_LEVEL (default info).
//
// By default logs go ONLY to the file, keeping the command's console output
// clean (results on stdout, command errors via cobra). Set ANFRA_LOG_STDERR=1
// to also stream logs to stderr (useful for `anfra serve` / debugging).
func Setup(logsDir, repo string) (*Logging, error) {
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logsDir, "anfra.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	// Log stream (host records + sidecar stderr) → file only by default; also to
	// the host's stderr when ANFRA_SIDECAR_STDERR is set.
	var stderrWriter io.Writer = f
	if os.Getenv("ANFRA_SIDECAR_STDERR") != "" {
		stderrWriter = io.MultiWriter(f, os.Stderr)
	}
	// Sidecar stdout (banners / incidental output) → discarded by default; to the
	// host's stdout when ANFRA_SIDECAR_STDOUT is set.
	stdoutWriter := io.Discard
	if os.Getenv("ANFRA_SIDECAR_STDOUT") != "" {
		stdoutWriter = os.Stdout
	}
	handler := slog.NewJSONHandler(stderrWriter, &slog.HandlerOptions{
		Level: parseLevel(os.Getenv("LOG_LEVEL")),
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Key = "ts"
			}
			return a
		},
	})
	logger := slog.New(handler).With(
		"service", "anfra-host",
		"repo", repo,
		"pid", os.Getpid(),
	)

	return &Logging{Logger: logger, StderrWriter: stderrWriter, StdoutWriter: stdoutWriter, LogPath: logPath, closeFn: f.Close}, nil
}
