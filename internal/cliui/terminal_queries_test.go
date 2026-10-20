package cliui

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestTerminalQueryWriterPreservesRenderingAcrossWrites(t *testing.T) {
	input := "\x1b[?25l\x1b[?2026$p\x1b[?2027$p\x1b[?u\x1b[31mSigning out…\x1b[0m\r\x1b[2K\x1b[?25h\x1b[?2028$p\x1b[?202"
	want := "\x1b[?25l\x1b[31mSigning out…\x1b[0m\r\x1b[2K\x1b[?25h\x1b[?2028$p\x1b[?202"
	for chunkSize := 1; chunkSize <= len(input); chunkSize++ {
		var output bytes.Buffer
		writer := &terminalQueryWriter{out: &output}
		for start := 0; start < len(input); start += chunkSize {
			chunk := []byte(input[start:min(start+chunkSize, len(input))])
			n, err := writer.Write(chunk)
			if err != nil || n != len(chunk) {
				t.Fatalf("chunk size %d: Write = %d, %v", chunkSize, n, err)
			}
		}
		if err := writer.flush(); err != nil {
			t.Fatal(err)
		}
		if output.String() != want {
			t.Fatalf("chunk size %d: output = %q; want %q", chunkSize, output.String(), want)
		}
	}
}

func TestTerminalQueryWriterReportsOutputFailure(t *testing.T) {
	reader, output := io.Pipe()
	_ = reader.Close()
	defer output.Close()
	writer := &terminalQueryWriter{out: output}
	if _, err := writer.Write([]byte("Signing out")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("output error hidden: %v", err)
	}
	if _, err := writer.Write([]byte("more")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("subsequent write error hidden: %v", err)
	}
	if err := writer.flush(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("flush error hidden: %v", err)
	}
}
