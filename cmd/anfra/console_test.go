package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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

func TestConsoleDispatch(t *testing.T) {
	for _, tc := range []struct {
		mode  []string // the session's mode before the entry
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
		{entry: `\?`, act: actHelp},
		{entry: `\`, act: actHelp},
		{entry: `\query 'abc`, act: actSay, msg: "isn't closed"},
		{entry: `console query`, act: actSay, msg: "ending it with ;;"},
		{entry: `c query --lang sql`, act: actSay, msg: "ending it with ;"},
		{entry: `\c search`, act: actSay, msg: "press Enter"},
		{entry: `c init`, act: actSay, msg: "init takes no text to type. console <command> works with query"},
		{entry: `c version`, act: actSay, msg: "version takes no text to type"},
		{entry: `c sales`, act: actSay, msg: `\-d sales`},
		{entry: `console`, act: actSay, msg: "Already at commands"},
		// in query mode: text is AQL, its ;; left out, after --; a command needs its \
		{mode: []string{"query"}, entry: "orders\n| count();;", args: []string{"query", "--", "orders\n| count()"}},
		{mode: []string{"query"}, entry: `-1;;`, args: []string{"query", "--", "-1"}},
		{mode: []string{"query"}, entry: `\validate`, args: []string{"validate"}},
		{mode: []string{"query"}, entry: `\c`, act: actSay, msg: "Back to commands"},
		{mode: []string{"query", "--lang", "sql"}, entry: `select 1;`, args: []string{"query", "--lang", "sql", "--", "select 1"}},
		// in search mode: text is words
		{mode: []string{"search"}, entry: `revenue by "sales region"`, args: []string{"search", "--", "revenue", "by", "sales region"}},
	} {
		var s consoleSession
		if tc.mode != nil {
			s.switchMode(tc.mode)
			if s.mode == nil {
				t.Fatalf("%q isn't a mode", tc.mode)
			}
		}
		args, act, msg := s.dispatch(tc.entry)
		if !slices.Equal(args, tc.args) || act != tc.act || !strings.Contains(msg, tc.msg) || (tc.msg == "" && msg != "") {
			t.Errorf("%q: dispatch(%q) = %q, %v, %q", tc.mode, tc.entry, args, act, msg)
		}
	}
}

// Enter ends an entry, or starts a line where the mode's language says it
// goes on: AQL until ;;, SQL until ;, outside a string or a comment.
func TestConsoleEnds(t *testing.T) {
	aql, sql, search := consoleSession{}, consoleSession{}, consoleSession{}
	aql.switchMode([]string{"query"})
	sql.switchMode([]string{"query", "-l", "sql"})
	search.switchMode([]string{"search"})
	for _, tc := range []struct {
		s    consoleSession
		text string
		ends bool
	}{
		{consoleSession{}, "validate", true},
		{aql, "orders", false},
		{aql, "metric m = count(orders.id);", false},
		{aql, "orders | count();;", true},
		{aql, "orders | count();;  ", true},
		{aql, "orders | where(orders.note == ';;')", false},
		{aql, "orders // ends with ;;", false},
		{aql, `\validate`, true},
		{aql, "", true},
		{sql, "select 1", false},
		{sql, "select 1;", true},
		{sql, "select ';'", false},
		{sql, "select 1 -- done;", false},
		{search, "revenue", true},
	} {
		if got := tc.s.ends(tc.text); got != tc.ends {
			t.Errorf("%s: ends(%q) = %v", tc.s.prompt(), tc.text, got)
		}
	}
}

// A line of flags sets them for every command that takes them, the entry's
// own winning; \-- lists them, an empty value unsets one.
func TestConsoleFlags(t *testing.T) {
	var s consoleSession
	for _, line := range []string{`-d sales`, `--timezone UTC --json`} {
		if msg := s.setFlags(line); msg != "" {
			t.Fatalf("setFlags(%q): %s", line, msg)
		}
	}
	if got := s.withFlags([]string{"query", "--", "orders"}); !slices.Equal(got, []string{"query", "--dataset=sales", "--timezone=UTC", "--json", "--", "orders"}) {
		t.Errorf("query: %q", got)
	}
	if got := s.withFlags([]string{"query", "-d", "other", "x"}); slices.Contains(got, "--dataset=sales") {
		t.Errorf("the entry's own dataset lost: %q", got)
	}
	if got := s.withFlags([]string{"auth", "status"}); !slices.Equal(got, []string{"auth", "status", "--json"}) {
		t.Errorf("auth status, taking --json only: %q", got)
	}
	if got := s.withFlags([]string{"serve"}); slices.Contains(got, "--json") {
		t.Errorf("serve, an artifact, given --json: %q", got)
	}
	if got := s.context("acme"); got != "acme · -d sales --timezone UTC --json" {
		t.Errorf("context %q", got)
	}
	if got := s.setFlags("--"); !strings.Contains(got, "Set in this session:\n  --dataset    sales") {
		t.Errorf("list: %q", got)
	}
	s.setFlags("--dataset= --json=false")
	if got := s.context("acme"); got != "acme · --timezone UTC" {
		t.Errorf("after unsetting: %q", got)
	}
	for line, want := range map[string]string{
		"--nope 1":   "No command takes --nope",
		"-s pg":      "-s is --data-source",
		"--help":     "No command takes --help",
		"--dataset":  "needs a value",
		"sales":      "isn't a flag",
		"--dataset=": "",
	} {
		if got := s.setFlags(line); !strings.Contains(got, want) || want == "" && got != "" {
			t.Errorf("setFlags(%q) = %q, want %q", line, got, want)
		}
	}
	// A session's --lang decides how a query mode's entries end.
	s.switchMode([]string{"query"})
	if msg := s.setFlags("--lang sql"); !strings.Contains(msg, "Type SQL, ending it with ;") {
		t.Errorf("--lang sql in query mode said %q", msg)
	}
	if !s.ends("select 1;") || s.prompt() != "query sql> " {
		t.Errorf("--lang sql set after the mode: %q", s.prompt())
	}
	// In a text mode, a flag must be one its command takes; a short name is
	// its command's, so -s is --data-source.
	for line, want := range map[string]string{
		"--limit 5": "query doesn't take --limit",
		"-o out":    "query doesn't take -o",
		"-s pg":     "",
	} {
		if got := s.setFlags(line); !strings.Contains(got, want) || want == "" && got != "" {
			t.Errorf("in query mode, setFlags(%q) = %q", line, got)
		}
	}
	if got := s.setFlags("--"); !strings.Contains(got, "Others available:") || strings.Contains(got, "--limit") {
		t.Errorf("query's list: %q", got)
	}

	// A flag the mode gives is the mode's to change: c query --lang sql, then
	// \--lang aql, is query's AQL mode, not a session's --lang it would beat.
	var m consoleSession
	m.switchMode([]string{"query", "--lang", "sql"})
	if msg := m.setFlags("--lang aql"); !strings.Contains(msg, "Type AQL") || m.prompt() != "query> " || len(m.flags) != 0 {
		t.Errorf("\\--lang aql in query sql: %q, prompt %q, flags %v", msg, m.prompt(), m.flags)
	}
	if !slices.Equal(m.mode, []string{"query", "--lang=aql"}) {
		t.Errorf("mode %q", m.mode)
	}
	m.switchMode([]string{"query", "-lsql", "-d", "a"})
	m.setFlags("-l aql -d b")
	if !slices.Equal(m.mode, []string{"query", "--lang=aql", "--dataset=b"}) || len(m.flags) != 0 {
		t.Errorf("mode %q, flags %v", m.mode, m.flags)
	}
}

// \? is where the session is, then anfra's own help for it, then what the
// console adds: the commands in command mode, the mode's command in a mode.
func TestConsoleHelp(t *testing.T) {
	var s consoleSession
	s.setFlags("-d sales")
	got := consoleHelp(&s, "/tmp/acme", "none")
	for _, want := range []string{"Repo:  /tmp/acme\nMode:  commands\nFlags: -d sales (view all with \\--)\n", "Core commands:", "types its text directly: query", "sets a flag"} {
		if !strings.Contains(got, want) {
			t.Errorf("command mode: missing %q in:\n%s", want, got)
		}
	}
	s.switchMode([]string{"query"})
	got = consoleHelp(&s, "/tmp/acme", "none")
	for _, want := range []string{"Repo:  /tmp/acme\nMode:  query AQL\nFlags: -d sales (view all with \\--)\n", "Usage:\n  Type AQL", "ending it with ;;", `\c`, "goes back to commands", "query flags:\n", "--dataset", "Global flags:\n      --jq string"} {
		if !strings.Contains(got, want) {
			t.Errorf("query mode: missing %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"Core commands:", "Answers one page of rows", "--help"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("query mode shows %q:\n%s", unwanted, got)
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

// A flag the mode's command requires and nothing gives is warned of before an
// entry fails for it, by the command's own check: the mode's flags and the
// session's both count.
func TestConsoleMissingFlag(t *testing.T) {
	for _, tc := range []struct {
		mode  []string
		flags string
		want  string
	}{
		{nil, "", ""},
		{[]string{"query"}, "", "-d"},
		{[]string{"query"}, "-d sales", ""},
		{[]string{"query", "-d", "sales"}, "", ""},
		{[]string{"query", "--lang", "sql"}, "", "-s"},
		{[]string{"query", "--lang", "sql"}, "-s pg", ""},
		{[]string{"search"}, "", ""},
		{[]string{"query", "--lang", "sql"}, "-d sales", "-d: a SQL query runs against a data source, not a dataset"},
	} {
		var s consoleSession
		if tc.mode != nil {
			s.switchMode(tc.mode)
		}
		if tc.flags != "" {
			if msg := s.setFlags(tc.flags); msg != "" && !strings.Contains(msg, "Type") {
				t.Fatalf("%q: setFlags(%q): %s", tc.mode, tc.flags, msg)
			}
		}
		_, flag, code, msg := s.flagProblem()
		got := flag
		if code != "required" && flag != "" {
			got = flag + ": " + msg
		}
		if got != tc.want {
			t.Errorf("%q with %q: %q, want %q", tc.mode, tc.flags, got, tc.want)
		}
	}
}

// The mode's own flags are shown where the session's are, and listed apart.
func TestConsoleModeFlagsShown(t *testing.T) {
	var s consoleSession
	s.switchMode([]string{"query", "-d", "a", "--lang", "aql"})
	s.setFlags("--timezone UTC")
	if got := s.context("acme"); got != "acme · -d a --timezone UTC" {
		t.Errorf("context %q", got)
	}
	got := s.setFlags("--")
	if !strings.Contains(got, "Set by the mode:\n  --dataset   a") || !strings.Contains(got, "Set in this session:") || strings.Contains(got, "Others available: --lang --dataset") {
		t.Errorf("list:\n%s", got)
	}
}

// A dataset's or a data source's name is picked out where the flags are
// shown, however its flag was typed.
func TestConsoleContextTargets(t *testing.T) {
	var s consoleSession
	s.switchMode([]string{"query", "-dsales", "--page", "2"})
	s.setFlags("--timezone UTC --json")
	var targets []string
	var text strings.Builder
	for _, p := range s.contextParts("acme", true) {
		text.WriteString(p.text)
		if p.target {
			targets = append(targets, p.text)
		}
	}
	if got := text.String(); got != "acme · query AQL · -dsales --page 2 --timezone UTC --json" {
		t.Errorf("text %q", got)
	}
	if !slices.Equal(targets, []string{"sales"}) {
		t.Errorf("picked out %q", targets)
	}
	var sql consoleSession
	sql.switchMode([]string{"query", "-l", "sql"})
	sql.setFlags("-s warehouse")
	for _, p := range sql.contextParts("acme", false) {
		if p.target && p.text != "warehouse" {
			t.Errorf("picked out %q", p.text)
		}
	}
}

// A warning is a sentence: what's wrong with the flag, and how to fix it.
func TestConsoleWarning(t *testing.T) {
	for _, tc := range []struct {
		mode  []string
		flags string
		want  string
	}{
		{[]string{"query"}, "", `! --dataset is missing. Set it using \-d <value>`},
		{[]string{"query", "-s", "ds"}, "", `! An AQL query runs against a dataset, not a data source. Unset it using \--data-source=`},
		{[]string{"query", "-l", "sql"}, "-d sales", `! A SQL query runs against a data source, not a dataset. Unset it using \--dataset=`},
		{[]string{"query"}, "-d sales", ""},
	} {
		var s consoleSession
		s.switchMode(tc.mode)
		if tc.flags != "" {
			s.setFlags(tc.flags)
		}
		if got := consoleWarning(&s); got != tc.want {
			t.Errorf("%q with %q:\n got %q\nwant %q", tc.mode, tc.flags, got, tc.want)
		}
	}
	// The fix it gives works for a flag the mode was entered with.
	var s consoleSession
	s.switchMode([]string{"query", "-s", "ds"})
	s.setFlags("--data-source=")
	if w := consoleWarning(&s); !strings.Contains(w, "--dataset is missing") {
		t.Errorf("after unsetting the mode's -s: %q", w)
	}
}

// --json and --yaml are alternatives: setting one switches to it.
func TestConsoleFormatSwitch(t *testing.T) {
	var s consoleSession
	s.setFlags("--json -d sales")
	s.setFlags("--yaml")
	if got := s.context("acme"); got != "acme · -d sales --yaml" {
		t.Errorf("after --yaml: %q", got)
	}
	s.setFlags("--yaml=false")
	if got := s.context("acme"); got != "acme · -d sales" {
		t.Errorf("after --yaml=false: %q", got)
	}
}

// A command forwarded to a warm server starts one; one the CLI runs itself,
// or a CLI-only command, doesn't.
func TestRunsOnEngine(t *testing.T) {
	for args, want := range map[string]bool{
		"query": true, "query compile": true, "validate": true, "show": true,
		"version": false, "auth status": false, "init": false, "nope": false,
	} {
		if got := runsOnEngine(strings.Fields(args)); got != want {
			t.Errorf("runsOnEngine(%q) = %v", args, got)
		}
	}
}
