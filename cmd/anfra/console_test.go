package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/cliui"
)

func TestSplitArgs(t *testing.T) {
	for in, want := range map[string][]string{
		`validate`:                          {"validate"},
		`  query  -d sales  `:               {"query", "-d", "sales"},
		`query -d sales "orders | count()"`: {"query", "-d", "sales", "orders | count()"},
		`query 'it''s'`:                     {"query", "its"},
		`query "say \"hi\" \n"`:             {"query", `say "hi" \n`},
		`query a\ b`:                        {"query", "a b"},
		`query ''`:                          {"query", ""},
		`publish -m "first: \\ and \$"`:     {"publish", "-m", `first: \ and $`},
		"query 'two\nlines'":                {"query", "two\nlines"},
	} {
		got, err := splitArgs(in)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("splitArgs(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{`query 'abc`, `query "abc`} {
		if _, err := splitArgs(in); err == nil || !strings.Contains(err.Error(), "isn't closed") {
			t.Errorf("splitArgs(%q): %v, want a quote not closed", in, err)
		}
	}
}

func TestConsoleArgs(t *testing.T) {
	for _, tc := range []struct {
		entry string
		args  []string
		act   consoleAct
		msg   string // a part of the message
	}{
		{entry: `validate --json`, args: []string{"validate", "--json"}},
		{entry: `\validate --json`, args: []string{"validate", "--json"}},
		{entry: `orders | count()`, args: []string{"orders", "|", "count()"}},
		{entry: `\q`, act: actLeave},
		{entry: `exit`, act: actLeave},
		{entry: `quit`, act: actLeave},
		{entry: `\exit`, act: actLeave},
		{entry: `\?`, act: actHelp},
		{entry: `\`, act: actHelp},
		{entry: `\console`, act: actSay, msg: "in the console already"},
		{entry: `c`, act: actSay, msg: "in the console already"},
		{entry: `\query 'abc`, act: actSay, msg: "isn't closed"},
	} {
		args, act, msg := consoleArgs(tc.entry)
		if !slices.Equal(args, tc.args) || act != tc.act || !strings.Contains(msg, tc.msg) || (tc.msg == "" && msg != "") {
			t.Errorf("consoleArgs(%q) = %q, %v, %q", tc.entry, args, act, msg)
		}
	}
}

// \? lists the console's commands, then anfra's, in their groups, leaving out
// what can't or needn't run in a console.
func TestConsoleHelp(t *testing.T) {
	got := consoleHelp(cliui.TerminalStyles{})
	for _, want := range []string{"The console's commands:", `\q, exit, quit`, "Core commands:", "  query ", "CLI-only commands:", "  publish ", "--help says more"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"  console ", "  help ", "  completion "} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%q listed in:\n%s", unwanted, got)
		}
	}
}

func TestConsoleNeedsATerminal(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run(t.Context(), []string{"console"}, &stdout, &stderr, runOptions{}); code != 1 || !strings.Contains(stderr.String(), "precondition_failed") {
		t.Fatalf("console without a terminal: exit %d: %s", code, stderr.String())
	}
}
