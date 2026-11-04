package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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

// A console's history is kept between sessions: an entry of several lines
// whole, the newest last, only so many, and private to its user.
func TestConsoleHistory(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	first := openConsoleHistory()
	if got := first.load(); len(got) != 0 {
		t.Fatalf("a new history holds %q", got)
	}
	first.add("validate")
	first.add("orders\n| count()")
	if got := openConsoleHistory().load(); !slices.Equal(got, []string{"validate", "orders\n| count()"}) {
		t.Fatalf("the next session's history = %q", got)
	}
	if info, err := os.Stat(first.path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("history file: %v, %v", info, err)
	}

	for i := range 2*historyKept + 1 {
		first.add(fmt.Sprint(i))
	}
	got := openConsoleHistory().load()
	if len(got) != historyKept || got[len(got)-1] != fmt.Sprint(2*historyKept) {
		t.Fatalf("history kept %d entries, the last %q", len(got), got[len(got)-1])
	}
	b, _ := os.ReadFile(first.path)
	if lines := bytes.Count(b, []byte("\n")); lines != historyKept {
		t.Fatalf("the file wasn't cut back: %d lines", lines)
	}
}

// An update renames the running anfra aside, then puts a new one at its
// path, as minio/selfupdate does: told apart, by the path the console started
// from.
func TestReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anfra")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	f := anfraFile{path: path, info: info}
	if f.replaced() {
		t.Fatal("the same file told apart")
	}
	if err := os.Rename(path, filepath.Join(dir, ".anfra.old")); err != nil {
		t.Fatal(err)
	}
	if f.replaced() {
		t.Fatal("replaced, with nothing at the path yet")
	}
	if err := os.WriteFile(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !f.replaced() {
		t.Fatal("a new file at the path not told apart")
	}
}
