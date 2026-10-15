// Package cliui owns inline terminal presentation, not authentication or storage.
package cliui

import (
	"io"
	"os"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// CanInteract requires terminal input and the supplied stderr writer; NO_COLOR does not disable interaction.
func CanInteract(in io.Reader, out io.Writer) bool {
	return terminalEnvironment() && isTerminal(in) && isTerminal(out)
}

func terminalEnvironment() bool {
	ci := os.Getenv("CI")
	return (ci == "" || strings.EqualFold(ci, "false") || ci == "0") && os.Getenv("TERM") != "dumb"
}

func isTerminal(stream any) bool {
	f, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}

// TerminalStyles gives auth views a shared hierarchy; its zero value renders plain text.
type TerminalStyles struct {
	Heading, Muted, Accent, Selection, Success, Warning, Error, Link lipgloss.Style
}

// NewTerminalStyles styles only terminal output, respecting NO_COLOR, CI and TERM=dumb.
// ANSI colors use the user's terminal palette rather than assuming a light or dark background.
func NewTerminalStyles(out io.Writer) TerminalStyles {
	_, noColor := os.LookupEnv("NO_COLOR")
	if noColor || !terminalEnvironment() || !isTerminal(out) {
		return TerminalStyles{}
	}
	return TerminalStyles{
		Heading:   lipgloss.NewStyle().Bold(true),
		Muted:     lipgloss.NewStyle().Faint(true),
		Accent:    lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6)),
		Selection: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(6)),
		Success:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)),
		Warning:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)),
		Error:     lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)),
		Link:      lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(12)),
	}
}

// SafeText strips terminal escapes and flattens untrusted display text into one plain line.
func SafeText(s string) string {
	s = ansi.Strip(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return ' '
		}
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func terminalOptions(in io.Reader, out io.Writer) []tea.ProgramOption {
	opts := []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(out)}
	if _, present := os.LookupEnv("NO_COLOR"); present {
		opts = append(opts, tea.WithColorProfile(colorprofile.ASCII))
	}
	return opts
}
