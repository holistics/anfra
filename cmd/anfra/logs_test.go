package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/logging"
	"github.com/holistics/anfra/internal/repo"
)

// A repo's log, as anfra and its sidecars write it: anfra's records, a
// sidecar's own lines, an anfra-node record.
const sampleLog = `{"ts":"2026-11-16T12:11:57.474+07:00","level":"INFO","msg":"Sidecar ready","service":"anfra-host","repo":"r","pid":7,"event":"sidecar.ready","name":"canal-lake","trace_id":"abc"}
2026/11/16 12:11:57 Canal Lake is ready
{"ts":"2026-11-16T12:11:58.100+07:00","level":"WARN","msg":"Query ran without the cache","service":"anfra-host","repo":"r","pid":7,"event":"query_cache.unavailable","error":"start canal-lake: boom"}
{"level":"error","msg":"sidecar.crashed","pid":9,"repo":"r","service":"anfra-node","ts":"2026-11-16T05:11:59.000Z"}
`

// logsRepo is a repo in a temp folder, the current one, with sampleLog as
// its log, under a home of its own; and its log's path.
func logsRepo(t *testing.T) string {
	t.Helper()
	t.Cleanup(resetOutput)
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	dir := t.TempDir()
	t.Chdir(dir)
	wd, _ := os.Getwd()
	path := filepath.Join(repo.Resolve(wd).LogsDir(), logging.FileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sampleLog), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The pretty format: an entry a line, its fields under it but pid and the
// trace's, a sidecar's line beside them; another service named.
func TestLogsPretty(t *testing.T) {
	logsRepo(t)
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"logs"}, &stdout, &stderr, runOptions{}); code != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	out := stdout.String()
	for _, want := range []string{
		"All 4 entries of ",
		"WARN   Query ran without the cache  query_cache.unavailable",
		"error: start canal-lake: boom",
		"Canal Lake is ready",
		"ERROR  sidecar.crashed  (anfra-node)",
		"name: canal-lake",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, hidden := range []string{"pid", "trace_id", "abc"} {
		if strings.Contains(out, hidden) {
			t.Errorf("shows %q in\n%s", hidden, out)
		}
	}
}

// --level, --event and -n narrow the entries; the result keeps every field
// but repo.
func TestLogsNarrowed(t *testing.T) {
	logsRepo(t)
	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), []string{"logs", "--level", "warn", "-n", "1", "--yaml"}, &stdout, &stderr, runOptions{}); code != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	out := stdout.String()
	if !strings.Contains(out, "The last entry of ") || !strings.Contains(out, "that matches.") || !strings.Contains(out, "msg: sidecar.crashed") ||
		strings.Contains(out, "Query ran without the cache") || strings.Contains(out, "repo:") {
		t.Errorf("the last warning or error:\n%s", out)
	}

	stdout.Reset()
	if code := run(t.Context(), []string{"logs", "--event", "query_cache", "--yaml"}, &stdout, &stderr, runOptions{}); code != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	if out := stdout.String(); !strings.Contains(out, "event: query_cache.unavailable") || strings.Contains(out, "sidecar.ready") || !strings.Contains(out, "pid: 7") {
		t.Errorf("the query cache's events:\n%s", out)
	}

	stdout.Reset()
	if code := run(t.Context(), []string{"logs", "--event", "nosuch", "--yaml"}, &stdout, &stderr, runOptions{}); code != 0 {
		t.Fatalf("exit %d: %s", code, &stderr)
	}
	if out := stdout.String(); !strings.Contains(out, "No entry in ") || !strings.Contains(out, "entries: []") || !strings.Contains(out, "Drop --level") {
		t.Errorf("none matching:\n%s", out)
	}
}

// No Data App's report says why: anfra serve runs here with its reports off,
// or runs here and no app was opened, or runs in another repo, or nowhere.
func TestLogsHintsWhyNoAppReports(t *testing.T) {
	logsRepo(t)
	wd, _ := os.Getwd()
	here := repo.Resolve(wd)
	other := repo.Resolve(t.TempDir())
	serve := func(t *testing.T, r repo.Local, noAppLogs bool) {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"status":"ok","repo_id":%q,"instance_id":"i1"}`, r.ID)
		}))
		t.Cleanup(srv.Close)
		if err := writeRuntime(r, runtimeFile{URL: srv.URL, RepoID: r.ID, RepoDir: r.Dir, InstanceID: "i1", Config: runtimeConfig{NoAppLogs: noAppLogs}}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(runtimePath(r)) })
	}
	hint := func(t *testing.T) string {
		t.Helper()
		t.Cleanup(resetOutput)
		var stdout, stderr bytes.Buffer
		if code := run(t.Context(), []string{"logs", "--event", "app", "--yaml"}, &stdout, &stderr, runOptions{}); code != 0 {
			t.Fatalf("exit %d: %s", code, &stderr)
		}
		return stdout.String()
	}

	t.Run("nowhere", func(t *testing.T) {
		if out := hint(t); !strings.Contains(out, "running, and only it logs") {
			t.Errorf("%s", out)
		}
	})
	t.Run("in another repo", func(t *testing.T) {
		serve(t, other, false)
		if out := hint(t); !strings.Contains(out, "run in this repo") || !strings.Contains(out, "run `anfra logs` there") { // YAML doubles its apostrophes
			t.Errorf("%s", out)
		}
	})
	t.Run("here, no app opened", func(t *testing.T) {
		serve(t, here, false)
		if out := hint(t); !strings.Contains(out, "Open it, use it") {
			t.Errorf("%s", out)
		}
	})
	t.Run("here, its reports off", func(t *testing.T) {
		serve(t, here, true)
		if out := hint(t); !strings.Contains(out, "ANFRA_NO_APP_LOGS=1, so it doesn") {
			t.Errorf("%s", out)
		}
	})
}

func TestLogsRefusals(t *testing.T) {
	logsRepo(t)
	for args, want := range map[string]string{
		"logs -f --json":         "anfra logs doesn't take -f with --json",
		"logs --level loud":      `--level is debug, info, warn or error, not "loud"`,
		"logs --since yesterday": `--since is a duration back from now`,
		"logs -n -1":             "-n is how many entries to show",
	} {
		var stdout, stderr bytes.Buffer
		if code := run(t.Context(), strings.Fields(args), &stdout, &stderr, runOptions{}); code == 0 || !strings.Contains(stderr.String(), want) {
			t.Errorf("%s: exit %d, stderr %s", args, code, &stderr)
		}
		resetOutput()
	}
}

// -f shows each entry written after the ones it showed, laid out as they
// were, until stopped.
func TestLogsFollow(t *testing.T) {
	path := logsRepo(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	stdout := &lockedBuffer{}
	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- run(ctx, []string{"logs", "-f", "-n", "1"}, stdout, &stderr, runOptions{}) }()
	for !strings.Contains(stdout.String(), "then each as it's written") {
		if ctx.Err() != nil {
			t.Fatalf("no first entries: %s / %s", stdout.String(), &stderr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // G304: the test's own log
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"ts":"2026-11-16T12:12:00.000+07:00","level":"INFO","msg":"Closing sidecar","service":"anfra-host","event":"sidecar.closing"}` + "\n")
	_ = f.Close()
	for !strings.Contains(stdout.String(), "  12:12:00") || !strings.Contains(stdout.String(), "Closing sidecar  sidecar.closing") {
		if ctx.Err() != nil {
			t.Fatalf("not followed: %s", stdout.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if code := <-done; code != 0 {
		t.Errorf("exit %d: %s", code, &stderr)
	}
}

// lockedBuffer is a buffer a command writes to while a test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
