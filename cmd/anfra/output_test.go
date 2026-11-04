package main

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// resetOutput clears what a run's --json and --non-interactive set, which
// outlives the run: a test that runs a command with them cleans up after it.
func resetOutput() {
	output.yaml, output.json, output.schema, output.jq, output.nonInteractive = false, false, false, "", false
	jqCode = nil
}

// With --json, an answer is its op's JSON as it is, one document a line.
func TestJSONAnswer(t *testing.T) {
	t.Cleanup(resetOutput)
	output.json = true
	var out bytes.Buffer
	if err := presentTo(registered(t, "version"), []byte(`{ "version": "1.2.3" }`), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != `{"version":"1.2.3"}`+"\n" {
		t.Errorf("answer = %q", got)
	}
}

// With --json, a warning and an error are JSONL documents, each keyed as its
// YAML document is, the error's fields in the wire's order and with the CLI's
// arg and help.
func TestJSONWarningAndError(t *testing.T) {
	t.Cleanup(resetOutput)
	output.json = true
	var out bytes.Buffer
	printWarning(&out, "deprecated", "--dry-run is deprecated: use --list.")
	err := apperr.NewWith(apperr.ValidationFailed, "bad dataset",
		apperr.Violate(apperr.Violation{Field: "dataset", Code: "invalid", Message: "no such dataset"}))
	printError(&out, &commandError{path: "anfra query", args: registered(t, "query").Args(), err: err}, "")

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got:\n%s", out.String())
	}
	if lines[0] != `{"warning":{"code":"deprecated","message":"--dry-run is deprecated: use --list."}}` {
		t.Errorf("warning = %s", lines[0])
	}
	var doc struct {
		Error struct {
			Code    string `json:"code"`
			Help    string `json:"help"`
			Details struct {
				Violations []struct {
					Arg string `json:"arg"`
				} `json:"violations"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := jsonkit.Unmarshal([]byte(lines[1]), &doc); err != nil {
		t.Fatalf("error line is not JSON: %v\n%s", err, lines[1])
	}
	if !strings.HasPrefix(lines[1], `{"error":{"code":"validation_failed",`) ||
		len(doc.Error.Details.Violations) != 1 || doc.Error.Details.Violations[0].Arg != "--dataset" ||
		doc.Error.Help != "Run `anfra query --help` for usage." {
		t.Errorf("error = %s", lines[1])
	}
}

// Every runnable command has an output kind: structured output, or an
// artifact. A new command can't print prose unnoticed.
func TestEveryCommandHasAnOutputKind(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Runnable() {
			switch c.Annotations[outputKind] {
			case outputStructured, outputArtifact:
			default:
				t.Errorf("%s has no output kind: markStructuredOutput, markPrettyOutput or markArtifactOutput", c.CommandPath())
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newRootCmd())
}

// --json and --yaml are refused before the command runs by an artifact, which
// writes no structured output, saying what it writes instead. An export takes
// them only with --link. The two together are refused anywhere.
func TestJSONRefused(t *testing.T) {
	t.Cleanup(resetOutput)
	t.Chdir(t.TempDir())
	for args, want := range map[string]string{
		"serve --json":           "anfra serve writes a log, until it is stopped, not structured output",
		"completion bash --json": "anfra completion bash writes a shell script, not structured output",
		"serve --yaml":           "anfra serve writes a log, until it is stopped, not structured output",
		"query export --json":    "without --link it writes the file",
		"status --yaml --json":   "pass --yaml or --json, not both",
	} {
		cmd := newRootCmd()
		cmd.SetArgs(strings.Fields(args))
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		if apperr.From(err).Code != apperr.ValidationFailed.Code() || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", args, err)
		}
	}
	if entries, _ := os.ReadDir("."); len(entries) != 0 {
		t.Errorf("a refused command ran: %v", entries)
	}
}

// openapi writes its document as JSON with --json.
func TestOpenAPIAsJSON(t *testing.T) {
	t.Cleanup(resetOutput)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"openapi", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := jsonkit.Unmarshal(out.Bytes(), &spec); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if spec["openapi"] == nil || spec["paths"] == nil {
		t.Errorf("spec = %.80s…", out.String())
	}
}

// --no-interactive is --non-interactive under another name.
func TestNoInteractiveIsNonInteractive(t *testing.T) {
	t.Cleanup(resetOutput)
	for _, flag := range []string{"--non-interactive", "--no-interactive"} {
		resetOutput()
		cmd := newRootCmd()
		cmd.SetArgs([]string{"openapi", flag})
		cmd.SetOut(&bytes.Buffer{})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s: %v", flag, err)
		}
		if !output.nonInteractive || stderrIsInteractive() {
			t.Errorf("%s did not make the run non-interactive", flag)
		}
	}
}

// An error printed before the flags are parsed, or because they could not be,
// still follows --json.
func TestJSONAsked(t *testing.T) {
	for args, want := range map[string]bool{
		"query --json":         true,
		"query --json=true":    true,
		"query":                false,
		"query -- --json":      false,
		"query --json=false":   false,
		"--json query --nope":  true,
		"query --jsonish":      false,
		"query --sql --json x": true,
	} {
		if got := jsonAsked(strings.Fields(args)); got != want {
			t.Errorf("jsonAsked(%q) = %v", args, got)
		}
	}
}

// An answer's YAML is its JSON's shape: the op's field order, its numbers as
// written, and its strings strings, a number's digits kept as one quoted.
func TestAnswerYAMLKeepsTheShape(t *testing.T) {
	var out bytes.Buffer
	body := []byte(`{"server": {"state": "running", "port": 9007199254740993, "row_id": "9007199254740993"}, "repo": {"id": "r1", "ratio": 1.50, "tags": ["b", "a"], "on": "yes", "empty": {}}}`)
	if err := renderTo(body, "application/json", &out); err != nil {
		t.Fatal(err)
	}
	want := "server:\n" +
		"  state: running\n" +
		"  port: 9007199254740993\n" +
		"  row_id: \"9007199254740993\"\n" +
		"repo:\n" +
		"  id: r1\n" +
		"  ratio: 1.50\n" +
		"  tags:\n" +
		"    - b\n" +
		"    - a\n" +
		"  \"on\": \"yes\"\n" +
		"  empty: {}\n"
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}

// With --json, a forwarded error is the server's, in its order, with the CLI's
// additions after its fields.
func TestJSONRemoteErrorKeepsTheShape(t *testing.T) {
	t.Cleanup(resetOutput)
	output.json = true
	re, ok := remoteErrorFrom([]byte(`{"error": {"code": "validation_failed", "scope": "user", "status": 422, "message": "x",
		"details": {"violations": [{"field": "dataset", "code": "invalid", "message": "x"}]}, "request_id": "req_1"}}`))
	if !ok {
		t.Fatal("not an error answer")
	}
	var out bytes.Buffer
	printError(&out, &commandError{path: "anfra query", args: registered(t, "query").Args(), err: re}, "")
	want := `{"error":{"code":"validation_failed","scope":"user","status":422,"message":"x",` +
		`"details":{"violations":[{"field":"dataset","code":"invalid","message":"x","arg":"--dataset"}]},` +
		`"request_id":"req_1","help":"Run ` + "`anfra query --help`" + ` for usage."}}` + "\n"
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}

// A CLI-only command's result, asked for, is written as an answer is: YAML in its fields'
// order, or one JSON document; a field it leaves out is absent from both.
func TestWriteStructured(t *testing.T) {
	t.Cleanup(resetOutput)
	res := updateResult{Message: "anfra 0.16.0 is available (you have 0.15.0).", Version: "0.15.0", Update: command.UpdateInfo{Latest: "0.16.0", Available: true},
		Changelog: "https://github.com/holistics/anfra/blob/v0.16.0/CHANGELOG.md"}
	var out bytes.Buffer
	if err := writeStructured(&out, res); err != nil {
		t.Fatal(err)
	}
	want := "message: anfra 0.16.0 is available (you have 0.15.0).\n" +
		"version: 0.15.0\n" +
		"update:\n" +
		"  latest: 0.16.0\n" +
		"  available: true\n" +
		"changelog: https://github.com/holistics/anfra/blob/v0.16.0/CHANGELOG.md\n"
	if out.String() != want {
		t.Errorf("YAML:\n%s\nwant\n%s", out.String(), want)
	}

	output.json = true
	out.Reset()
	res.Message, res.Version, res.Previous, res.Update.Available = "Updated anfra 0.15.0 → 0.16.0.", "0.16.0", "0.15.0", false
	res.Hints = []string{"Restart it."}
	if err := writeStructured(&out, res); err != nil {
		t.Fatal(err)
	}
	want = `{"message":"Updated anfra 0.15.0 → 0.16.0.","version":"0.16.0","update":{"latest":"0.16.0","available":false},"previous":"0.15.0",` +
		`"changelog":"https://github.com/holistics/anfra/blob/v0.16.0/CHANGELOG.md","hints":["Restart it."]}` + "\n"
	if out.String() != want {
		t.Errorf("JSON:\n%s\nwant\n%s", out.String(), want)
	}
}

// An action's preflight is its check subcommand; the former --check still
// works, warning.
func TestPreflightsAreCheckCommands(t *testing.T) {
	root := newRootCmd()
	for _, path := range [][]string{{"publish", "check"}, {"update", "check"}, {"agent", "setup", "check"}} {
		c, _, err := root.Find(path)
		if err != nil || c.Name() != "check" || c.Annotations[outputKind] != outputStructured {
			t.Errorf("%v: %v %v", path, c, err)
		}
	}
	for _, path := range [][]string{{"update"}, {"agent", "setup"}} {
		c, _, _ := root.Find(path)
		if f := c.Flags().Lookup("check"); f == nil || !f.Hidden {
			t.Errorf("%v: --check is gone, or shown", path)
		}
	}
}

// Every command that writes structured output has a schema: a core command's
// from its op, a CLI-only command's from its result type (resultIs). openapi's
// result is an OpenAPI document, described as one.
func TestEveryStructuredCommandHasASchema(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.Runnable() && c.Annotations[outputKind] == outputStructured && c.Name() != "openapi" {
			_, typed := resultTypes[c]
			if _, err := schemaOf(c); err != nil || c.Annotations[opAnnotation] == "" && !typed {
				t.Errorf("%s: no schema of its result (%v): give it resultIs", c.CommandPath(), err)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newRootCmd())
}

// --schema writes a command's input and output instead of running it, a field
// that isn't a flag marked x-cli, as its cli tag; an artifact refuses it.
func TestSchema(t *testing.T) {
	t.Cleanup(resetOutput)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"query", "--schema", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var s struct {
		Command string `json:"command"`
		Input   struct {
			Properties map[string]struct {
				CLI []string `json:"x-cli"`
			} `json:"properties"`
		} `json:"input"`
		Output struct {
			Properties map[string]any `json:"properties"`
		} `json:"output"`
	}
	if err := jsonkit.Unmarshal(out.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if s.Command != "anfra query" || !slices.Equal(s.Input.Properties["query"].CLI, []string{"positional", "stdin"}) || s.Input.Properties["data_source"].CLI != nil || s.Output.Properties["columns"] == nil {
		t.Errorf("schema: %s", out.String())
	}
	if !strings.HasPrefix(out.String(), `{"command":"anfra query","input":{"type":"object",`) {
		t.Errorf("not in order: %.80s", out.String())
	}

	resetOutput()
	cmd = newRootCmd()
	cmd.SetArgs([]string{"serve", "--schema"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "has no schema") {
		t.Errorf("serve --schema: %v", err)
	}
}

// A flag by its field's name works too: --data_source as --data-source.
func TestUnderscoreFlags(t *testing.T) {
	cmd := newRootCmd()
	q, _, _ := cmd.Find([]string{"query"})
	if err := q.ParseFlags([]string{"--data_source", "warehouse"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := q.Flags().GetString("data-source"); v != "warehouse" {
		t.Errorf("--data_source set data-source to %q", v)
	}
}

// --jq filters an answer before it's written: scalars as plain lines, as jq -r
// writes them; objects as YAML documents; with --json, a JSON line each. A big
// integer keeps every digit.
func TestJQ(t *testing.T) {
	t.Cleanup(resetOutput)
	body := []byte(`{"orgs": [{"url": "https://a.example/o/x", "n": 9007199254740993}, {"url": "https://b.example/o/y", "n": 1}]}`)
	for _, tc := range []struct {
		expr string
		json bool
		want string
	}{
		{".orgs[].url", false, "https://a.example/o/x\nhttps://b.example/o/y\n"},
		{".orgs[0].n", false, "9007199254740993\n"},
		{".orgs[] | {url}", false, "---\nurl: https://a.example/o/x\n---\nurl: https://b.example/o/y\n"},
		{".orgs[].url", true, "\"https://a.example/o/x\"\n\"https://b.example/o/y\"\n"},
		{".orgs | length", true, "2\n"},
	} {
		resetOutput()
		output.json = tc.json
		if err := compileJQ(tc.expr); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := renderTo(body, "application/json", &out); err != nil {
			t.Fatalf("%s: %v", tc.expr, err)
		}
		if out.String() != tc.want {
			t.Errorf("%s (json %v):\n%s\nwant\n%s", tc.expr, tc.json, out.String(), tc.want)
		}
	}

	// A filter that fails on the answer is an invalid --jq.
	resetOutput()
	_ = compileJQ(".orgs[0].url[]")
	if err := renderTo(body, "application/json", &bytes.Buffer{}); apperr.From(err).Code != apperr.ValidationFailed.Code() {
		t.Errorf("failing filter: %v", err)
	}
}

// An expression that doesn't parse is refused before the command runs.
func TestJQRefusedBeforeRunning(t *testing.T) {
	t.Cleanup(resetOutput)
	t.Chdir(t.TempDir())
	cmd := newRootCmd()
	cmd.SetArgs([]string{"init", "--jq", ".["})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if apperr.From(err).Code != apperr.ValidationFailed.Code() || !strings.Contains(err.Error(), "--jq") {
		t.Errorf("err = %v", err)
	}
	if entries, _ := os.ReadDir("."); len(entries) != 0 {
		t.Errorf("init ran: %v", entries)
	}
}

// --pretty is for a command with a pretty view, alone: refused elsewhere and
// beside another format, before the command runs.
func TestPrettyRefused(t *testing.T) {
	for args, want := range map[string]string{
		"validate --pretty":        "validate has no pretty view yet",
		"query x --pretty --json":  "pass one format",
		"query x --pretty --jq .a": "pass one format",
		"serve --pretty":           "writes a log",
	} {
		var stdout, stderr bytes.Buffer
		if code := run(t.Context(), strings.Fields(args), &stdout, &stderr, runOptions{}); code == 0 || !strings.Contains(stderr.String(), want) {
			t.Errorf("%s: exit %d: %s", args, code, &stderr)
		}
	}
}

// query's answer, pretty: its rows as a grid labelled by its columns, from its
// JSON, as a server would send it; YAML in a console when asked for it.
func TestQueryPretty(t *testing.T) {
	saved := output
	t.Cleanup(func() { output = saved })
	output = saved
	output.json, output.jq = false, "" // whatever an earlier test left
	q := registered(t, "query")
	body := []byte(`{"sql":"select","columns":[{"name":"s","fieldName":"s","label":"Status","adhoc":false,"isMeasure":false}],` +
		`"result":{"fields":["s"],"records":[["delivered"]]},"has_more":false,"timings":{"steps":[],"end_ms":3}}`)
	for _, tc := range []struct {
		pretty, inConsole, yaml bool
		want                    string
	}{
		{pretty: true, want: "Status"},
		{inConsole: true, want: "Status"},
		{inConsole: true, yaml: true, want: "sql: select"},
		{want: "sql: select"},
	} {
		output.pretty, output.inConsole, output.yaml = tc.pretty, tc.inConsole, tc.yaml
		var out bytes.Buffer
		if err := presentTo(q, body, &out); err != nil || !strings.Contains(out.String(), tc.want) {
			t.Errorf("pretty=%v inConsole=%v yaml=%v: %v\n%s", tc.pretty, tc.inConsole, tc.yaml, err, &out)
		}
	}
}
