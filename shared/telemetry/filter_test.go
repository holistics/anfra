package telemetry_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/shared/telemetry"
)

// on is settings with data on.
var on = telemetry.NewSettings(true, 0)

// filtered is the one span a span with every kind of key ends as, through
// Filter with fallback, started in ctx.
func filtered(t *testing.T, ctx context.Context, fallback telemetry.Settings) sdktrace.ReadOnlySpan {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.Filter(rec, fallback)))
	t.Cleanup(func() { _ = tp.Shutdown(context.WithoutCancel(ctx)) })
	_, span := tp.Tracer("test").Start(ctx, "op")
	span.SetAttributes(
		attribute.String("anfra.op", "core.query"),
		attribute.String("db.system.name", "postgresql"),
		attribute.String("anfra.dataset", "ecommerce"),
		attribute.String("db.query.text", "SELECT * FROM t WHERE email = 'a@b.c'"),
		attribute.String("client.address", "203.0.113.9"),
		attribute.String("server.address", "acme.holistics.io"),
		attribute.String("error.message", "invalid date '2024-13-01'"),
		attribute.String("some.new.key", "a@b.c"),
	)
	span.AddEvent("anfra.op.input", trace.WithAttributes(attribute.String("anfra.payload", `{"q":1}`)))
	span.AddEvent("anfra.op.output", trace.WithAttributes(attribute.String("anfra.payload", `{"a":1}`)))
	span.AddEvent("some.new.event")
	span.RecordError(errors.New("invalid date '2024-13-01'"), trace.WithStackTrace(true))
	span.SetStatus(codes.Error, "connect to 10.0.0.1 failed")
	span.End()
	return rec.Ended()[0]
}

func attrNames(kvs []attribute.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.String()
	}
	return m
}

func eventNames(s sdktrace.ReadOnlySpan) []string {
	var n []string
	for _, e := range s.Events() {
		n = append(n, e.Name)
	}
	return n
}

// By default the user's data is left out, a left-out key never recorded,
// anfra's own keys kept, and an undeclared key or event left out, its name
// recorded. That the span failed is kept, but not its message.
func TestFilterAppliesTheDefaults(t *testing.T) {
	s := filtered(t, context.Background(), telemetry.Defaults())
	got := attrNames(s.Attributes())
	if got["anfra.op"] != "core.query" {
		t.Errorf("anfra's own key was dropped: %v", got)
	}
	if d := got["anfra.telemetry.dropped_keys"]; d != `["some.new.event","some.new.key"]` {
		t.Errorf("anfra.telemetry.dropped_keys = %s, want the undeclared key and event", d)
	}
	for _, k := range []string{"some.new.key", "anfra.dataset", "client.address", "server.address", "error.message", "db.query.text"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s recorded by default", k)
		}
	}
	if n := eventNames(s); len(n) != 0 {
		t.Errorf("events = %v, want none", n)
	}
	if s.Status().Code != codes.Error || s.Status().Description != "" {
		t.Errorf("status = %+v, want an error without its message", s.Status())
	}
}

// Data on records each data key as it is; a left-out key still isn't.
func TestFilterRecordsDataWhenOn(t *testing.T) {
	s := filtered(t, context.Background(), on)
	got := attrNames(s.Attributes())
	if got["anfra.dataset"] != "ecommerce" || got["db.query.text"] != "SELECT * FROM t WHERE email = 'a@b.c'" || got["error.message"] != "invalid date '2024-13-01'" {
		t.Errorf("data on: %v", got)
	}
	if _, ok := got["some.new.key"]; ok {
		t.Error("an undeclared key was recorded with data on")
	}
	if _, ok := got["client.address"]; ok {
		t.Error("client.address recorded with data on")
	}
	if n := eventNames(s); !slices.Equal(n, []string{"anfra.op.input", "anfra.op.output", "exception"}) {
		t.Errorf("events = %v", n)
	}
	if exc := attrNames(s.Events()[2].Attributes); exc["exception.stacktrace"] == "" {
		t.Error("an error lost its stack")
	}
	if d := s.Status().Description; d != "connect to 10.0.0.1 failed" {
		t.Errorf("status = %q, want the message", d)
	}
}

// anfra's own status text, a public code, is kept with data off.
func TestAPublicCodesStatusIsKept(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.Filter(rec, telemetry.Defaults())))
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	span.SetStatus(codes.Error, "internal_server_error")
	span.End()
	if d := rec.Ended()[0].Status().Description; d != "internal_server_error" {
		t.Errorf("a public code's status = %q", d)
	}
}

// A status text that only looks like a code, a table's name say, is a
// message: left out with data off, not kept as a code is.
func TestAStatusThatIsNoCodeIsAMessage(t *testing.T) {
	for status, want := range map[string]string{
		"analytics.customers":   "",
		"internal_server_error": "internal_server_error",
		telemetry.PanicStatus:   telemetry.PanicStatus,
	} {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.Filter(rec, telemetry.Defaults())))
		_, span := tp.Tracer("test").Start(context.Background(), "op")
		span.SetStatus(codes.Error, status)
		span.End()
		if d := rec.Ended()[0].Status().Description; d != want {
			t.Errorf("status %q, data off: recorded %q, want %q", status, d, want)
		}
	}
}

// A span follows the settings of the context it started in, over the
// fallback.
func TestFilterFollowsTheContextsSettings(t *testing.T) {
	ctx := telemetry.WithSettings(context.Background(), on)
	if got := attrNames(filtered(t, ctx, telemetry.Defaults()).Attributes()); got["anfra.dataset"] != "ecommerce" {
		t.Errorf("with the context's data on, anfra.dataset = %q", got["anfra.dataset"])
	}
}

// RecordError records an error, with its stack, only when data is on.
func TestRecordErrorFollowsErrors(t *testing.T) {
	for _, dataOn := range []bool{true, false} {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		_, span := tp.Tracer("test").Start(context.Background(), "op")
		s := telemetry.Defaults()
		if dataOn {
			s = on
		}
		telemetry.RecordError(span, s, errors.New("bad '1'"), true)
		span.End()
		events := rec.Ended()[0].Events()
		if !dataOn {
			if len(events) != 0 {
				t.Errorf("off: events = %v, want none", events)
			}
			continue
		}
		exc := attrNames(events[0].Attributes)
		if exc["exception.message"] != "bad '1'" || exc["exception.stacktrace"] == "" {
			t.Errorf("on: exception = %v, want its message and stack", exc)
		}
	}
}
