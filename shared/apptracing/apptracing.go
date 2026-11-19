// Package apptracing runs the steps appstep defines.
//
// Start begins a step: a span in the trace, named for the step, its parameters
// as attributes with the sensitive ones redacted. End finishes it, and if the
// function is failing, adds the step to its error through apperr.WithStep,
// leaving the error's meaning as it was.
//
// A span is marked as errored only for a server-scope failure: a user's invalid
// input or a client's missing session is the system working, and an error status
// on it would bury the failures that need someone. Every failing span carries
// error.type, the code, so the others are still findable.
//
// OpenTelemetry stays in this package: neither appstep nor apperr knows of
// tracing. Spans go to the global tracer provider, which the host
// installs; without it they are no-ops, and the error half works the same.
package apptracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
	"github.com/holistics/anfra/shared/telemetry"
)

// tracer follows the global provider, including one installed after this
// package is initialised.
var tracer = otel.Tracer("github.com/holistics/anfra/shared/apptracing")

// Span is one running step.
type Span struct {
	step     appstep.Step
	span     trace.Span
	settings telemetry.Settings // the data settings of the context it started in
	ended    bool
}

// Start starts a step of def. Its End must be deferred with a pointer to the
// enclosing function's named error result:
//
//	func (s *Imports) importOne(ctx context.Context, f File) (err error) {
//		ctx, span := apptracing.Start(ctx, steps.ImportDashboard, "name", f.Name)
//		defer span.End(&err)
//		...
//	}
//
// kv are key/value pairs, as in slog. Which are sensitive is def's declaration.
func Start(ctx context.Context, def appstep.Def, kv ...any) (context.Context, *Span) {
	step := appstep.NewStep(def, kv...)
	var attrs []attribute.KeyValue
	for k, v := range step.Logged() {
		attrs = append(attrs, attr("h.step."+k, v))
	}
	// Ended by the caller, through Span.End: spancheck checks each caller instead
	// (extra-start-span-signatures in .golangci.yml).
	ctx, span := tracer.Start(ctx, step.Name(), trace.WithAttributes(attrs...)) //nolint:spancheck // see above
	return ctx, &Span{step: step, span: span, settings: telemetry.SettingsOf(ctx)}
}

// End ends the step. If the function is failing, the step is added to its
// error, leaving the error's meaning as it was. A second call does nothing.
//
// Deferred, it also sees a panic passing through: the span records it and is
// marked as errored, and the panic goes on unchanged to whatever recovers it.
func (s *Span) End(errp *error) {
	if s.ended {
		return
	}
	s.ended = true
	// recover works only when called directly by the deferred function, which
	// End is in the supported form; keep it here, not in a helper.
	if p := recover(); p != nil {
		telemetry.RecordError(s.span, s.settings, fmt.Errorf("panic: %v", p), true)
		s.span.SetStatus(codes.Error, telemetry.PanicStatus)
		s.span.End()
		panic(p)
	}
	defer s.span.End()
	if errp == nil || *errp == nil {
		return
	}
	*errp = apperr.WithStep(*errp, s.step)
	// err.Error() is the log form: sensitive parameters already redacted.
	MarkFailed(s.span, s.settings, *errp)
}

// MarkFailed marks span as failing with err: error.type, its code, always; and
// an error status and the error, its message recorded as settings' errors say
// (docs/designs/telemetry.md), only when the failure is anfra's, not the
// caller's.
func MarkFailed(span trace.Span, settings telemetry.Settings, err error) {
	code := apperr.From(err).Code
	span.SetAttributes(attribute.String(telemetry.ErrorType.String(), code.String()))
	if code.Public().Scope() == apperr.Server {
		telemetry.RecordError(span, settings, err, false)
		span.SetStatus(codes.Error, code.Public().String())
	}
}

// AddEvent adds the event name, at, to the span current in ctx: a step of
// what it did, such as a query's, which holds no data.
func AddEvent(ctx context.Context, name string, at time.Time) {
	trace.SpanFromContext(ctx).AddEvent(name, trace.WithTimestamp(at))
}

// attr is v as an attribute, keeping the types OpenTelemetry has and rendering
// any other the way the log line does.
func attr(k string, v any) attribute.KeyValue {
	switch v := v.(type) {
	case string:
		return attribute.String(k, v)
	case bool:
		return attribute.Bool(k, v)
	case int:
		return attribute.Int(k, v)
	case int64:
		return attribute.Int64(k, v)
	case float64:
		return attribute.Float64(k, v)
	default:
		return attribute.String(k, fmt.Sprint(v))
	}
}

// SetAttributes adds kv to the span current in ctx: what an op knows only as
// it runs, such as the rows a query read. Each key is declared in
// shared/telemetry, as data or metadata, and data is set only when data is
// recorded (docs/designs/telemetry.md). A pair whose value is nil, or a nil
// *int64, is left out, so an optional value needs no check.
func SetAttributes(ctx context.Context, kv ...telemetry.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	settings := telemetry.SettingsOf(ctx)
	var attrs []attribute.KeyValue
	for _, p := range kv {
		v := p.Value
		if ptr, isPtr := v.(*int64); isPtr {
			if ptr == nil {
				continue
			}
			v = *ptr
		}
		if v == nil {
			continue
		}
		if p.Key.IsData() && !settings.RecordsData() {
			continue
		}
		attrs = append(attrs, attr(p.Key.String(), v))
	}
	span.SetAttributes(attrs...)
}
