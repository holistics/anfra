package appserve

import (
	"log/slog"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
	"github.com/holistics/anfra/shared/jsonkit"
)

// Report is what the appserve page posts to /appserve/log: one runtime error, or one console line,
// of the Data App at Path, as the SDK's host reported it (anfra.error, anfra.console). It reaches
// anfra.log, so an agent reads a Data App's failures without a browser.
type Report struct {
	// Path is the Data App's, under apps/.
	Path string `json:"path"`
	// Type is "error" or "console".
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
	At        string `json:"at,omitempty"`
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

// limiter counts each Data App's reports in the current minute.
type limiter struct {
	mu      sync.Mutex
	now     func() time.Time
	windows map[string]*window
}

type window struct {
	start   time.Time
	count   int
	dropped int
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, windows: map[string]*window{}}
}

// allow says whether path may log one more report now, and how many of its reports were dropped
// in the minute that just ended, to say so once.
func (l *limiter) allow(path string) (ok bool, dropped int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	w := l.windows[path]
	if w == nil || t.Sub(w.start) >= time.Minute {
		if w != nil {
			dropped = w.dropped
		}
		w = &window{start: t}
		l.windows[path] = w
	}
	if w.count >= reportsPerMinute {
		w.dropped++
		return false, dropped
	}
	w.count++
	return true, dropped
}

// clip cuts s to limit runes, saying so.
func clip(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + "… (truncated)"
}

// log writes a Data App's report to anfra.log, as "service":"app".
func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	var rep Report
	if err := jsonkit.UnmarshalRead(http.MaxBytesReader(w, r.Body, reportBodyLimit), &rep); err != nil {
		httpkit.WriteError(w, r, apperr.Encapsulate(err, apperr.InvalidRequest, "Not a Data App's report."))
		return
	}
	if rep.Path == "" || len(rep.Path) > 512 || (rep.Type != "error" && rep.Type != "console") {
		httpkit.WriteError(w, r, apperr.New(apperr.InvalidRequest, "A report names its Data App's path, and is an error or a console line."))
		return
	}
	logger := s.opts.AppLogger
	if logger == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ok, dropped := s.reports.allow(rep.Path)
	if dropped > 0 {
		logger.LogAttrs(r.Context(), slog.LevelWarn, "app.reports_dropped", slog.String("app", rep.Path), slog.Int("dropped", dropped),
			slog.String("hint", "the Data App reported more than the log takes in a minute"))
	}
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	e := rep.Event
	attrs := []slog.Attr{slog.String("app", rep.Path)}
	add := func(key, value string, limit int) {
		if value != "" {
			attrs = append(attrs, slog.String(key, clip(value, limit)))
		}
	}
	level, msg := slog.LevelError, "app.error"
	if rep.Type == "console" {
		msg = "app.console"
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
	add("message", e.Message, messageLimit)
	add("stack", e.Stack, stackLimit)
	add("at", e.At, shortLimit)
	logger.LogAttrs(r.Context(), level, msg, attrs...)
	w.WriteHeader(http.StatusNoContent)
}
