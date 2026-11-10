package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/update"
)

func TestUpdateBanner(t *testing.T) {
	orig := meta.Version
	t.Cleanup(func() { meta.Version = orig })
	meta.Version = "0.9.1"
	r := &update.Release{Version: "0.11.0"}

	want := "▲ Update available  0.9.1 → 0.11.0\n  Run `anfra update` to install."
	if got := updateBanner(r, cliui.TerminalStyles{}); got != want {
		t.Errorf("updateBanner(plain) = %q, want %q", got, want)
	}

	// As NewTerminalStyles makes them for 256 colours or more: the command is
	// a padded chip, shown without its backquotes.
	styled := cliui.TerminalStyles{
		Heading: lipgloss.NewStyle().Bold(true),
		Muted:   lipgloss.NewStyle().Faint(true),
		Success: lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)),
		Code:    lipgloss.NewStyle().Foreground(lipgloss.Color("#d4c0fc")).Background(lipgloss.Color("#332349")).Padding(0, 1),
	}
	wantStyled := "▲ Update available  0.9.1 → 0.11.0\n  Run  anfra update  to install."
	if got := updateBanner(r, styled); !strings.Contains(got, "\033[") || ansi.Strip(got) != wantStyled {
		t.Errorf("updateBanner(styled) = %q, want escape codes around %q", got, wantStyled)
	}

	for _, r := range []*update.Release{nil, {Version: "0.9.1"}} {
		if got := updateBanner(r, styled); got != "" {
			t.Errorf("updateBanner(%v) = %q, want empty", r, got)
		}
	}
}
