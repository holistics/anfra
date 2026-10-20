package cliui

import (
	"bytes"
	"io"
	"sync"
)

// Short inline programs can exit before capability replies arrive, leaving them
// as shell input: https://github.com/charmbracelet/bubbletea/issues/1590.
// Bubble Tea v2.1 has no query-disable option that retains keyboard input. Omit
// its optional synchronized-output, Unicode-width and Kitty keyboard probes.
var terminalCapabilityQueries = [...]string{"\x1b[?2026$p", "\x1b[?2027$p", "\x1b[?u"}

type terminalQueryWriter struct {
	mu      sync.Mutex
	out     io.Writer
	pending []byte
	err     error
}

func (w *terminalQueryWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	w.pending = append(w.pending, p...)
	data := w.pending
	w.pending = nil
	output := make([]byte, 0, len(data))
	for len(data) > 0 {
		matched, incomplete := false, false
		for _, query := range terminalCapabilityQueries {
			if bytes.HasPrefix(data, []byte(query)) {
				data = data[len(query):]
				matched = true
				break
			}
			if bytes.HasPrefix([]byte(query), data) {
				incomplete = true
			}
		}
		if matched {
			continue
		}
		if incomplete {
			w.pending = append(w.pending, data...)
			break
		}
		output = append(output, data[0])
		data = data[1:]
	}
	w.write(output)
	return len(p), w.err
}

func (w *terminalQueryWriter) write(p []byte) {
	if len(p) == 0 || w.err != nil {
		return
	}
	var n int
	n, w.err = w.out.Write(p)
	if w.err == nil && n != len(p) {
		w.err = io.ErrShortWrite
	}
}

func (w *terminalQueryWriter) flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.write(w.pending)
	w.pending = nil
	return w.err
}

type terminalOutputFile interface {
	io.ReadWriteCloser
	Fd() uintptr
}

// Preserve the full terminal file interface Bubble Tea uses for resize events.
type terminalQueryFileWriter struct {
	*terminalQueryWriter
	file terminalOutputFile
}

func (w terminalQueryFileWriter) Read(p []byte) (int, error) { return w.file.Read(p) }
func (w terminalQueryFileWriter) Close() error               { return w.file.Close() }
func (w terminalQueryFileWriter) Fd() uintptr                { return w.file.Fd() }
