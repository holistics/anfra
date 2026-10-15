package cliui

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type activityResult struct{ err error }
type activityProgress string
type activityMessage string

type activityModel struct {
	spinner            spinner.Model
	label              string
	messages           []string
	width              int
	finished, canceled bool
	cancel             context.CancelFunc
	run                tea.Cmd
	progress           <-chan activityProgress
	complete           <-chan struct{}
	styles             TerminalStyles
}

// RunActivity runs work in a Tea command and waits for its cleanup even after cancellation or a UI error.
// Progress labels must be bounded and nonsecret. The second callback prints persistent user messages
// above the activity; callers handle plain mode and final output.
func RunActivity(ctx context.Context, in io.Reader, out io.Writer, initial string, work func(context.Context, func(string), func(string)) error) error {
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	progress := make(chan activityProgress, 1)
	var mu sync.Mutex
	var program *tea.Program
	started, stopped := false, false
	run := func() tea.Msg {
		mu.Lock()
		if stopped {
			mu.Unlock()
			return nil
		}
		started = true
		mu.Unlock()
		err := work(workCtx, func(label string) {
			select {
			case progress <- activityProgress(SafeText(label)):
			default:
			}
		}, func(message string) {
			program.Send(activityMessage(SafeText(message)))
		})
		done <- err
		return activityResult{err: err}
	}
	s := spinner.New()
	s.Spinner = spinner.Dot
	styles := NewTerminalStyles(out)
	s.Style = styles.Accent
	m := &activityModel{spinner: s, label: SafeText(initial), width: 80, cancel: cancel, run: run, progress: progress, complete: workCtx.Done(), styles: styles}
	opts := append(terminalOptions(in, out), tea.WithContext(ctx))
	program = tea.NewProgram(m, opts...)
	_, uiErr := program.Run()
	cancel()
	mu.Lock()
	stopped = true
	wasStarted := started
	mu.Unlock()
	var workErr error
	if wasStarted {
		workErr = <-done
	}
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), workErr, uiErr)
	}
	if m.canceled && uiErr == nil && (workErr == nil || workErr == context.Canceled) {
		return context.Canceled
	}
	if m.canceled || !m.finished {
		return errors.Join(context.Canceled, workErr, uiErr)
	}
	return errors.Join(workErr, uiErr)
}

func (m *activityModel) waitProgress() tea.Cmd {
	return func() tea.Msg {
		select {
		case label := <-m.progress:
			return label
		case <-m.complete:
			return nil
		}
	}
}

func (m *activityModel) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, m.run, m.waitProgress()) }
func (m *activityModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || msg.String() == "esc" {
			m.canceled = true
			m.cancel()
			m.label = "Canceling…"
		}
	case activityProgress:
		if !m.canceled {
			m.label = string(msg)
		}
		return m, m.waitProgress()
	case activityMessage:
		words := strings.Fields(string(msg))
		for i, word := range words {
			if strings.HasPrefix(word, "https://") || strings.HasPrefix(word, "http://") {
				words[i] = m.styles.Link.Render(word)
			}
		}
		m.messages = append(m.messages, strings.Join(words, " "))
		return m, nil
	case activityResult:
		m.finished = true
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}
func (m *activityModel) View() tea.View {
	messages := strings.Join(m.messages, "\n")
	if messages != "" {
		messages += "\n"
	}
	if m.finished {
		return tea.NewView(ansi.Wrap(messages, max(1, m.width), ""))
	}
	label := m.styles.Heading.Render(m.label)
	if m.canceled {
		label = m.styles.Warning.Render(m.label)
	}
	body := m.spinner.View() + " " + label + "\n" + m.styles.Muted.Render("Esc/Ctrl-C: cancel")
	if messages != "" {
		body = messages + "\n" + body
	}
	return tea.NewView(ansi.Wrap(body, max(1, m.width), ""))
}
