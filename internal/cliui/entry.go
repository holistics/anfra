package cliui

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// EntryOptions are what ReadEntry shows and remembers.
type EntryOptions struct {
	Prompt  string   // before the entry: "query> "
	History []string // earlier entries, oldest first, for Up and Down
	Leave   string   // the hint shown under the prompt after Ctrl-C on an empty entry, in Status's place
	// Warning, Context and Status share the line under the prompt while
	// typing: Warning first, styled, what the next entry would fail for, where
	// the eye lands; then Context, styled, where the session is;
	// Status on the right, styled, a notice's badge. Too narrow, Context is cut
	// short with … first, then Status, then Warning. "" for none.
	Warning string
	Context string
	Status  string
	// Ends reports whether the text typed is a whole entry, for Enter to end
	// it rather than start a line (AQL's ;;); nil, Enter always ends it.
	// Alt+Enter and Ctrl+Enter end it whatever it holds.
	Ends func(text string) bool
}

// more is the prompt of an entry's lines after its first: "... ", under the
// end of its prompt.
func more(prompt string) string {
	return strings.Repeat(" ", max(ansi.StringWidth(prompt)-4, 0)) + "... "
}

// ReadEntry reads one entry of a console at a terminal, as a shell reads a
// line: the prompt, then what the person types, ended by Enter, and left on
// screen after the prompt. Up and Down step through the history; Ctrl-C clears
// the entry, or on an empty one shows opts.Leave; Ctrl-D on an empty entry is
// io.EOF. The program ends with the entry, so whatever runs next has the
// terminal to itself. Callers pass stdout as out.
func ReadEntry(ctx context.Context, in io.Reader, out io.Writer, opts EntryOptions) (string, error) {
	styles := NewTerminalStyles(out)
	styles.Accent = styles.Accent.Bold(true) // the prompt: where each entry starts, found scrolling back
	ta := textarea.New()
	ta.SetPromptFunc(ansi.StringWidth(opts.Prompt), func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return opts.Prompt
		}
		return more(opts.Prompt)
	})
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 20
	ta.SetWidth(80)
	plain := textarea.StyleState{Base: lipgloss.NewStyle(), Text: lipgloss.NewStyle(), CursorLine: lipgloss.NewStyle(), EndOfBuffer: lipgloss.NewStyle(), Prompt: styles.Accent, Placeholder: styles.Muted}
	taStyles := textarea.DefaultDarkStyles()
	taStyles.Focused, taStyles.Blurred = plain, plain
	ta.SetStyles(taStyles)
	// Enter starts a line only where opts.Ends says the entry goes on.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("enter"))

	m := &entryReader{input: ta, opts: opts, styles: styles, browsing: len(opts.History)}
	programOpts, finishOutput := terminalOptions(in, out)
	programOpts = append(programOpts, tea.WithContext(ctx))
	_, err := tea.NewProgram(m, programOpts...).Run()
	err = errors.Join(err, finishOutput())
	switch {
	case err != nil && ctx.Err() != nil:
		return "", ctx.Err()
	case err != nil:
		return "", err
	case m.eof:
		return "", io.EOF
	}
	return m.input.Value(), nil
}

type entryReader struct {
	input    textarea.Model
	opts     EntryOptions
	styles   TerminalStyles
	browsing int    // the history entry shown; len(History) for the one being typed
	draft    string // the entry being typed, kept while browsing
	leaving  bool   // Ctrl-C on an empty entry: show opts.Leave
	width    int    // the terminal's, for the frame's rules
	done     bool
	eof      bool
}

// The terminal's background, asked once a process, by the first entry's
// reader, for the band behind entries left on screen: unknown until the
// terminal answers, and for good when it doesn't (some, and tmux, don't).
var terminalBackground struct {
	sync.Mutex
	asked bool
	dark  *bool
}

// entryBand is the background behind an entry left on screen, as Claude Code
// shows a message sent: the design package's dark raised surface (ink-825) on
// a dark terminal, its chrome (surface-chrome) on a light one. ok is false
// when the terminal's background is unknown, or without styles: no band.
func entryBand(styles TerminalStyles) (band lipgloss.Style, ok bool) {
	terminalBackground.Lock()
	dark := terminalBackground.dark
	terminalBackground.Unlock()
	if dark == nil || styles.Muted.Render("x") == "x" {
		return band, false
	}
	if *dark {
		return lipgloss.NewStyle().Background(lipgloss.Color("#26232c")), true
	}
	return lipgloss.NewStyle().Background(lipgloss.Color("#f2f0eb")), true
}

