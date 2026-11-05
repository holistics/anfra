package appserve

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/holistics/anfra/shared/logkit"
)

// A Data App's records carry its path as app. Their msg is what the app said, for an error or a
// console line: the sentence a person reads is the app's own (docs/designs/logs.md).
var (
	eventAppError = logkit.DefineEvent("app.error",
		"A Data App's runtime error, as its SDK host reported it from the browser.",
		"app", "kind", "code", "scope", "request_id", "entity", "call", "stack")
	eventAppConsole = logkit.DefineEvent("app.console",
		"A console line a Data App wrote in the browser (a warning or an error).",
		"app", "stack")
	eventAppMounted = logkit.DefineEvent("app.mounted",
		"The appserve page loaded a Data App; the records after it are of the file as it was then.",
		"app", "modified")
	eventAppReportsDropped = logkit.DefineEvent("app.reports_dropped",
		"A Data App reported more in a minute than the log takes; the ones over were dropped.",
		"app", "dropped", "hint")
)

// Report is what the appserve page posts to /appserve/log: one runtime error, or one console line,
// of the Data App at Path, as the SDK's host reported it (anfra.error, anfra.console), or that the
// page mounted it. It reaches anfra.log, so an agent reads a Data App's failures without a browser,
// and tells the ones from before an edit from the ones after it.
type Report struct {
	// Path is the Data App's, under apps/.
	Path string `json:"path"`
	// Type is "error", "console" or "mounted" (no event: the page loaded the file as it is now).
	Type  string      `json:"type"`
	Event ReportEvent `json:"event"`
}

