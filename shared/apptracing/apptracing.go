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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
)

// tracer follows the global provider, including one installed after this
// package is initialised.
var tracer = otel.Tracer("github.com/holistics/anfra/shared/apptracing")

// Span is one running step.
type Span struct {
	step  appstep.Step
	span  trace.Span
	ended bool
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
		attrs = append(attrs, attr("step."+k, v))
	}
	// Ended by the caller, through Span.End: spancheck checks each caller instead
	// (extra-start-span-signatures in .golangci.yml).
	ctx, span := tracer.Start(ctx, step.Name(), trace.WithAttributes(attrs...)) //nolint:spancheck // see above
	return ctx, &Span{step: step, span: span}
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
		s.span.RecordError(fmt.Errorf("panic: %v", p), trace.WithStackTrace(true))
		s.span.SetStatus(codes.Error, "panic")
		s.span.End()
		panic(p)
	}
	defer s.span.End()
	if errp == nil || *errp == nil {
		return
	}
	*errp = apperr.WithStep(*errp, s.step)

	code := apperr.From(*errp).Code
	s.span.SetAttributes(attribute.String("error.type", code.String()))
	if code.Public().Scope() == apperr.Server {
		// err.Error() is the log form: sensitive parameters already redacted.
		s.span.RecordError(*errp)
		s.span.SetStatus(codes.Error, code.Public().String())
	}
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
