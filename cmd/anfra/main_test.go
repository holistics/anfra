package main

import (
	"bytes"
	"encoding/json/jsontext"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
)

// The tests run as the binary does by default (main): causes shown, since
// ANFRA_HIDE_ERROR_CAUSES is unset.
func TestMain(m *testing.M) {
	apperr.DisableErrorEncapsulation()
	os.Exit(m.Run())
}

// Runs in one process start from the flags' defaults: a run with --json and
// --jq leaves the next one writing YAML, unfiltered, as a console needs.
func TestRunsInOneProcessStartClean(t *testing.T) {
	var first, second, stderr bytes.Buffer
	if code := run(t.Context(), []string{"version", "--json", "--jq", ".version"}, &first, &stderr, runOptions{}); code != 0 {
		t.Fatalf("first run: exit %d: %s", code, &stderr)
	}
	if got := strings.TrimSpace(first.String()); strings.ContainsAny(got, "{:\n") {
		t.Fatalf("first run not filtered to the version: %q", got)
	}
	types := len(resultTypes)
	if code := run(t.Context(), []string{"version"}, &second, &stderr, runOptions{}); code != 0 {
		t.Fatalf("second run: exit %d: %s", code, &stderr)
	}
	if !strings.HasPrefix(second.String(), "version: ") {
		t.Fatalf("second run not the YAML answer: %q", &second)
	}
	if len(resultTypes) != types {
		t.Fatalf("result types grew from %d to %d", types, len(resultTypes))
	}
}

// run is the exit code main exits with: 1 for a failure, its error on stderr.
func TestRunExitCodes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"nope"}, &stdout, &stderr, runOptions{}); code != 1 {
		t.Fatalf("unknown command: exit %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown_command") {
		t.Fatalf("unknown command's error: %q", &stderr)
	}
}

// A console's run colors only at a terminal: into anything else, its output is
// a shell's, but for JSON's indentation, a line a document still a document.
func TestColorOnlyAtATerminal(t *testing.T) {
	t.Cleanup(func() { output.inConsole = false }) // run leaves it as its last run had it
	// YAML or JSON asked for: a console's run is otherwise pretty.
	for _, args := range [][]string{{"version", "--yaml"}, {"version", "--json"}, {"nope", "--yaml"}, {"nope", "--json"}} {
		var plainOut, plainErr, consoleOut, consoleErr bytes.Buffer
		run(t.Context(), args, &plainOut, &plainErr, runOptions{})
		run(t.Context(), args, &consoleOut, &consoleErr, runOptions{inConsole: true})
		got, want := consoleOut.String()+consoleErr.String(), plainOut.String()+plainErr.String()
		if slices.Contains(args, "--json") {
			got, want = compactJSONLines(t, got), compactJSONLines(t, want)
		}
		if got != want {
			t.Errorf("%q colored off a terminal:\n%q\n%q", args, got, want)
		}
	}
}

// A console's JSON is indented: each document over lines, by 2.
func TestConsoleJSONIndented(t *testing.T) {
	t.Cleanup(func() { output.inConsole = false }) // run leaves it as its last run had it
	var out, errOut bytes.Buffer
	run(t.Context(), []string{"version", "--json"}, &out, &errOut, runOptions{inConsole: true})
	if !strings.HasPrefix(out.String(), "{\n  \"version\": ") {
		t.Fatalf("not indented: %q", &out)
	}
}

// compactJSONLines is s's JSON documents, one a line, compact.
func compactJSONLines(t *testing.T, s string) string {
	t.Helper()
	dec := jsontext.NewDecoder(strings.NewReader(s))
	var b strings.Builder
	for {
		v, err := dec.ReadValue()
		if err != nil {
			break
		}
		_ = v.Compact()
		b.Write(v)
		b.WriteByte('\n')
	}
	return b.String()
}

// In a console every command writes the pretty format, a core command without
// a reviewed view too (a trial); --yaml still gives YAML.
func TestConsoleAllPretty(t *testing.T) {
	t.Cleanup(func() { output.inConsole = false })
	var out, errOut bytes.Buffer
	run(t.Context(), []string{"version"}, &out, &errOut, runOptions{inConsole: true})
	if !strings.HasPrefix(out.String(), "Version:  ") {
		t.Errorf("version in a console: %q", &out)
	}
	out.Reset()
	run(t.Context(), []string{"version", "--yaml"}, &out, &errOut, runOptions{inConsole: true})
	if !strings.HasPrefix(out.String(), "version: ") {
		t.Errorf("version --yaml in a console: %q", &out)
	}
}
