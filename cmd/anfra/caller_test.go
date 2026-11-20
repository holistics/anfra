package main

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/shared/apperr"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// commandSession records the spans of the commands a test runs, in an agent's
// session (session, or none when empty), with anfra's home its own.
func commandSession(t *testing.T, session string) *tracetest.SpanRecorder {
	t.Helper()
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Setenv("ANFRA_NO_UPDATE_NOTIFIER", "1")
	for _, v := range []string{"CLAUDE_CODE_SESSION_ID", "CODEX_SESSION_ID", "OPENCODE_SESSION_ID", "AGENT_SESSION_ID", "ANFRA_SESSION_ID", "TRACEPARENT", "TRACESTATE"} {
		t.Setenv(v, "")
	}
	t.Setenv("CLAUDE_CODE_SESSION_ID", session)
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(caller.Spans{}),
		sdktrace.WithSpanProcessor(caller.Filter(rec, traceData.Settings())),
	))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return rec
}

// anfraRun runs anfra with args, and returns the spans it ended, what it
// wrote, and its exit code.
func anfraRun(t *testing.T, rec *tracetest.SpanRecorder, args ...string) (spans []sdktrace.ReadOnlySpan, stdout, stderr string, code int) {
	t.Helper()
	t.Cleanup(resetOutput)
	before := len(rec.Ended())
	var out, errOut bytes.Buffer
	code = run(t.Context(), args, &out, &errOut, runOptions{})
	resetOutput()
	return rec.Ended()[before:], out.String(), errOut.String(), code
}

// attrsOf is a span's attributes, as strings.
func attrsOf(s sdktrace.ReadOnlySpan) map[string]string {
	m := map[string]string{}
	for _, a := range s.Attributes() {
		m[string(a.Key)] = a.Value.String()
	}
	return m
}

// named is the span of spans named name; it fails the test without one.
func named(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range spans {
		if s.Name() == name {
			return s
		}
	}
	var names []string
	for _, s := range spans {
		names = append(names, s.Name())
	}
	t.Fatalf("no span %q in %v", name, names)
	return nil
}

// When the agent passes its trace, as TRACEPARENT, a command's span goes under
// the agent's span, marked as the command's, with its id and its exit code.
func TestACommandFollowsItsCallersTrace(t *testing.T) {
	rec := commandSession(t, "s-1")
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	const agentTrace, agentSpan = "920f174f97fbdcc18b7dd6f7d10bd48b", "79cbcb6bcdf1bccf"
	t.Setenv("TRACEPARENT", "00-"+agentTrace+"-"+agentSpan+"-01")
	spans, _, _, _ := anfraRun(t, rec, "version", "--yaml")
	cmd := named(t, spans, "anfra version")
	if cmd.Parent().TraceID().String() != agentTrace || cmd.Parent().SpanID().String() != agentSpan {
		t.Errorf("command's parent %v, want the agent's span", cmd.Parent())
	}
	if a := attrsOf(cmd); a["anfra.command"] != "true" || a["anfra.command.id"] != cmd.SpanContext().SpanID().String() || a["process.exit.code"] != "0" {
		t.Errorf("command's attributes: %v", a)
	}
}

// The caller's baggage in the environment (BAGGAGE) names a command's spans
// with the members the settings include, and the command leaves the
// environment as it found it.
func TestACommandRecordsItsCallersBaggage(t *testing.T) {
	withTraceData(t, map[string]string{caller.BaggageIncludeVar: "tenant.id"})
	rec := commandSession(t, "")
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })
	t.Setenv("BAGGAGE", "tenant.id=acme,user.id=u-1")

	spans, _, _, _ := anfraRun(t, rec, "version", "--yaml")
	a := attrsOf(named(t, spans, "anfra version"))
	if a["tenant.id"] != "acme" || a["user.id"] != "" {
		t.Errorf("command's attributes: %v", a)
	}
	if got := os.Getenv("BAGGAGE"); got != "tenant.id=acme,user.id=u-1" {
		t.Errorf("BAGGAGE after the command: %q", got)
	}
	if got := os.Getenv("TRACEPARENT"); got != "" {
		t.Errorf("TRACEPARENT left after the command: %q", got)
	}
}

// A run exits 0 when nothing failed, with the code a rendered result asks for,
// 130 when interrupted, and 1 otherwise.
func TestExitCode(t *testing.T) {
	for err, want := range map[error]int{
		nil:                     0,
		&exitCodeError{code: 2}: 2,
		apperr.New(clierr.Interrupted, "Interrupted."): 130,
		errors.New("boom"): 1,
	} {
		if got := exitCode(err); got != want {
			t.Errorf("exitCode(%v) = %d, want %d", err, got, want)
		}
	}
}
