package httpkit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/shared/httpkit"
	"github.com/holistics/anfra/shared/telemetry"
)

// Of any caller's baggage, trusted or not, the members the settings allow name
// it on the request's spans, and reach the handler, to go on to the sidecars;
// the others do neither.
func TestOnlyTheAllowedBaggageGetsIn(t *testing.T) {
	for _, trusted := range []bool{true, false} {
		rec := tracetest.NewSpanRecorder()
		prev, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.Spans{}), sdktrace.WithSpanProcessor(rec)))
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
		var seen baggage.Baggage
		h := httpkit.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = baggage.FromContext(r.Context())
		}), httpkit.Config{TrustTraceparent: trusted})
		req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
		req.Header.Set("Baggage", "gen_ai.conversation.id=conv-1,tenant.id=acme")
		h.ServeHTTP(httptest.NewRecorder(), req)
		otel.SetTracerProvider(prev)
		otel.SetTextMapPropagator(prevProp)

		recorded := map[string]string{}
		for _, a := range rec.Ended()[0].Attributes() {
			recorded[string(a.Key)] = a.Value.String()
		}
		if seen.Len() != 1 || seen.Member("gen_ai.conversation.id").Value() != "conv-1" {
			t.Errorf("trusted %v: the handler saw %q, want the session alone", trusted, seen)
		}
		if _, tenant := recorded["tenant.id"]; recorded["gen_ai.conversation.id"] != "conv-1" || tenant {
			t.Errorf("trusted %v: the span recorded %v, want the session alone", trusted, recorded)
		}
	}
}