func (m *entryReader) Init() tea.Cmd {
	terminalBackground.Lock()
	ask := !terminalBackground.asked && m.styles.Muted.Render("x") != "x"
	terminalBackground.asked = terminalBackground.asked || ask
	terminalBackground.Unlock()
	if ask {
		return tea.Batch(m.input.Focus(), tea.RequestBackgroundColor)
	}
	return m.input.Focus()
}

func (m *entryReader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		dark := msg.IsDark()
		terminalBackground.Lock()
		terminalBackground.dark = &dark
		terminalBackground.Unlock()
		return m, nil
	case tea.WindowSizeMsg:
		m.input.SetWidth(msg.Width)
		m.width = msg.Width
	case tea.KeyPressMsg:
		m.leaving = false
		switch msg.String() {
		case "enter":
			if m.opts.Ends == nil || m.opts.Ends(m.input.Value()) {
				m.done = true
				return m, tea.Quit
			}
		case "alt+enter", "ctrl+enter":
			m.done = true
			return m, tea.Quit
		case "ctrl+c":
			m.leaving = m.input.Value() == ""
			m.input.Reset()
			m.browsing = len(m.opts.History)
			return m, nil
		case "ctrl+d":
			if m.input.Value() == "" {
				m.eof = true
				return m, tea.Quit
			}
		case "up":
			if m.input.Line() == 0 && m.browsing > 0 {
				m.step(-1)
				return m, nil
			}
		case "down":
			if m.input.Line() == m.input.LineCount()-1 && m.browsing < len(m.opts.History) {
				m.step(1)
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// step shows the history entry by entries away, the entry being typed after
// the last.
func (m *entryReader) step(by int) {
	if m.browsing == len(m.opts.History) {
		m.draft = m.input.Value()
	}
	m.browsing += by
	if m.browsing == len(m.opts.History) {
		m.input.SetValue(m.draft)
	} else {
		m.input.SetValue(m.opts.History[m.browsing])
	}
	m.input.MoveToEnd()
}

func (m *entryReader) View() tea.View {
	if m.done || m.eof {
		// What stays on screen: the prompt and the entry, as typed, on a band
		// the terminal's width where its background is known, so each entry is
		// found scrolling back.
		lines := strings.Split(m.input.Value(), "\n")
		band, ok := entryBand(m.styles)
		for i, l := range lines {
			prompt := more(m.opts.Prompt)
			if i == 0 {
				prompt = m.opts.Prompt
			}
			if !ok {
				lines[i] = m.styles.Accent.Render(prompt) + l
				continue
			}
			pad := strings.Repeat(" ", max(m.width-ansi.StringWidth(prompt+l), 0))
			lines[i] = m.styles.Accent.Inherit(band).Render(prompt) + band.Render(l+pad)
		}
		return tea.NewView(strings.Join(lines, "\n") + "\n")
	}
	// While typing: the entry framed by a rule above and below, as Claude
	// Code frames its input, then one line, there even when blank, so a badge
	// coming or going shifts nothing. The leave hint takes the badge's place
	// until the next key.
	width := m.width
	if width <= 0 {
		width = 80 // until the terminal says, as the text area assumes
	}
	rule := m.styles.Muted.Render(strings.Repeat("─", width))
	status := m.opts.Status
	if m.leaving && m.opts.Leave != "" {
		status = m.styles.Muted.Render(m.opts.Leave)
	}
	return tea.NewView(rule + "\n" + m.input.View() + "\n" + rule + "\n" + m.bottom(width, status))
}

// bottom is the line under the frame: the warning and the context on the
// left, status on the right, two spaces apart; to fit width, the context is
// cut short first, then status, then the warning.
func (m *entryReader) bottom(width int, status string) string {
	const gap = 2
	warning := ansi.Truncate(m.opts.Warning, width, "…")
	if warning != "" && status != "" {
		if room := width - ansi.StringWidth(warning) - gap; room > 1 {
			status = ansi.Truncate(status, room, "…")
		} else {
			status = ""
		}
	} else {
		status = ansi.Truncate(status, width, "…")
	}
	room := width - ansi.StringWidth(warning) - ansi.StringWidth(status)
	if warning != "" {
		room -= gap
	}
	if status != "" {
		room -= gap
	}
	left := warning
	if m.opts.Context != "" && room > 1 {
		if warning != "" {
			left += strings.Repeat(" ", gap)
		}
		left += ansi.Truncate(m.opts.Context, room, m.styles.Muted.Render("…"))
	}
	line := left
	if status != "" {
		spaces := width - ansi.StringWidth(left) - ansi.StringWidth(status)
		if left != "" {
			spaces = max(spaces, 1)
		}
		line += strings.Repeat(" ", max(spaces, 0)) + status
	}
	if strings.TrimSpace(ansi.Strip(line)) == "" {
		return "\u00a0" // a blank line, kept: Bubble Tea drops a last line of spaces
	}
	return line
}
