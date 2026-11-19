package cloudapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/internal/cloudapi"
)

// A call to the org's server is a span in the command's trace, named for its
// op, and the trace isn't sent to the server: it is the user's.
func TestACallIsASpanThatStaysHere(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTracerProvider(prevTP); otel.SetTextMapPropagator(prevProp) })

	var traceparent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("traceparent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	org, err := cloudapi.ParseOrg(server.URL + "/o/acme")
	if err != nil {
		t.Fatal(err)
	}

	ctx, root := tp.Tracer("test").Start(context.Background(), "anfra publish")
	_, _ = cloudapi.NewClient(org, cloudapi.ClientOptions{}).Status(ctx)
	root.End()

	if traceparent != "" {
		t.Errorf("the org's server got traceparent %q, want none", traceparent)
	}
	var found bool
	for _, s := range rec.Ended() {
		if s.Name() == "org repos.status" {
			found = s.Parent().SpanID() == root.SpanContext().SpanID()
		}
	}
	if !found {
		t.Error("no span org repos.status under the command's")
	}
}
