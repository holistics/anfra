package cliui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

// maxWidth caps wrapping on a wide terminal, where long lines read poorly.
const maxWidth = 100

// Printer writes a command's progress, warnings, questions and outcomes as lines
// told apart at a glance: each kind has its mark, colored on a terminal, and a
// line too long for the terminal wraps under its text, not under its mark.
// Sections are separated by a blank line. Off a terminal, it writes the same
// marks without color or wrapping.
type Printer struct {
	w       io.Writer
	Styles  TerminalStyles
	width   int    // 0: no wrapping
	section string // a heading to write before the next line
	wrote   bool
}

// NewPrinter prints to w, styled and wrapped when w is a terminal.
func NewPrinter(w io.Writer) *Printer {
	return &Printer{w: w, Styles: NewTerminalStyles(w), width: Width(w)}
}

// Width is how wide to wrap text written to w: the terminal's width, capped at
// maxWidth, or 0, no wrapping, when w isn't a terminal.
func Width(w io.Writer) int {
	if f, ok := w.(interface{ Fd() uintptr }); ok && terminalEnvironment() && term.IsTerminal(int(f.Fd())) {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width > 0 {
			return min(width, maxWidth)
		}
	}
	return 0
}

// Section starts a group of lines under a heading, written with its first line:
// a section with nothing in it shows nothing.
func (p *Printer) Section(title string) {
	p.section = title
}

// Gap separates what comes next from what came before with a blank line.
func (p *Printer) Gap() {
	if p.wrote {
		fmt.Fprintln(p.w)
		p.wrote = false
	}
}

// Step says what is being done now.
func (p *Printer) Step(format string, a ...any) {
	p.line(p.Styles.Accent.Render("•"), p.Styles.Muted, fmt.Sprintf(format, a...))
}

// Success says something was done.
func (p *Printer) Success(format string, a ...any) {
	p.line(p.Styles.Success.Render("✓"), noStyle, fmt.Sprintf(format, a...))
}

// Warn says something the reader should act on, though the command goes on.
func (p *Printer) Warn(format string, a ...any) {
	p.line(p.Styles.Warning.Render("!"), noStyle, fmt.Sprintf(format, a...))
}

// Fail says something did not work.
func (p *Printer) Fail(format string, a ...any) {
	p.line(p.Styles.Error.Render("✗"), noStyle, fmt.Sprintf(format, a...))
}

// Note adds to the line before it, indented under its text.
func (p *Printer) Note(format string, a ...any) {
	p.line(" ", p.Styles.Muted, fmt.Sprintf(format, a...))
}

// Detail quotes text from elsewhere (a server's message, say), indented under the
// line before it, its own line breaks kept and its terminal escapes removed.
func (p *Printer) Detail(text string) {
	p.Open()
	for l := range strings.SplitSeq(strings.TrimRight(ansi.Strip(text), "\n"), "\n") {
		l = strings.ReplaceAll(strings.TrimRight(l, " \t\r"), "\t", "  ")
		p.write("    ", p.Styles.Muted, l)
	}
}

// Question writes a question with its choices, the last the default, leaving the
// cursor after it for the answer.
func (p *Printer) Question(question string, choices ...string) {
	p.Open()
	labels := make([]string, len(choices))
	for i, c := range choices {
		labels[i] = p.Styles.Accent.Render("["+c[:1]+"]") + c[1:]
	}
	text := p.Styles.Heading.Render(question) + " " +
		strings.Join(labels, " / ") + " " + p.Styles.Muted.Render("(default: "+choices[len(choices)-1]+")")
	lines := p.wrap(text, 2)
	for i, l := range lines {
		mark := "  "
		if i == 0 {
			mark = p.Styles.Code.Render("?") + " "
		}
		if i < len(lines)-1 {
			fmt.Fprintln(p.w, mark+l)
		} else {
			fmt.Fprint(p.w, mark+l+" ")
		}
	}
	p.wrote = true
}

// noStyle leaves a line's text as it is, for kinds told apart by their mark.
var noStyle lipgloss.Style

func (p *Printer) line(mark string, s lipgloss.Style, text string) {
	p.Open()
	p.write(mark+" ", s, text)
}

// write writes text after mark, wrapping it under itself.
func (p *Printer) write(mark string, s lipgloss.Style, text string) {
	indent := strings.Repeat(" ", ansi.StringWidth(mark))
	for i, l := range p.wrap(text, len(indent)) {
		if i > 0 {
			mark = indent
		}
		fmt.Fprintln(p.w, mark+s.Render(l))
	}
	p.wrote = true
}

func (p *Printer) wrap(text string, indent int) []string {
	if p.width > indent+20 {
		text = ansi.Wrap(text, p.width-indent, "")
	}
	return strings.Split(text, "\n")
}

// Open writes a pending section's heading now, before something else writes
// to the same place: a question asked through another Printer, say.
func (p *Printer) Open() {
	if p.section == "" {
		return
	}
	p.Gap()
	fmt.Fprintln(p.w, p.Styles.Heading.Render(p.section))
	p.section = ""
}
