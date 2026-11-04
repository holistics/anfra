package main

import (
	"slices"
	"strings"
	"testing"
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
		leave bool
		msg   string // a part of the message
	}{
		{entry: `\validate --json`, args: []string{"validate", "--json"}},
		{entry: `\q`, leave: true},
		{entry: `\exit`, leave: true},
		{entry: `\?`, msg: "The console's commands"},
		{entry: `\`, msg: "The console's commands"},
		{entry: `orders | count()`, msg: "Start a command with \\"},
		{entry: `\console`, msg: "in the console already"},
		{entry: `\query 'abc`, msg: "isn't closed"},
	} {
		args, leave, msg := consoleArgs(tc.entry)
		if !slices.Equal(args, tc.args) || leave != tc.leave || !strings.Contains(msg, tc.msg) || (tc.msg == "" && msg != "") {
			t.Errorf("consoleArgs(%q) = %q, %v, %q", tc.entry, args, leave, msg)
		}
	}
}

func TestConsoleNeedsATerminal(t *testing.T) {
	var stdout, stderr strings.Builder
	if code := run(t.Context(), []string{"console"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "precondition_failed") {
		t.Fatalf("console without a terminal: exit %d: %s", code, stderr.String())
	}
}
