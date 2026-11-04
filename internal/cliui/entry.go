package cliui

import (
	"context"
	"errors"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// EntryOptions are what ReadEntry shows and remembers.
type EntryOptions struct {
	Prompt  string   // before the entry: "sales> "
	History []string // earlier entries, oldest first, for Up and Down
	Leave   string   // the hint shown under the prompt after Ctrl-C on an empty entry
}

// ReadEntry reads one entry of a console at a terminal, as a shell reads a
// line: the prompt, then what the person types, ended by Enter, and left on
// screen after the prompt. Up and Down step through the history; Ctrl-C clears
// the entry, or on an empty one shows opts.Leave; Ctrl-D on an empty entry is
// io.EOF. The program ends with the entry, so whatever runs next has the
// terminal to itself. Callers pass stdout as out.
func ReadEntry(ctx context.Context, in io.Reader, out io.Writer, opts EntryOptions) (string, error) {
	styles := NewTerminalStyles(out)
	ta := textarea.New()
	ta.Prompt = opts.Prompt
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 20
	ta.SetWidth(80)
	plain := textarea.StyleState{Base: lipgloss.NewStyle(), Text: lipgloss.NewStyle(), CursorLine: lipgloss.NewStyle(), EndOfBuffer: lipgloss.NewStyle(), Prompt: styles.Accent, Placeholder: styles.Muted}
	taStyles := textarea.DefaultDarkStyles()
	taStyles.Focused, taStyles.Blurred = plain, plain
	ta.SetStyles(taStyles)
	// One entry, one line, in this console: Enter ends it.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithDisabled())

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
	done     bool
	eof      bool
}

func (m *entryReader) Init() tea.Cmd { return m.input.Focus() }

func (m *entryReader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.input.SetWidth(msg.Width)
	case tea.KeyPressMsg:
		m.leaving = false
		switch msg.String() {
		case "enter", "alt+enter", "ctrl+enter":
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
		// What stays on screen: the prompt and the entry, as typed.
		lines := strings.Split(m.input.Value(), "\n")
		lines[0] = m.styles.Accent.Render(m.opts.Prompt) + lines[0]
		return tea.NewView(strings.Join(lines, "\n") + "\n")
	}
	view := m.input.View()
	if m.leaving && m.opts.Leave != "" {
		view += "\n" + m.styles.Muted.Render("  "+m.opts.Leave)
	}
	return tea.NewView(view)
}
