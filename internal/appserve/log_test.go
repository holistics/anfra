package appserve

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/shared/jsonkit"
)

// logged is a Server whose Data Apps' reports are written, as JSON lines, to the returned buffer.
func logged(t *testing.T) (*Server, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil)).With("service", "app")
	return New(Options{RepoDir: repo(t), AppLogger: logger}), &buf
}

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/appserve/log", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	return w
}

// lines are the records written, each a map.
func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := jsonkit.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %s", line)
		}
		delete(m, "time")
		out = append(out, m)
	}
	return out
}

// A runtime error is an error line, a console line at its own level, both with the app's path, as
// "service":"app"; only the fields a report names, each capped.
func TestLog(t *testing.T) {
	s, buf := logged(t)
	for _, body := range []string{
		`{"path":"sales.html","type":"error","event":{"kind":"query","code":"query_failed","scope":"user","message":"column x does not exist",` +
			`"requestId":"req-1","entity":"orders","at":"2026-11-02T00:00:00Z","rows":[[1]]}}`,
		`{"path":"sales.html","type":"console","event":{"level":"warn","message":"careful"}}`,
		`{"path":"team/overview.html","type":"error","event":{"kind":"uncaught","message":"` + strings.Repeat("x", 3000) + `"}}`,
	} {
		if w := post(s, body); w.Code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	got := lines(t, buf)
	if len(got) != 3 {
		t.Fatalf("%d lines, want 3: %v", len(got), got)
	}
	want := map[string]any{
		"level": "ERROR", "msg": "app.error", "service": "app", "app": "sales.html", "kind": "query", "code": "query_failed", "scope": "user",
		"message": "column x does not exist", "request_id": "req-1", "entity": "orders", "at": "2026-11-02T00:00:00Z",
	}
	if a, b := jsonOf(got[0]), jsonOf(want); a != b {
		t.Errorf("error line =\n  %s\nwant\n  %s", a, b)
	}
	if got[1]["level"] != "WARN" || got[1]["msg"] != "app.console" || got[1]["message"] != "careful" {
		t.Errorf("console line = %v", got[1])
	}
	if m, _ := got[2]["message"].(string); !strings.HasSuffix(m, "… (truncated)") || len([]rune(m)) != messageLimit+len([]rune("… (truncated)")) {
		t.Errorf("a long message is not capped: %d runes", len([]rune(m)))
	}
}

func jsonOf(v any) string {
	b, _ := jsonkit.Marshal(v)
	return string(b)
}

// A mount is an info line with the file's modification time, read by the server; a path that
// names no Data App's file is not logged.
func TestLogMounted(t *testing.T) {
	s, buf := logged(t)
	stamp := time.Date(2026, 11, 3, 5, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(s.opts.RepoDir, "apps", "sales.html"), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"path":"sales.html","type":"mounted"}`,
		`{"path":"made/up.html","type":"mounted"}`,
		`{"path":".hidden/private.html","type":"mounted"}`,
		`{"path":"../models/orders.model.aml","type":"mounted"}`,
	} {
		if w := post(s, body); w.Code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	got := lines(t, buf)
	want := []map[string]any{{"level": "INFO", "msg": "app.mounted", "service": "app", "app": "sales.html", "modified": "2026-11-03T05:00:00Z"}}
	if a, b := jsonOf(got), jsonOf(want); a != b {
		t.Errorf("lines =\n  %s\nwant\n  %s", a, b)
	}
}

// What is not a report is refused, and nothing logged.
func TestLogRefuses(t *testing.T) {
	s, buf := logged(t)
	for _, body := range []string{
		`not json`,
		`{"type":"error","event":{"message":"no path"}}`,
		`{"path":"sales.html","type":"other","event":{"message":"x"}}`,
		`{"path":"sales.html","type":"error","event":{"message":"` + strings.Repeat("x", reportBodyLimit) + `"}}`,
	} {
		if w := post(s, body); w.Code != http.StatusBadRequest {
			t.Errorf("%.40s: %d, want 400", body, w.Code)
		}
	}
	if buf.Len() != 0 {
		t.Errorf("logged: %s", buf)
	}
}

// One Data App logs so many reports a minute; the rest are counted, and the count logged once the
// minute is over. Another Data App is not held to the first's.
func TestLogRateLimit(t *testing.T) {
	s, buf := logged(t)
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	s.reports = newLimiter(func() time.Time { return now }, s.logDropped)
	s.reports.afterFunc = func(time.Duration, func()) {}
	report := func(path string) {
		post(s, `{"path":"`+path+`","type":"error","event":{"kind":"uncaught","message":"again"}}`)
	}
	for range reportsPerMinute + 7 {
		report("sales.html")
	}
	report("team/overview.html")
	if got := len(lines(t, buf)); got != reportsPerMinute+1 {
		t.Fatalf("%d lines, want %d", got, reportsPerMinute+1)
	}
	buf.Reset()
	now = now.Add(time.Minute)
	report("sales.html")
	got := lines(t, buf)
	if len(got) != 2 || got[0]["msg"] != "app.reports_dropped" || got[0]["dropped"] != float64(7) || got[1]["msg"] != "app.error" {
		t.Errorf("after the minute: %v", got)
	}
}

// A flood that stops still has its drops logged, when its minute is over.
func TestLogRateLimitFloodStops(t *testing.T) {
	s, buf := logged(t)
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	s.reports = newLimiter(func() time.Time { return now }, s.logDropped)
	var timers []func()
	var waits []time.Duration
	s.reports.afterFunc = func(d time.Duration, f func()) {
		waits = append(waits, d)
		timers = append(timers, f)
	}
	now = now.Add(10 * time.Second)
	for range reportsPerMinute + 3 {
		post(s, `{"path":"sales.html","type":"error","event":{"kind":"uncaught","message":"again"}}`)
	}
	if len(timers) != 1 || waits[0] != time.Minute {
		t.Fatalf("timers %d, waits %v; want one, for the window's minute", len(timers), waits)
	}
	buf.Reset()
	timers[0]()
	got := lines(t, buf)
	if len(got) != 1 || got[0]["msg"] != "app.reports_dropped" || got[0]["dropped"] != float64(3) {
		t.Errorf("at the minute's end: %v", got)
	}
	if len(s.reports.windows) != 0 {
		t.Errorf("the window is kept: %v", s.reports.windows)
	}
}

// Windows whose minute is over are forgotten, so paths a page makes up do not pile up.
func TestLogRateLimitForgets(t *testing.T) {
	s, _ := logged(t)
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	s.reports = newLimiter(func() time.Time { return now }, s.logDropped)
	for i := range 50 {
		post(s, `{"path":"made-up-`+strconv.Itoa(i)+`.html","type":"console","event":{"level":"warn","message":"x"}}`)
	}
	if n := len(s.reports.windows); n != 50 {
		t.Fatalf("%d windows, want 50", n)
	}
	now = now.Add(time.Minute)
	post(s, `{"path":"sales.html","type":"console","event":{"level":"warn","message":"x"}}`)
	if n := len(s.reports.windows); n != 1 {
		t.Errorf("%d windows after a minute, want 1", n)
	}
}
