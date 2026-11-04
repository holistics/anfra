package cliui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

func TestReadEntry(t *testing.T) {
	history := []string{`\validate`, `\version`}
	for _, tc := range []struct {
		name, keys, want string
	}{
		{"typed", "\\version\r", `\version`},
		{"the last entry", "\x1b[A\r", `\version`},
		{"an earlier entry", "\x1b[A\x1b[A\r", `\validate`},
		{"no further than the first", "\x1b[A\x1b[A\x1b[A\r", `\validate`},
		{"back down", "\x1b[A\x1b[A\x1b[B\r", `\version`},
		{"the draft kept while browsing", "\\st\x1b[A\x1b[B\r", `\st`},
		{"Ctrl-C clears the entry", "\\nope\x03\\q\r", `\q`},
		{"Ctrl-D in an entry is no end", "\\q\x04\r", `\q`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := ReadEntry(ctx, strings.NewReader(tc.keys), io.Discard, EntryOptions{Prompt: "> ", History: history})
			if err != nil || got != tc.want {
				t.Fatalf("entry = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestReadEntryEnds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ReadEntry(ctx, strings.NewReader("\x03\x04"), io.Discard, EntryOptions{Prompt: "> "}); !errors.Is(err, io.EOF) {
		t.Fatalf("Ctrl-D on an empty entry: %v, want io.EOF", err)
	}
}

// Ctrl-C on an empty entry says how to leave, until the next key.
func TestReadEntrySaysHowToLeave(t *testing.T) {
	m := &entryReader{input: textarea.New(), opts: EntryOptions{Prompt: "> ", Leave: `\q or Ctrl-D to leave`}}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !strings.Contains(m.View().Content, `\q or Ctrl-D to leave`) {
		t.Fatalf("after Ctrl-C: %q", m.View().Content)
	}
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if strings.Contains(m.View().Content, "to leave") {
		t.Fatalf("after a key: %q", m.View().Content)
	}
}
