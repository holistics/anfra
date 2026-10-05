package apptracing_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
	"github.com/holistics/anfra/shared/apptracing"
)

var ns = appstep.DefineNamespace("test")

// A user-scope code, as a host defines one.
var unauthenticated = apperr.DefinePublicCode(ns, "unauthenticated", apperr.User, "Sign in to continue.")

var (
	importDashboard = appstep.Define(ns, "import_dashboard", appstep.Public("import dashboard {name}"))
	parseFile       = appstep.Define(ns, "parse_file")
	invite          = appstep.Define(ns, "invite", appstep.Public("invite {email}"), appstep.Sensitive("email"))
)

var errBoom = errors.New("boom")

func importOne(ctx context.Context, name string, fail error) (err error) {
	ctx, span := apptracing.Start(ctx, importDashboard, "name", name)
	defer span.End(&err)
	return parse(ctx, fail)
}

func parse(ctx context.Context, fail error) (err error) {
	_, span := apptracing.Start(ctx, parseFile)
	defer span.End(&err)
	return fail
}

// A failing function gets its step added to its error; nested steps nest, and
// only the public one becomes context.
func TestEndAddsTheStepToAFailingFunctionsError(t *testing.T) {
	err := importOne(context.Background(), "Sales", apperr.New(apperr.NotFound, "No such dataset."))

	if got, want := err.Error(), "test.import_dashboard(name=Sales): test.parse_file: apperr.not_found: No such dataset."; got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
	res := apperr.From(err).Response("req_1")
	want := []apperr.ContextEntry{
		{Step: "test.import_dashboard", Params: map[string]any{"name": "Sales"}, Text: "import dashboard 'Sales'"},
	}
	if !reflect.DeepEqual(res.Context, want) {
		t.Errorf("context = %+v, want %+v", res.Context, want)
	}
}

func TestEndLeavesSuccessAlone(t *testing.T) {
	if err := importOne(context.Background(), "Sales", nil); err != nil {
		t.Errorf("a succeeding function returned %v", err)
	}
}

// A step is context, not a decision: what the error is does not change.
func TestEndKeepsTheMeaning(t *testing.T) {
	err := importOne(context.Background(), "Sales", apperr.New(apperr.NotFound, ""))
	if !errors.Is(err, apperr.NotFound) {
		t.Error("the step hid the code from errors.Is")
	}
	if got, ok := errors.AsType[*apperr.Error](err); !ok || got.Code != apperr.NotFound {
		t.Error("the step hid the formal error from errors.As")
	}
	if apperr.From(err).Code != apperr.NotFound {
		t.Error("the step changed what is rendered")
	}
	if err := importOne(context.Background(), "Sales", errBoom); !errors.Is(err, errBoom) {
		t.Error("the step hid a plain cause from errors.Is")
	}
}

// A second End — an explicit call plus a deferred one — adds the step once.
func TestEndTwiceAddsTheStepOnce(t *testing.T) {
	err := errBoom
	_, span := apptracing.Start(context.Background(), parseFile)
	span.End(&err)
	span.End(&err)
	if got := len(apperr.From(err).Steps); got != 1 {
		t.Errorf("steps = %d, want 1", got)
	}
}

// A sensitive value stays out of the log and reaches the user it came from.
func TestSensitiveParams(t *testing.T) {
	err := apperr.New(apperr.NotFound, "No such tenant.")
	_, span := apptracing.Start(context.Background(), invite, "email", "carol@example.com")
	span.End(&err)

	if got, want := err.Error(), "test.invite(email=[redacted]): apperr.not_found: No such tenant."; got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
	if got := apperr.From(err).Response("req_1").Context[0].Text; got != "invite 'carol@example.com'" {
		t.Errorf("context = %q, want the email shown to the user", got)
	}
}

// recorder captures every span the tests end. Installed globally once: the
// package's tracer follows the global provider.
var recorder = func() *tracetest.SpanRecorder {
	r := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(r)))
	return r
}()

// spans returns the spans ended since the last call, by name.
func spans(t *testing.T) map[string]sdktrace.ReadOnlySpan {
	t.Helper()
	out := map[string]sdktrace.ReadOnlySpan{}
	for _, s := range recorder.Ended() {
		out[s.Name()] = s
	}
	recorder.Reset()
	return out
}

func attrs(s sdktrace.ReadOnlySpan) map[attribute.Key]string {
	out := map[attribute.Key]string{}
	for _, kv := range s.Attributes() {
		out[kv.Key] = kv.Value.String()
	}
	return out
}

// A step is a span named for it, nested as the steps nest, with its parameters
// as attributes.
func TestStartIsASpan(t *testing.T) {
	spans(t)
	_ = importOne(context.Background(), "Sales", nil)
	got := spans(t)
	outer, inner := got["test.import_dashboard"], got["test.parse_file"]
	if outer == nil || inner == nil {
		t.Fatalf("spans = %v", got)
	}
	if inner.Parent().SpanID() != outer.SpanContext().SpanID() {
		t.Error("the inner step is not the outer step's child")
	}
	if a := attrs(outer); a["step.name"] != "Sales" {
		t.Errorf("attributes = %v", a)
	}
	if outer.Status().Code != codes.Unset {
		t.Errorf("a succeeding step has status %v", outer.Status())
	}
}

// A sensitive parameter is redacted from the trace, as from the log.
func TestSpanRedactsSensitiveParams(t *testing.T) {
	spans(t)
	err := errBoom
	_, span := apptracing.Start(context.Background(), invite, "email", "carol@example.com")
	span.End(&err)
	if a := attrs(spans(t)["test.invite"]); a["step.email"] != "[redacted]" {
		t.Errorf("attributes = %v", a)
	}
}

// Only a server-scope failure marks a span as errored; every failure names its
// code.
func TestSpanStatusFollowsScope(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status codes.Code
		typ    string
	}{
		{apperr.New(apperr.NotFound, ""), codes.Unset, "not_found"},
		{apperr.New(unauthenticated, ""), codes.Unset, "unauthenticated"},
		{errBoom, codes.Error, "internal_server_error"},
	} {
		spans(t)
		_ = parse(context.Background(), tc.err)
		s := spans(t)["test.parse_file"]
		if s.Status().Code != tc.status || attrs(s)["error.type"] != tc.typ {
			t.Errorf("%v: status %v, error.type %q", tc.err, s.Status(), attrs(s)["error.type"])
		}
		if recorded := len(s.Events()) > 0; recorded != (tc.status == codes.Error) {
			t.Errorf("%v: error event recorded = %v", tc.err, recorded)
		}
	}
}

func panics(ctx context.Context) (err error) {
	_, span := apptracing.Start(ctx, parseFile)
	defer span.End(&err)
	panic("kaboom")
}

// A panic passing through a step marks its span and goes on unchanged.
func TestSpanSeesAPanic(t *testing.T) {
	spans(t)
	func() {
		defer func() {
			if p := recover(); p != "kaboom" {
				t.Errorf("recovered %v, want the original panic", p)
			}
		}()
		_ = panics(context.Background())
	}()
	if s := spans(t)["test.parse_file"]; s == nil || s.Status().Code != codes.Error {
		t.Errorf("span = %v", s)
	}
}
