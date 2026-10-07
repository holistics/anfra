package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"

	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/telemetry"
)

// tracer starts the CLI's own spans: a command's root.
var tracer = otel.Tracer("github.com/holistics/anfra/cmd/anfra")

// flushTimeout bounds how long exiting waits on the last spans: a collector that
// is down must not hold up a command that is done.
const flushTimeout = 2 * time.Second

// startTelemetry installs tracing, if the environment asks for it, and returns
// the function that flushes it. Telemetry never stops a command: a setup that
// fails is reported and the command runs untraced.
func startTelemetry(ctx context.Context) (flush func()) {
	shutdown, err := telemetry.Setup(ctx, "anfra", meta.Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "anfra: telemetry is off: %v\n", err)
		return func() {}
	}
	return func() {
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
		defer cancel()
		_ = shutdown(flushCtx)
	}
}

// traceSidecar traces each call to a sidecar, and carries the trace into it.
// Without telemetry the spans are no-ops and nothing is propagated.
func traceSidecar(sidecar string, rt http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(rt, otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return sidecar + " " + r.Method + " " + r.URL.Path
	}))
}
