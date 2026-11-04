package logkit

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// define defines an event for one test, and forgets it after, so tests can
// reuse names.
func define(t *testing.T, name, doc string, attrs ...string) Event {
	t.Helper()
	before := len(defined)
	e := DefineEvent(name, doc, attrs...)
	t.Cleanup(func() {
		defined = defined[:before]
		delete(names, name)
	})
	return e
}

func panics(fn func()) (msg string) {
	defer func() {
		if p := recover(); p != nil {
			msg, _ = p.(string)
		}
	}()
	fn()
	return ""
}

func TestDefineEvent(t *testing.T) {
	e := define(t, "test.thing_happened", "A thing happened.", "thing", "count")
	if e.Name() != "test.thing_happened" || e.Doc() != "A thing happened." {
		t.Errorf("got %s %q", e.Name(), e.Doc())
	}
	if got := e.Attrs(); len(got) != 2 || got[0] != "thing" || got[1] != "count" {
		t.Errorf("Attrs() = %v", got)
	}
	all := Events()
	if len(all) == 0 || all[len(all)-1].Name() != e.Name() {
		t.Errorf("Events() does not end with the event just defined: %v", all)
	}
}

func TestDefineEventRefuses(t *testing.T) {
	define(t, "test.taken", "Taken.")
	for _, c := range []struct {
		name, doc string
		attrs     []string
		want      string
	}{
		{"test.taken", "Again.", nil, "duplicate"},
		{"nodot", "No area.", nil, "not <area>"},
		{"Test.upper", "Upper case.", nil, "not <area>"},
		{"test.has-dash", "Kebab case.", nil, "not <area>"},
		{"test.no_doc", "", nil, "no doc"},
		{"test.camel_attr", "Camel attribute.", []string{"childPid"}, "not a snake_case"},
		{"test.event_attr", "Its own key.", []string{"event"}, "not a snake_case"},
		{"test.twice", "Attribute twice.", []string{"a", "a"}, "twice"},
	} {
		msg := panics(func() { DefineEvent(c.name, c.doc, c.attrs...) })
		if !strings.Contains(msg, c.want) {
			t.Errorf("DefineEvent(%q, %q, %v): panic %q, want one saying %q", c.name, c.doc, c.attrs, msg, c.want)
		}
	}
}

func TestEventInARecord(t *testing.T) {
	e := define(t, "test.logged", "Logged.")
	var b bytes.Buffer
	slog.New(slog.NewJSONHandler(&b, nil)).Info("Logged", EventKey, e)
	if !strings.Contains(b.String(), `"event":"test.logged"`) {
		t.Errorf("record = %s", b.String())
	}
}

func TestTraceHandler(t *testing.T) {
	var b bytes.Buffer
	logger := slog.New(TraceHandler(slog.NewJSONHandler(&b, nil))).With("service", "test")

	logger.Info("Outside a span")
	if strings.Contains(b.String(), "trace_id") {
		t.Errorf("a record outside a span has trace context: %s", b.String())
	}

	b.Reset()
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36},
		SpanID:  trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7},
	})
	logger.InfoContext(trace.ContextWithSpanContext(context.Background(), sc), "In a span")
	for _, want := range []string{`"service":"test"`, `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"`, `"span_id":"00f067aa0ba902b7"`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("record = %s, want %s in it", b.String(), want)
		}
	}
}
