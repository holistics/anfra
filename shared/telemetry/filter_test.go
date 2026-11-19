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

// levels is settings with the levels given, the rest at their defaults.
func levels(l map[telemetry.Category]telemetry.Level) telemetry.Settings {
	return telemetry.NewSettings(l, 0)
}

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

// By default, inputs and outputs are left out, SQL and errors masked, a
// left-out key never recorded, anfra's own keys kept, and an undeclared key or
// event left out, its name recorded.
func TestFilterAppliesTheDefaults(t *testing.T) {
	s := filtered(t, context.Background(), telemetry.Defaults())
	got := attrNames(s.Attributes())
	if got["anfra.op"] != "core.query" {
		t.Errorf("anfra's own key was dropped: %v", got)
	}
	if _, ok := got["some.new.key"]; ok {
		t.Error("an undeclared key was recorded")
	}
	if d := got["anfra.telemetry.dropped_keys"]; d != `["some.new.event","some.new.key"]` {
		t.Errorf("anfra.telemetry.dropped_keys = %s, want the undeclared key and event", d)
	}
	if _, ok := got["anfra.dataset"]; ok {
		t.Error("anfra.dataset recorded with inputs off")
	}
	if _, ok := got["client.address"]; ok {
		t.Error("client.address recorded")
	}
	if _, ok := got["server.address"]; ok {
		t.Error("server.address, which can name the org, recorded with inputs off")
	}
	if got["error.message"] != "invalid date '?'" {
		t.Errorf("error.message = %q, want it masked as exception.message is", got["error.message"])
	}
	if got["db.query.text"] != "SELECT * FROM t WHERE email = ?" {
		t.Errorf("db.query.text = %q, want it masked", got["db.query.text"])
	}
	if n := eventNames(s); !slices.Equal(n, []string{"exception"}) {
		t.Errorf("events = %v, want the exception alone", n)
	}
	exc := attrNames(s.Events()[0].Attributes)
	if exc["exception.message"] != "invalid date '?'" {
		t.Errorf("exception.message = %q, want it masked", exc["exception.message"])
	}
	if _, ok := exc["exception.stacktrace"]; ok {
		t.Error("a masked error kept its stack")
	}
	if d := s.Status().Description; d != "connect to ? failed" {
		t.Errorf("status = %q, want it masked", d)
	}
}

// Raw records each category as it is; a left-out key still isn't.
func TestFilterRecordsRawAsItIs(t *testing.T) {
	all := levels(map[telemetry.Category]telemetry.Level{
		telemetry.Inputs: telemetry.Raw, telemetry.Outputs: telemetry.Raw, telemetry.SQL: telemetry.Raw, telemetry.Errors: telemetry.Raw,
	})
	s := filtered(t, context.Background(), all)
	got := attrNames(s.Attributes())
	if got["anfra.dataset"] != "ecommerce" || got["db.query.text"] != "SELECT * FROM t WHERE email = 'a@b.c'" {
		t.Errorf("raw: %v", got)
	}
	if _, ok := got["some.new.key"]; ok {
		t.Error("an undeclared key was recorded with everything raw")
	}
	if _, ok := got["client.address"]; ok {
		t.Error("client.address recorded with everything raw")
	}
	if n := eventNames(s); !slices.Equal(n, []string{"anfra.op.input", "anfra.op.output", "exception"}) {
		t.Errorf("events = %v", n)
	}
	if exc := attrNames(s.Events()[2].Attributes); exc["exception.stacktrace"] == "" {
		t.Error("a raw error lost its stack")
	}
}

// Off leaves the category out, an error's whole exception event included, but
// keeps that it failed; anfra's own status text, a public code, is always kept.
func TestFilterLeavesOffOut(t *testing.T) {
	off := levels(map[telemetry.Category]telemetry.Level{telemetry.SQL: telemetry.Off, telemetry.Errors: telemetry.Off})
	s := filtered(t, context.Background(), off)
	if _, ok := attrNames(s.Attributes())["db.query.text"]; ok {
		t.Error("db.query.text recorded with SQL off")
	}
	if n := eventNames(s); slices.Contains(n, "exception") {
		t.Errorf("errors off: events = %v, want no exception", n)
	}
	if s.Status().Description != "" {
		t.Errorf("status = %q, want none", s.Status().Description)
	}

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.Filter(rec, off)))
	_, span := tp.Tracer("test").Start(context.Background(), "op")
	span.SetStatus(codes.Error, "internal_server_error")
	span.End()
	if d := rec.Ended()[0].Status().Description; d != "internal_server_error" {
		t.Errorf("a public code's status = %q", d)
	}
}

// A status text that only looks like a code, a table's name say, is a
// message: masked as errors say, not kept as a code is.
func TestAStatusThatIsNoCodeIsAMessage(t *testing.T) {
	for status, want := range map[string]string{
		"analytics.customers":   "",
		"internal_server_error": "internal_server_error",
		telemetry.PanicStatus:   telemetry.PanicStatus,
	} {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.Filter(rec, levels(map[telemetry.Category]telemetry.Level{telemetry.Errors: telemetry.Off}))))
		_, span := tp.Tracer("test").Start(context.Background(), "op")
		span.SetStatus(codes.Error, status)
		span.End()
		if d := rec.Ended()[0].Status().Description; d != want {
			t.Errorf("status %q, errors off: recorded %q, want %q", status, d, want)
		}
	}
}

// A span follows the settings of the context it started in, over the
// fallback.
func TestFilterFollowsTheContextsSettings(t *testing.T) {
	ctx := telemetry.WithSettings(context.Background(), levels(map[telemetry.Category]telemetry.Level{telemetry.Inputs: telemetry.Raw}))
	if got := attrNames(filtered(t, ctx, telemetry.Defaults()).Attributes()); got["anfra.dataset"] != "ecommerce" {
		t.Errorf("with the context's inputs raw, anfra.dataset = %q", got["anfra.dataset"])
	}
}

// RecordError records an error as errors says, before any processor: off
// records no event.
func TestRecordErrorFollowsErrors(t *testing.T) {
	for level, want := range map[telemetry.Level]string{telemetry.Raw: "bad '1'", telemetry.Masked: "bad '?'", telemetry.Off: ""} {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		_, span := tp.Tracer("test").Start(context.Background(), "op")
		telemetry.RecordError(span, levels(map[telemetry.Category]telemetry.Level{telemetry.Errors: level}), errors.New("bad '1'"), true)
		span.End()
		events := rec.Ended()[0].Events()
		if level == telemetry.Off {
			if len(events) != 0 {
				t.Errorf("off: events = %v, want none", events)
			}
			continue
		}
		exc := attrNames(events[0].Attributes)
		if exc["exception.message"] != want {
			t.Errorf("%s: exception.message = %q, want %q", level, exc["exception.message"], want)
		}
		if _, ok := exc["exception.stacktrace"]; ok != (level == telemetry.Raw) {
			t.Errorf("%s: stack recorded = %v", level, ok)
		}
	}
}
