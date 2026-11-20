package engine_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/engine"
	"github.com/holistics/anfra/shared/telemetry"
)

// A platform that embeds the engine traces its own work, such as its
// assistant's tool call, and dispatches a command under it: the command's
// spans go under the platform's span, in its trace, and each carries the ids
// the platform names its caller by (Attribution), anfra's and its own.
func TestDispatchNestsUnderTheCallersSpanWithItsIDs(t *testing.T) {
	raw := tracetest.NewSpanRecorder()      // every span, as recorded
	exported := tracetest.NewSpanRecorder() // what a platform's exporter gets, through anfra's filter
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(telemetry.Spans{}),
		sdktrace.WithSpanProcessor(raw),
		sdktrace.WithSpanProcessor(telemetry.Filter(exported, telemetry.Settings{})),
	)
	ctx, tool := tp.Tracer("platform").Start(context.Background(), "assistant.tool_call")
	defer tool.End()
	inv := engine.Invocation{
		DataPerms:   engine.Ungoverned(),
		Attribution: engine.Attribution{"gen_ai.conversation.id": "conv-1", "tenant.id": "acme"},
	}
	// The global provider is what anfra's spans start on.
	restore := useProvider(tp)
	defer restore()
	if _, err := engine.Dispatch(ctx, inv, engine.Request{Command: "version"}); err != nil {
		t.Fatal(err)
	}

	spans := raw.Ended()
	if len(spans) == 0 {
		t.Fatal("the command recorded no span")
	}
	for _, s := range spans {
		if s.SpanContext().TraceID() != tool.SpanContext().TraceID() {
			t.Errorf("%s: trace %s, want the platform's %s", s.Name(), s.SpanContext().TraceID(), tool.SpanContext().TraceID())
		}
		if !s.Parent().IsValid() {
			t.Errorf("%s: a root, want it under the platform's span", s.Name())
		}
		attrs := map[string]string{}
		for _, a := range s.Attributes() {
			attrs[string(a.Key)] = a.Value.String()
		}
		if attrs["gen_ai.conversation.id"] != "conv-1" || attrs["tenant.id"] != "acme" {
			t.Errorf("%s, as recorded: gen_ai.conversation.id %q, tenant.id %q", s.Name(), attrs["gen_ai.conversation.id"], attrs["tenant.id"])
		}
	}
	for _, s := range exported.Ended() {
		attrs := map[string]string{}
		for _, a := range s.Attributes() {
			attrs[string(a.Key)] = a.Value.String()
		}
		if attrs["gen_ai.conversation.id"] != "conv-1" || attrs["tenant.id"] != "acme" {
			t.Errorf("%s, as exported: gen_ai.conversation.id %q, tenant.id %q", s.Name(), attrs["gen_ai.conversation.id"], attrs["tenant.id"])
		}
	}
}

// useProvider installs tp as the global tracer provider, and returns the
// function that puts the one before back.
func useProvider(tp *sdktrace.TracerProvider) (restore func()) {
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	return func() { otel.SetTracerProvider(prev) }
}
