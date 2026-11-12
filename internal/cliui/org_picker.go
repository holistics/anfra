package cliui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/holistics/anfra/internal/cloudapi"
)

// CloudChoices are the cloud servers login offers besides organization URL entry, and
// the address to email for another region, which login shows under them, linked.
type CloudChoices struct {
	Servers      []cloudapi.CloudServer
	SupportEmail string
}

// supportNote tells whom to email for another region, the address a mailto: link.
func supportNote(styles TerminalStyles, email string) string {
	return styles.Muted.Render("Need another region? Send us an email at ") +
		styles.LinkTo(email, "mailto:"+email) + styles.Muted.Render(".")
}

// OrgChoice is a stored organization's display name and canonical URL, never credentials.
type OrgChoice struct {
	Name string
	URL  string
}

type orgItem struct {
	name, url string
	index     int
	input     bool
	// origin is a cloud server's, which choosing it returns as the address.
	origin string
	// upcoming is a cloud server that isn't available yet: shown, but not selectable.
	upcoming bool
}

func (i orgItem) FilterValue() string { return i.name }

type orgDelegate struct {
	height int
	styles TerminalStyles
}

func (d orgDelegate) Height() int                         { return d.height }
func (d orgDelegate) Spacing() int                        { return 1 }
func (d orgDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d orgDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i := item.(orgItem)
	marker := "  "
	if index == m.Index() {
		marker = d.styles.Accent.Render("> ")
	}
	width := max(1, m.Width()-2)
	text := orgItemText(i, width, d.styles, index == m.Index())
	lines := strings.Split(text, "\n")
	for n, line := range lines {
		if n > 0 {
			_, _ = fmt.Fprint(w, "\n")
			marker = "  "
		}
		_, _ = fmt.Fprint(w, marker+line)
	}
}

func orgItemText(item orgItem, width int, styles TerminalStyles, selected bool) string {
	name := item.name
	if item.upcoming {
		return styles.Muted.Render(ansi.Wrap(name+"  (upcoming)", width, ""))
	}
	if selected {
		name = styles.Selection.Render(name)
	}
	if item.url == "" {
		return ansi.Wrap(name, width, "")
	}
	inline := name + "  " + styles.Muted.Render(item.url)
	if ansi.StringWidth(inline) <= width {
		return inline
	}
	return ansi.Wrap(name, width, "") + "\n" + styles.Muted.Render(ansi.Wrap(item.url, width, ""))
}

type orgPicker struct {
	choices                     list.Model
	input                       textinput.Model
	title                       string
	width, height               int
	editing, selected, finished bool
	index                       int
	address, validation         string
	supportEmail                string
	styles                      TerminalStyles
}

// PickOrg returns the original stored index, or -1 plus a canonical URL: a cloud server's
// origin, or the URL entered. With allowURL, it offers cloud's servers after orgs, showing
// those without an origin as upcoming, and cloud's support email under the list. Esc or
// Ctrl-C returns context.Canceled. Callers own interaction policy and pass stderr as out.
//
//nolint:revive // Keep the shared picker API explicit about cloud selection and URL entry.
func PickOrg(ctx context.Context, in io.Reader, out io.Writer, title string, orgs []OrgChoice, cloud *CloudChoices, allowURL bool) (int, string, error) {
	if cloud == nil || !allowURL {
		cloud = &CloudChoices{}
	}
	items := make([]list.Item, 0, len(orgs)+len(cloud.Servers)+1)
	for index, org := range orgs {
		name := SafeText(org.Name)
		if name == "" {
			if parsed, err := cloudapi.ParseOrg(org.URL); err == nil {
				name = parsed.Slug
			} else {
				name = "Stored organization"
			}
		}
		items = append(items, orgItem{name: name, url: SafeText(org.URL), index: index})
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].(orgItem), items[j].(orgItem)
		if a.url == "" && b.url == "" {
			return false // Keep actions in the caller's order.
		}
		if (a.url == "") != (b.url == "") {
			return a.url != ""
		}
		if a.name == b.name {
			return a.url < b.url
		}
		return a.name < b.name
	})
	if allowURL {
		for _, server := range cloud.Servers {
			item := orgItem{name: SafeText(server.Label), url: SafeText(server.Origin), index: -1, origin: server.Origin, upcoming: server.Origin == ""}
			if item.name == "" {
				item.name, item.url = item.url, ""
			}
			items = append(items, item)
		}
		items = append(items, orgItem{name: "Enter your organization URL", index: -1, input: true})
	}
	if len(items) == 0 {
		return -1, "", errors.New("org picker: no organizations available")
	}
	l := list.New(items, orgDelegate{height: 2}, 80, 12)
	l.SetFilteringEnabled(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.DisableQuitKeybindings()
	l.KeyMap = list.KeyMap{CursorUp: key.NewBinding(key.WithKeys("up")), CursorDown: key.NewBinding(key.WithKeys("down"))}
	t := textinput.New()
	t.Placeholder = "https://bi.example.com/o/acme"
	t.CharLimit = 4096
	styles := NewTerminalStyles(out)
	inputStyles := textinput.StyleState{Prompt: styles.Accent, Placeholder: styles.Muted, Suggestion: styles.Muted}
	t.SetStyles(textinput.Styles{Focused: inputStyles, Blurred: inputStyles})
	titleLines := strings.Split(title, "\n")
	for i := range titleLines {
		titleLines[i] = SafeText(titleLines[i])
	}
	m := &orgPicker{choices: l, input: t, title: strings.Join(titleLines, "\n"), width: 80, height: 20, index: -1, editing: len(items) == 1 && items[0].(orgItem).input, supportEmail: SafeText(cloud.SupportEmail), styles: styles}
	m.skipUpcoming(0, 1)
	m.resize()
	opts, finishOutput := terminalOptions(in, out)
	opts = append(opts, tea.WithContext(ctx))
	_, err := tea.NewProgram(m, opts...).Run()
	err = errors.Join(err, finishOutput())
	if err != nil {
		if ctx.Err() != nil {
			return -1, "", ctx.Err()
		}
		return -1, "", err
	}
	if !m.selected {
		return -1, "", context.Canceled
	}
	return m.index, m.address, nil
}

