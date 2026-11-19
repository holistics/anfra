package apikit_test

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/shared/telemetry"
)

// recorder keeps every span that ends. It is installed once: the global
// provider a package's tracer delegates to is the first one set.
var spanRecorder = sync.OnceValue(func() *tracetest.SpanRecorder {
	rec := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	return rec
})

// spansFrom are the spans that end after it is called.
type spansFrom int

func record(*testing.T) spansFrom { return spansFrom(len(spanRecorder().Ended())) }

// events are the op span's events, by name, as their payload attribute, and its
// attributes.
func events(t *testing.T, from spansFrom) (map[string]string, map[string]string) {
	t.Helper()
	spans := spanRecorder().Ended()[from:]
	if len(spans) != 1 {
		t.Fatalf("got %d spans", len(spans))
	}
	got := map[string]string{}
	for _, e := range spans[0].Events() {
		for _, kv := range e.Attributes {
			if kv.Key == "anfra.payload" {
				got[e.Name] = kv.Value.AsString()
			}
		}
	}
	attrs := map[string]string{}
	for _, kv := range spans[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.String()
	}
	return got, attrs
}

// Every op's span names it, and records nothing of its input or answer by
// default: inputs and outputs are off.
func TestPayloadsAreOffByDefault(t *testing.T) {
	rec := record(t)
	o := one(t, greet("greetings.create"))
	if _, err := o.Invoke(context.Background(), runtime(), request{user: "ann"}, []byte(`{"name":"Zed"}`)); err != nil {
		t.Fatal(err)
	}
	got, attrs := events(t, rec)
	if len(got) != 0 {
		t.Errorf("payloads recorded with payloads off: %v", got)
	}
	if attrs[telemetry.Op.String()] != "greetings.create" {
		t.Errorf("anfra.op = %q", attrs[telemetry.Op.String()])
	}
}

// With inputs and outputs raw, the input is recorded as sent and the answer as
// JSON, each cut at the limit.
func TestPayloadsWhenOn(t *testing.T) {
	rec := record(t)
	o := one(t, greet("greetings.create"))
	ctx := telemetry.WithSettings(context.Background(), raw(1024))
	if _, err := o.Invoke(ctx, runtime(), request{user: "ann"}, []byte(`{"name":"Zed"}`)); err != nil {
		t.Fatal(err)
	}
	got, _ := events(t, rec)
	if got[telemetry.OpInput.String()] != `{"name":"Zed"}` {
		t.Errorf("input = %q", got[telemetry.OpInput.String()])
	}
	if got[telemetry.OpOutput.String()] != `{"text":"ann greets Zed","tags":[]}` {
		t.Errorf("output = %q", got[telemetry.OpOutput.String()])
	}
}

func TestAPayloadOverTheLimitIsCutAndMarked(t *testing.T) {
	rec := record(t)
	o := one(t, greet("greetings.create"))
	ctx := telemetry.WithSettings(context.Background(), raw(5))
	if _, err := o.Invoke(ctx, runtime(), request{user: "ann"}, []byte(`{"name":"Zed"}`)); err != nil {
		t.Fatal(err)
	}
	got, _ := events(t, rec)
	if got[telemetry.OpInput.String()] != `{"nam…` {
		t.Errorf("input = %q", got[telemetry.OpInput.String()])
	}
}

// With data off, no payload is recorded.
func TestNoPayloadWithDataOff(t *testing.T) {
	rec := record(t)
	o := one(t, greet("greetings.create"))
	ctx := telemetry.WithSettings(context.Background(), telemetry.Defaults())
	if _, err := o.Invoke(ctx, runtime(), request{user: "ann"}, []byte(`{"name":"Zed"}`)); err != nil {
		t.Fatal(err)
	}
	if got, _ := events(t, rec); got[telemetry.OpInput.String()] != "" || got[telemetry.OpOutput.String()] != "" {
		t.Errorf("with data off, payloads = %v, want none", got)
	}
}

// raw is data on, each payload cut at limit bytes.
func raw(limit int) telemetry.Settings {
	return telemetry.NewSettings(true, limit)
}