// ReportEvent holds the fields of the SDK's AppErrorEvent and AppConsoleEvent that are logged.
// Rows and a request's params are never among them.
type ReportEvent struct {
	Kind      string `json:"kind,omitempty"`
	Level     string `json:"level,omitempty"`
	Code      string `json:"code,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
	Entity    string `json:"entity,omitempty"`
	Call      string `json:"call,omitempty"`
	Stack     string `json:"stack,omitempty"`
}

const (
	// reportBodyLimit caps a report's body: the SDK caps its message and stack well under it.
	reportBodyLimit = 32 << 10
	// messageLimit and stackLimit cap what is logged of a report, whatever the page sent.
	messageLimit = 2000
	stackLimit   = 4000
	shortLimit   = 200
	// reportsPerMinute is how many reports one Data App may log in a minute: a page that throws
	// in a loop must not flood the log. The ones over it are counted, and the count logged.
	reportsPerMinute = 300
)

// limiter counts each Data App's reports in the current minute. A window is forgotten once its
// minute is over, so paths the page makes up do not pile up; the reports a window dropped are
// told to onDropped when its minute ends, whether or not the Data App reports again.
type limiter struct {
	mu        sync.Mutex
	now       func() time.Time
	afterFunc func(time.Duration, func())
	onDropped func(path string, dropped int)
	windows   map[string]*window
	lastSweep time.Time
}

type window struct {
	start   time.Time
	count   int
	dropped int
	// timed is set once a timer is to tell this window's drops.
	timed bool
}

func newLimiter(now func() time.Time, onDropped func(path string, dropped int)) *limiter {
	return &limiter{
		now:       now,
		afterFunc: func(d time.Duration, f func()) { time.AfterFunc(d, f) },
		onDropped: onDropped,
		windows:   map[string]*window{},
		lastSweep: now(),
	}
}

// allow says whether path may log one more report now.
func (l *limiter) allow(path string) bool {
	var told []func()
	defer func() {
		for _, tell := range told {
			tell()
		}
	}()
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	if t.Sub(l.lastSweep) >= time.Minute {
		told = l.sweep(t)
		l.lastSweep = t
	}
	w := l.windows[path]
	if w == nil || t.Sub(w.start) >= time.Minute {
		if w != nil && w.dropped > 0 {
			told = append(told, l.tell(path, w))
		}
		w = &window{start: t}
		l.windows[path] = w
	}
	if w.count >= reportsPerMinute {
		w.dropped++
		if !w.timed {
			w.timed = true
			l.afterFunc(w.start.Add(time.Minute).Sub(t), func() { l.flush(path, w) })
		}
		return false
	}
	w.count++
	return true
}

// tell takes w's drops, to be told once the lock is released.
func (l *limiter) tell(path string, w *window) func() {
	n := w.dropped
	w.dropped = 0
	return func() {
		if l.onDropped != nil {
			l.onDropped(path, n)
		}
	}
}

// sweep forgets the windows whose minute is over, taking their drops. Called with the lock held.
func (l *limiter) sweep(t time.Time) []func() {
	var told []func()
	for path, w := range l.windows {
		if t.Sub(w.start) < time.Minute {
			continue
		}
		if w.dropped > 0 {
			told = append(told, l.tell(path, w))
		}
		delete(l.windows, path)
	}
	return told
}

// flush tells w's drops at the end of its minute, and forgets it, unless a later report did either.
func (l *limiter) flush(path string, w *window) {
	l.mu.Lock()
	var tell func()
	if w.dropped > 0 {
		tell = l.tell(path, w)
	}
	if l.windows[path] == w {
		delete(l.windows, path)
	}
	l.mu.Unlock()
	if tell != nil {
		tell()
	}
}

// clip cuts s to limit runes, saying so.
func clip(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "… (truncated)"
}

// logDropped says how many of a Data App's reports in a minute were over the limit, and dropped.
func (s *Server) logDropped(path string, dropped int) {
	if s.opts.AppLogger == nil {
		return
	}
	s.opts.AppLogger.LogAttrs(context.Background(), slog.LevelWarn, "Data App reports dropped",
		slog.Any(logkit.EventKey, eventAppReportsDropped), slog.String("app", path), slog.Int("dropped", dropped),
		slog.String("hint", "the Data App reported more than the log takes in a minute"))
}

// log writes a Data App's report to anfra.log, as "service":"app".
func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	var rep Report
	if err := jsonkit.UnmarshalRead(http.MaxBytesReader(w, r.Body, reportBodyLimit), &rep); err != nil {
		httpkit.WriteError(w, r, apperr.Encapsulate(err, apperr.InvalidRequest, "Not a Data App's report."))
		return
	}
	if rep.Path == "" || len(rep.Path) > 512 || (rep.Type != "error" && rep.Type != "console" && rep.Type != "mounted") {
		httpkit.WriteError(w, r, apperr.New(apperr.InvalidRequest, "A report names its Data App's path, and is an error, a console line or a mount."))
		return
	}
	// Taken: what it says is a line of its own (or counted for one), so the request needs none.
	httpkit.Unlogged(r.Context())
	logger := s.opts.AppLogger
	if logger == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.reports.allow(rep.Path) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if rep.Type == "mounted" {
		s.logMounted(r.Context(), logger, rep.Path)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	e := rep.Event
	level, event := slog.LevelError, eventAppError
	if rep.Type == "console" {
		event = eventAppConsole
	}
	attrs := []slog.Attr{slog.Any(logkit.EventKey, event), slog.String("app", rep.Path)}
	add := func(key, value string, limit int) {
		if value != "" {
			attrs = append(attrs, slog.String(key, clip(value, limit)))
		}
	}
	if rep.Type == "console" {
		if e.Level == "warn" {
			level = slog.LevelWarn
		}
	} else {
		add("kind", e.Kind, shortLimit)
		add("code", e.Code, shortLimit)
		add("scope", e.Scope, shortLimit)
		add("request_id", e.RequestID, shortLimit)
		add("entity", e.Entity, shortLimit)
		add("call", e.Call, shortLimit)
	}
	add("stack", e.Stack, stackLimit)
	logger.LogAttrs(r.Context(), level, clip(e.Message, messageLimit), attrs...)
	w.WriteHeader(http.StatusNoContent)
}

// logMounted says the page loaded the Data App at path, with the file's modification time, read
// here rather than taken from the page: the lines after it are the file as it was then. A path
// that names no Data App's file is not logged.
func (s *Server) logMounted(ctx context.Context, logger *slog.Logger, path string) {
	file := appFile(s.opts.RepoDir, path)
	if file == "" {
		return
	}
	info, err := os.Stat(file) //nolint:gosec // G703: appFile confines it to apps/
	if err != nil {
		return
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "Data App mounted", slog.Any(logkit.EventKey, eventAppMounted), slog.String("app", path),
		slog.String("modified", info.ModTime().UTC().Format(time.RFC3339Nano)))
}
