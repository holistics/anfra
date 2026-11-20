package telemetry_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/shared/telemetry"
)

func provider(t *testing.T) (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.Spans{}), sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp, rec
}

func attrsOf(s sdktrace.ReadOnlySpan) map[string]string {
	m := map[string]string{}
	for _, kv := range s.Attributes() {
		m[string(kv.Key)] = kv.Value.String()
	}
	return m
}

// Every span started in the invocation's context gets the caller's fields,
// however deep, and the span current when they were given (the root) too.
func TestEverySpanInTheContextNamesTheCaller(t *testing.T) {
	tp, rec := provider(t)
	tr := tp.Tracer("test")

	ctx, root := tr.Start(context.Background(), "root")
	ctx = telemetry.WithAttributes(ctx, map[string]string{"gen_ai.conversation.id": "s-1", "gen_ai.agent.name": "claude-code", "empty": ""})
	ctx, op := tr.Start(ctx, "op")
	_, inner := tr.Start(ctx, "inner")
	inner.End()
	op.End()
	root.End()

	spans := rec.Ended()
	if len(spans) != 3 {
		t.Fatalf("got %d spans", len(spans))
	}
	for _, s := range spans {
		got := attrsOf(s)
		for k, want := range map[string]string{"gen_ai.conversation.id": "s-1", "gen_ai.agent.name": "claude-code"} {
			if got[k] != want {
				t.Errorf("%s: %s = %q, want %q", s.Name(), k, got[k], want)
			}
		}
		if _, ok := got["empty"]; ok {
			t.Errorf("%s: an empty field was recorded", s.Name())
		}
	}
}

// Fields given later add to the earlier ones, a key given again taking the new
// value; a span started before any is given gets none.
func TestFieldsAddUp(t *testing.T) {
	tp, rec := provider(t)
	tr := tp.Tracer("test")

	_, before := tr.Start(context.Background(), "before")
	before.End()
	ctx := telemetry.WithAttributes(context.Background(), map[string]string{"tenant.id": "acme", "user.id": "u-1"})
	ctx = telemetry.WithAttributes(ctx, map[string]string{"user.id": "u-2"})
	_, after := tr.Start(ctx, "after")
	after.End()

	spans := rec.Ended()
	if got := attrsOf(spans[0]); len(got) != 0 {
		t.Errorf("before: got %v", got)
	}
	if got := attrsOf(spans[1]); got["tenant.id"] != "acme" || got["user.id"] != "u-2" {
		t.Errorf("after: got %v", got)
	}
}

// Nothing given, nothing added: the CLI and serve today.
func TestNoFieldsNoAttributes(t *testing.T) {
	tp, rec := provider(t)
	ctx := telemetry.WithAttributes(context.Background(), nil)
	_, s := tp.Tracer("test").Start(ctx, "op")
	s.End()
	if got := rec.Ended()[0].Attributes(); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

// A log record gets the fields snake_case.
func TestLogRecordsNameTheCallerSnakeCase(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(telemetry.LogHandler(slog.NewJSONHandler(&buf, nil)))
	ctx := telemetry.WithAttributes(context.Background(), map[string]string{"gen_ai.conversation.id": "s-1", "gen_ai.agent.name": "codex"})

	log.InfoContext(ctx, "Ran")
	log.With("k", "v").InfoContext(context.Background(), "Alone")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if !strings.Contains(lines[0], `"gen_ai_agent_name":"codex"`) || !strings.Contains(lines[0], `"gen_ai_conversation_id":"s-1"`) {
		t.Errorf("got %s", lines[0])
	}
	if strings.Contains(lines[0], "gen_ai.") {
		t.Errorf("got %s", lines[0])
	}
	if strings.Contains(lines[1], "gen_ai_conversation_id") {
		t.Errorf("a record outside the context got the fields: %s", lines[1])
	}
}
