// Package cliui owns inline terminal presentation, not authentication or storage.
package cliui

import (
	"image/color"
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
	Heading, Muted, Accent, Selection, Success, Warning, Error, Link, Code, Key lipgloss.Style
}

// violet is anfra's accent, the design package's primary
// (web/design/DESIGN.md: oklch(0.565 0.194 300.3)): what a person can act on,
// a prompt, a selection, a link, a command to run.
var violet = lipgloss.Color("#8a51d4")

// A code span, a command to run, is the design package's dark violet text
// (violet-350) on its violet wash (violet-a26 over its dark panel, ink-925):
// both set, so it reads on a dark terminal and a light one alike, 8.6:1.
var (
	codeText = lipgloss.Color("#d4c0fc")
	codeFill = lipgloss.Color("#332349")
)

// NewTerminalStyles styles only terminal output, respecting NO_COLOR, CI and TERM=dumb.
// Hues: violet, anfra's accent, for what can be acted on; bright blue for a key;
// green, yellow and red for tones; everything else by weight. Violet is the
// one color not of the user's terminal palette, converted to what the
// terminal shows (the nearest of 256 colors; magenta, of 16), and mid-light,
// for light and dark backgrounds alike. A command to run is a code span, a
// violet chip; of 16 colors, bold magenta, without a fill.
func NewTerminalStyles(out io.Writer) TerminalStyles {
	_, noColor := os.LookupEnv("NO_COLOR")
	if noColor || !terminalEnvironment() || !isTerminal(out) {
		return TerminalStyles{}
	}
	var accent color.Color = lipgloss.ANSIColor(5) // magenta: of 16 colors, the nearest violet that isn't a key's bright blue
	code := lipgloss.NewStyle().Bold(true).Foreground(accent)
	if profile := colorprofile.Detect(out, os.Environ()); profile > colorprofile.ANSI {
		accent = profile.Convert(violet)
		code = lipgloss.NewStyle().Foreground(profile.Convert(codeText)).Background(profile.Convert(codeFill)).Padding(0, 1)
	}
	return TerminalStyles{
		Heading:   lipgloss.NewStyle().Bold(true),
		Muted:     lipgloss.NewStyle().Faint(true),
		Accent:    lipgloss.NewStyle().Foreground(accent),
		Selection: lipgloss.NewStyle().Bold(true).Foreground(accent),
		Success:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)),
		Warning:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)),
		Error:     lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)),
		Link:      lipgloss.NewStyle().Underline(true).Foreground(accent),
		Code:      code,
		Key:       lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(12)), // bright: blue reads poorly on dark backgrounds
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

func terminalOptions(in io.Reader, out io.Writer) ([]tea.ProgramOption, func() error) {
	filtered := &terminalQueryWriter{out: out}
	var writer io.Writer = filtered
	if file, ok := out.(terminalOutputFile); ok {
		writer = terminalQueryFileWriter{terminalQueryWriter: filtered, file: file}
	}
	opts := []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(writer)}
	if _, present := os.LookupEnv("NO_COLOR"); present {
		opts = append(opts, tea.WithColorProfile(colorprofile.ASCII))
	}
	return opts, filtered.flush
}
