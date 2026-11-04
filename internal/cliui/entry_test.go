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

// A status, a notice's badge, shows under the prompt while typing, and isn't
// left on screen with the entry.
func TestReadEntryStatus(t *testing.T) {
	m := &entryReader{input: textarea.New(), opts: EntryOptions{Prompt: "> ", Status: "↑ anfra 0.17.0 · Run update"}}
	if !strings.Contains(m.View().Content, "↑ anfra 0.17.0") {
		t.Fatalf("while typing: %q", m.View().Content)
	}
	m.done = true
	if got := m.View().Content; strings.Contains(got, "anfra 0.17.0") || strings.Contains(got, "─") {
		t.Fatalf("left on screen: %q", got)
	}
}

// The frame keeps one height, a badge or the leave hint there or not: nothing
// under the prompt shifts as they come and go.
func TestReadEntryFrameKeepsItsHeight(t *testing.T) {
	m := &entryReader{input: textarea.New(), opts: EntryOptions{Prompt: "> ", Leave: "leave"}}
	height := func() int { return strings.Count(m.View().Content, "\n") }
	quiet := height()
	lines := strings.Split(m.View().Content, "\n")
	if strings.TrimSpace(lines[len(lines)-1]) == "" && !strings.Contains(lines[len(lines)-1], "\u00a0") {
		t.Fatalf("a blank status line of spaces, which Bubble Tea doesn't draw: %q", m.View().Content)
	}
	m.opts.Status = "↑ anfra 0.17.0 · Run update"
	withBadge := height()
	m.leaving = true
	withHint := height()
	if withBadge != quiet || withHint != quiet {
		t.Fatalf("heights: quiet %d, with a badge %d, with the hint %d", quiet, withBadge, withHint)
	}
}

// The line under the frame holds the context on the left and the status on
// the right; too narrow, the context is cut short first.
func TestReadEntryBottomLine(t *testing.T) {
	m := &entryReader{input: textarea.New(), opts: EntryOptions{Context: "acme · -d sales --json"}} // unstyled, without a terminal
	if got := m.bottom(40, "↑ anfra 0.17.0"); got != "acme · -d sales --json    ↑ anfra 0.17.0" {
		t.Errorf("wide: %q", got)
	}
	if got := m.bottom(28, "↑ anfra 0.17.0"); got != "acme · -d s…  ↑ anfra 0.17.0" {
		t.Errorf("narrow: %q", got)
	}
	if got := m.bottom(10, "↑ anfra 0.17.0"); got != "↑ anfra 0…" {
		t.Errorf("narrower than the status: %q", got)
	}
	// A warning comes first, the context after it, cut short before the status.
	m.opts.Warning = "! --dataset is missing"
	if got := m.bottom(62, "↑ anfra 0.17.0"); got != "! --dataset is missing  acme · -d sales --json  ↑ anfra 0.17.0" {
		t.Errorf("with a warning: %q", got)
	}
	if got := m.bottom(45, "↑ anfra 0.17.0"); got != "! --dataset is missing  acme…  ↑ anfra 0.17.0" {
		t.Errorf("with a warning, narrow: %q", got)
	}
	if got := m.bottom(30, "↑ anfra 0.17.0"); got != "! --dataset is missing  ↑ anf…" {
		t.Errorf("with a warning, narrower: %q", got)
	}
}
