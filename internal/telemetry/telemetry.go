// Package telemetry installs OpenTelemetry for the anfra CLI and `anfra serve`:
// the global tracer provider and W3C propagation, which the spans anfra already
// makes (apptracing's per op, httpkit's per request) go to.
//
// Always on, for the ids it gives each span: log records carry them, so one
// request's records join across services (docs/designs/logs.md). Exported only
// when an endpoint is set. Everything is configured by the standard OTEL_*
// environment variables, which the SDK and its exporter read themselves:
// OTEL_EXPORTER_OTLP_ENDPOINT (or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT) turns
// export on, over OTLP/HTTP; OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES
// label it; and OTEL_SDK_DISABLED=true turns it all off, ids included. Without
// an endpoint, a span is sampled only when its caller's is, and nothing is
// exported: each still has its ids, and passes them on (traceparent).
//
// A copy of anfra-cloud's shared/telemetry, trimmed to traces and over HTTP, as
// anfra-node exports: one collector endpoint serves both.
package telemetry

import (
	"context"
	"errors"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0" // the SDK resource's own, so their schema URLs agree
)

// Setup installs the tracer provider the environment asks for, and returns the
// function that flushes and stops it; call it on the way out, with a deadline.
// On error nothing is installed.
func Setup(ctx context.Context, service, version string) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	if os.Getenv("OTEL_SDK_DISABLED") == "true" {
		return noop, nil
	}
	base := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	traces := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	exporting := base != "" || traces != ""

	opts := []sdktrace.TracerProviderOption{sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.NeverSample()))}
	if exporting {
		res, err := newResource(ctx, service, version)
		if err != nil {
			return noop, err
		}
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return noop, err
		}
		opts = []sdktrace.TracerProviderOption{
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
			sdktrace.WithResource(res),
			sdktrace.WithBatcher(exp),
		}
	}
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	if exporting {
		exportFromNode(base, traces)
	}
	return tp.Shutdown, nil
}

// exportFromNode points the anfra-node this process spawns, which inherits its
// environment, at the same collector, unless it is configured already. Its
// h-otel reads its own variables, and takes the base URL, adding /v1/traces.
func exportFromNode(base, traces string) {
	if os.Getenv("H_OTEL_ENABLED") != "" {
		return
	}
	if base == "" {
		base = strings.TrimSuffix(strings.TrimSuffix(traces, "/"), "/v1/traces")
	}
	_ = os.Setenv("H_OTEL_ENABLED", "1")
	_ = os.Setenv("H_OTEL_INTERNAL_EXPORTER_URL", strings.TrimSuffix(base, "/"))
}

// newResource describes this process. OTEL_SERVICE_NAME and
// OTEL_RESOURCE_ATTRIBUTES override what is set here (WithFromEnv comes last).
func newResource(ctx context.Context, service, version string) (*resource.Resource, error) {
	r, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithAttributes(semconv.ServiceName(service), semconv.ServiceVersion(version)),
		resource.WithFromEnv(),
	)
	// A partial resource still describes the process: a host or process detail
	// that could not be read is not worth refusing to start over.
	if errors.Is(err, resource.ErrPartialResource) || errors.Is(err, resource.ErrSchemaURLConflict) {
		return r, nil
	}
	return r, err
}
