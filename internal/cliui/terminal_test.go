package cliui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestActivityDoesNotLeaveTerminalQueriesForShell(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("TERM_PROGRAM", "kitty")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := RunActivity(ctx, strings.NewReader("\x1b[8;24;80t"), &output, "Signing out", func(_ context.Context, _ func(string), message func(string)) error {
		message("Signed out.")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"\x1b[?2026$p", "\x1b[?2027$p", "\x1b[?u"} {
		if strings.Contains(output.String(), query) {
			t.Fatalf("activity emitted a query whose reply could reach the shell: %q; output: %q", query, output.String())
		}
	}
}

func TestActivityKeyboardCancellation(t *testing.T) {
	for _, key := range []string{"\x03", "\x1b"} {
		t.Run(key, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var output bytes.Buffer
			err := RunActivity(ctx, strings.NewReader(key), &output, "Signing out", func(ctx context.Context, _ func(string), _ func(string)) error {
				<-ctx.Done()
				return ctx.Err()
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("keyboard cancellation: %v", err)
			}
		})
	}
}
