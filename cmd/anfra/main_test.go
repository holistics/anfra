package main

import (
	"bytes"
	"os"
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
	if code := run(t.Context(), []string{"version", "--json", "--jq", ".version"}, &first, &stderr); code != 0 {
		t.Fatalf("first run: exit %d: %s", code, &stderr)
	}
	if got := strings.TrimSpace(first.String()); strings.ContainsAny(got, "{:\n") {
		t.Fatalf("first run not filtered to the version: %q", got)
	}
	types := len(resultTypes)
	if code := run(t.Context(), []string{"version"}, &second, &stderr); code != 0 {
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
	if code := run(t.Context(), []string{"nope"}, &stdout, &stderr); code != 1 {
		t.Fatalf("unknown command: exit %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown_command") {
		t.Fatalf("unknown command's error: %q", &stderr)
	}
}
