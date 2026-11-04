package pretty

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/holistics/anfra/internal/cliui"
)

// testStyles are a terminal's styles, as NewTerminalStyles gives them at one.
var testStyles = cliui.TerminalStyles{
	Heading: lipgloss.NewStyle().Bold(true),
	Muted:   lipgloss.NewStyle().Faint(true),
	Success: lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)),
	Warning: lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)),
	Error:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)),
	Link:    lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("#8a51d4")),
	Key:     lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(12)),
}

var errorTones = Tones{"error.code": "bad", "warning.code": "warn"}

const yamlSample = `---
error:
    code: unknown_command
    scope: client
    message: 'quoted: yes'
    n: "12345678901234567890"
    empty: []
    obj: {}
    nul: null
    url: https://x.example.com/o/a?b=1#c
    text: |-
        two
        lines
    list:
        - a
        - b: 1
          c: true
        - - nested
    "yes": "no"
    tab: "a\tb"
    uni: café ✓
---
warning:
    code: settings_not_sent
`

const jsonSample = `{"error":{"code":"unknown_command","scope":"client","n":12345678901234567890,"nul":null,"url":"https://x.example.com","list":["a",{"b":1,"c":true}],"q":"say \"hi\": {x}"}}
{"version":"0.17.0","update":{"latest":"0.17.0","available":false}}
`

// Coloring adds escape codes only: removed, they leave the text as it was.
func TestColorKeepsTheText(t *testing.T) {
	for name, tc := range map[string]struct {
		in    string
		color func([]byte, cliui.TerminalStyles, Tones) []byte
	}{
		"yaml": {yamlSample, ColorYAML},
		"json": {jsonSample, ColorJSON},
	} {
		got := string(tc.color([]byte(tc.in), testStyles, errorTones))
		if got == tc.in {
			t.Errorf("%s: not colored", name)
		}
		if ansi.Strip(got) != tc.in {
			t.Errorf("%s: the text changed:\n%s", name, ansi.Strip(got))
		}
		if off := string(tc.color([]byte(tc.in), cliui.TerminalStyles{}, errorTones)); off != tc.in {
			t.Errorf("%s: colored without styles: %q", name, off)
		}
	}
}

func TestColorYAML(t *testing.T) {
	got := string(ColorYAML([]byte(yamlSample), testStyles, errorTones))
	for _, want := range []string{
		render(testStyles.Key, "error") + render(testStyles.Muted, ":"), // a key opening a map
		render(testStyles.Key, "list"),                                  // a key opening a list
		render(testStyles.Key, "code"),                                  // a key with a value, alike
		render(testStyles.Error, "unknown_command"),                     // error.code: bad
		render(testStyles.Warning, "settings_not_sent"),                 // warning.code: warn
		render(testStyles.Muted, "null"),
		render(testStyles.Link, "https://x.example.com/o/a?b=1#c"),
		render(testStyles.Muted, "-") + " a",
		" client\n", // a value without a tone, plain
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%q", want, got)
		}
	}
}

func TestColorJSON(t *testing.T) {
	got := string(ColorJSON([]byte(jsonSample), testStyles, errorTones))
	for _, want := range []string{
		render(testStyles.Key, `"error"`),
		render(testStyles.Key, `"update"`),
		render(testStyles.Key, `"code"`),
		render(testStyles.Error, `"unknown_command"`),
		render(testStyles.Muted, "null"),
		render(testStyles.Link, `"https://x.example.com"`),
		`"say \"hi\": {x}"`, // punctuation in a string is the string's
		`"client"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%q", want, got)
		}
	}
}

// render is text in s, as the colorers style a part: s's codes around the
// whole of it (lipgloss underlines each character apart).
func render(s lipgloss.Style, text string) string {
	before, after, _ := strings.Cut(s.Render("x"), "x")
	return before + text + after
}

// Every label is a key, blue as YAML's are, a value's and a section's alike;
// a table's header faint; bold is the message's alone.
func TestSectionLabels(t *testing.T) {
	type versions struct {
		Version string `json:"version"`
	}
	type copyOf struct {
		Status   string     `json:"status"`
		Versions []versions `json:"versions"`
	}
	type result struct {
		Message string `json:"message"`
		Version string `json:"version"`
		Update  struct {
			Latest string `json:"latest"`
		} `json:"update"`
		Copies []copyOf `json:"copies"`
		Hints  []string `json:"hints"`
	}
	v := result{Message: "Done.", Version: "0.17.0", Copies: []copyOf{{Status: "mixed", Versions: []versions{{"0.4.4"}}}}, Hints: []string{"Run it."}}
	r := prettyRenderer{styles: testStyles}
	got := r.doc(buildPrettyDoc(reflect.ValueOf(v)))
	for _, want := range []string{
		render(testStyles.Heading, "Done."),
		render(testStyles.Key, "Version:"),
		render(testStyles.Key, "Update:"),
		render(testStyles.Key, "Latest:"),
		render(testStyles.Key, "Copies:"),
		render(testStyles.Muted, "STATUS"), // a header faint, not a key's blue
		render(testStyles.Key, "Hints:"),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