func (m *orgPicker) Init() tea.Cmd {
	if m.editing {
		return m.input.Focus()
	}
	return nil
}

func (m *orgPicker) resize() {
	width := max(1, m.width-2)
	height := 1
	for _, item := range m.choices.Items() {
		i := item.(orgItem)
		height = max(height, lipgloss.Height(orgItemText(i, width, TerminalStyles{}, false)))
	}
	m.choices.SetDelegate(orgDelegate{height: height, styles: m.styles})
	reserved := lipgloss.Height(ansi.Wrap(m.title, max(1, m.width), "")) + 4
	if m.supportEmail != "" {
		reserved += lipgloss.Height(ansi.Wrap(supportNote(TerminalStyles{}, m.supportEmail), max(1, m.width), "")) + 1
	}
	contentHeight := len(m.choices.Items()) * (height + 1)
	m.choices.SetSize(max(1, m.width), max(height, min(contentHeight, min(12, m.height-reserved))))
	m.input.SetWidth(max(1, m.width-4))
}

func (m *orgPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.finished = true
			return m, tea.Quit
		case "esc":
			if m.editing {
				m.editing = false
				m.input.Blur()
				m.validation = ""
				return m, nil
			}
			m.finished = true
			return m, tea.Quit
		case "enter":
			if m.editing {
				// An org's address, any page's under it, or its server's alone.
				origin, slug, err := cloudapi.ParseAddress(m.input.Value())
				if err != nil {
					m.validation = "Invalid organization URL. Use https://bi.example.com/o/acme."
					return m, nil
				}
				m.index, m.address = -1, origin
				if slug != "" {
					m.address = cloudapi.Org{Origin: origin, Slug: slug}.String()
				}
			} else {
				i := m.choices.SelectedItem().(orgItem)
				if i.input {
					m.editing = true
					return m, m.input.Focus()
				}
				m.index, m.address = i.index, i.origin
			}
			m.selected, m.finished = true, true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	if m.editing {
		m.input, cmd = m.input.Update(msg)
	} else {
		before := m.choices.Index()
		m.choices, cmd = m.choices.Update(msg)
		if after := m.choices.Index(); after != before {
			m.skipUpcoming(before, after-before)
		}
	}
	return m, cmd
}

// skipUpcoming moves the cursor on from an upcoming server, which can't be chosen,
// in the direction it moved (step's sign). With none to land on that way, it goes back to from.
func (m *orgPicker) skipUpcoming(from, step int) {
	if step > 0 {
		step = 1
	} else {
		step = -1
	}
	items := m.choices.Items()
	for i := m.choices.Index(); i >= 0 && i < len(items); i += step {
		if !items[i].(orgItem).upcoming {
			m.choices.Select(i)
			return
		}
	}
	m.choices.Select(from)
}

func (m *orgPicker) View() tea.View {
	if m.finished {
		return tea.NewView("")
	}
	body := m.choices.View() + "\n"
	if m.supportEmail != "" {
		body += supportNote(m.styles, m.supportEmail) + "\n\n"
	}
	body += m.styles.Muted.Render("Up/down: move  Enter: select  Esc: cancel")
	if m.editing {
		body = m.styles.Heading.Render("Organization URL") + "\n" + m.input.View()
		if m.validation != "" {
			body += "\n" + m.styles.Error.Render(m.validation)
		}
		body += "\n" + m.styles.Muted.Render("Enter: confirm  Esc: back  Ctrl-C: cancel")
	}
	title, description, _ := strings.Cut(m.title, "\n")
	heading := m.styles.Heading.Render(title)
	if description != "" {
		heading += "\n" + m.styles.Muted.Render(description)
	}
	return tea.NewView(ansi.Wrap(heading+"\n\n"+body, max(1, m.width), ""))
}
