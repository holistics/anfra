// Package telemetry names an invocation's caller on what it records: every span
// started in its context, and every log record logged with it.
//
// A host knows its caller (a session, a tenant, a user, a request id) and puts
// it in the context with WithAttributes, before the work starts. Spans pick it
// up through the span processor (Spans), which the host installs on its tracer
// provider, and log records through LogHandler. With nothing put there, they
// add nothing.
//
// A key is the span attribute's name, dotted as OpenTelemetry's conventions are
// (session.id, agent.name). Log keys are snake_case (docs/designs/logs.md), so
// on a log record each dot becomes an underscore: session.id is logged as
// session_id. session.id is also written to spans as gen_ai.conversation.id,
// the GenAI conventions' name, which some agent tracing platforms read instead.
//
// For observation only: nothing that decides what a command does reads it.
//
// Ported from anfra-cloud's shared/telemetry (identity.go), which names its
// caller the same way with fixed fields.
package telemetry

import (
	"context"
	"maps"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// aliases are the other span names a key is also written as.
var aliases = map[string][]string{
	"session.id": {"gen_ai.conversation.id"},
}

type attributesKey struct{}

// WithAttributes is ctx with fields naming its caller, added to any it names
// already (a key given again takes the new value), for every span started in it
// and every record logged with it. The span current in ctx, started before the
// caller was known, is named too: for the CLI, the command's root span. Empty
// values are left out.
func WithAttributes(ctx context.Context, fields map[string]string) context.Context {
	added := map[string]string{}
	for k, v := range fields {
		if k != "" && v != "" {
			added[k] = v
		}
	}
	if len(added) == 0 {
		return ctx
	}
	merged := maps.Clone(attributesOf(ctx))
	if merged == nil {
		merged = map[string]string{}
	}
	maps.Copy(merged, added)
	trace.SpanFromContext(ctx).SetAttributes(spanAttrs(added)...)
	return context.WithValue(ctx, attributesKey{}, merged)
}

// attributesOf is what WithAttributes put in ctx; nil for nothing.
func attributesOf(ctx context.Context) map[string]string {
	m, _ := ctx.Value(attributesKey{}).(map[string]string)
	return m
}

// spanAttrs are fields as span attributes, aliases included, in key order.
func spanAttrs(fields map[string]string) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		attrs = append(attrs, attribute.String(k, fields[k]))
		for _, a := range aliases[k] {
			attrs = append(attrs, attribute.String(a, fields[k]))
		}
	}
	return attrs
}

// LogKey is a field's key on a log record: its span name, snake_case.
func LogKey(k string) string { return strings.ReplaceAll(k, ".", "_") }

// Spans is the span processor that names the caller on every span started in a
// context WithAttributes filled, whoever starts it: anfra's own spans,
// otelhttp's and a library's. Install it on the tracer provider before its
// exporter's batcher (sdktrace.WithSpanProcessor).
type Spans struct{}

var _ sdktrace.SpanProcessor = Spans{}

// OnStart names the caller on s.
func (Spans) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	if fields := attributesOf(parent); len(fields) > 0 {
		s.SetAttributes(spanAttrs(fields)...)
	}
}

// OnEnd does nothing: a span's attributes are set when it starts.
func (Spans) OnEnd(sdktrace.ReadOnlySpan) {}

// Shutdown does nothing: Spans holds nothing.
func (Spans) Shutdown(context.Context) error { return nil }

// ForceFlush does nothing: Spans holds nothing.
func (Spans) ForceFlush(context.Context) error { return nil }
