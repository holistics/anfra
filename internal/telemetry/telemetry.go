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
// The repo's .env sets these variables too, below the environment, so an
// agent's commands in the repo are traced with nothing set in their
// environment (internal/repoenv, loaded before Setup).
//
// Exporting never holds up a command: each export gives up after
// exportTimeout, under the CLI's wait for its last spans on exit.
//
// A copy of anfra-cloud's shared/telemetry, trimmed to traces and over HTTP, as
// anfra-node exports: one collector endpoint serves both.
package telemetry

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0" // the SDK resource's own, so their schema URLs agree

	"github.com/holistics/anfra/internal/sidecar/canalquery"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// exportTimeout bounds one export, retries included: under the CLI's wait for
// its last spans on exit (2 seconds), so a collector that is down costs a
// command no more than that. OTEL_EXPORTER_OTLP_TIMEOUT and
// OTEL_BSP_EXPORT_TIMEOUT change it.
const exportTimeout = 1500 * time.Millisecond

// Setup installs the tracer provider the environment asks for, and returns the
// function that flushes and stops it; call it on the way out, with a deadline.
// On error nothing is installed.
//
// data is what of the user's data spans record (shared/telemetry): every span
// this process exports follows it, unless its context carries its own, and
// each sidecar this process spawns is told it (configureData).
func Setup(ctx context.Context, service, version string, data caller.Settings) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	configureData(data)
	dest, err := resolve()
	installed = dest
	if err != nil {
		return noop, err
	}
	if dest.Disabled() {
		return noop, nil
	}
	base := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	traces := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	var expOpts []otlptracehttp.Option
	exporting := base != "" || traces != ""
	if os.Getenv("OTEL_EXPORTER_OTLP_TIMEOUT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_TIMEOUT") == "" {
		expOpts = append(expOpts, otlptracehttp.WithTimeout(exportTimeout))
	}
	var bspOpts []sdktrace.BatchSpanProcessorOption
	if os.Getenv("OTEL_BSP_EXPORT_TIMEOUT") == "" {
		bspOpts = append(bspOpts, sdktrace.WithExportTimeout(exportTimeout))
	}

	// The caller's fields go on every span (shared/telemetry), before the batcher
	// takes it.
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.NeverSample())),
		sdktrace.WithSpanProcessor(caller.Spans{}),
	}
	if exporting {
		res, err := newResource(ctx, service, version)
		if err != nil {
			return noop, err
		}
		exp, err := otlptracehttp.New(ctx, expOpts...)
		if err != nil {
			return noop, err
		}
		opts = []sdktrace.TracerProviderOption{
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
			sdktrace.WithResource(res),
			sdktrace.WithSpanProcessor(caller.Spans{}),
			sdktrace.WithSpanProcessor(caller.Filter(sdktrace.NewBatchSpanProcessor(exp, bspOpts...), data)),
		}
	}
	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	configureNode(exporting, base, traces)
	configureCanalQuery(exporting, base, traces)
	return tp.Shutdown, nil
}

// configureData tells the sidecars this process spawns whether their spans
// record the user's data, as a variable they inherit (H_OTEL_DATA): set even
// at its default, since a sidecar without it records everything as it is. A
// sidecar's prefix (ANFRA_NODE_ENV_, ANFRA_CANAL_QUERY_ENV_) sets it for that
// sidecar alone, over this (internal/sidecar, childEnv).
func configureData(data caller.Settings) {
	for _, kv := range caller.SidecarEnv(data) {
		name, value, _ := strings.Cut(kv, "=")
		_ = os.Setenv(name, value)
	}
}

// configureNode sets up tracing in the anfra-node this process spawns, which
// inherits its environment, as here, unless it is configured already: always
// on, for the ids its log records carry, and exporting to the same collector
// when this process does, otherwise nothing (H_OTEL_TRACES_EXPORTER=none). Its
// h-otel reads its own variables, and takes the base URL, adding /v1/traces.
func configureNode(exporting bool, base, traces string) {
	if os.Getenv("H_OTEL_ENABLED") != "" {
		return
	}
	_ = os.Setenv("H_OTEL_ENABLED", "1")
	if !exporting {
		_ = os.Setenv("H_OTEL_TRACES_EXPORTER", "none")
		return
	}
	if base == "" {
		base = strings.TrimSuffix(strings.TrimSuffix(traces, "/"), "/v1/traces")
	}
	_ = os.Setenv("H_OTEL_INTERNAL_EXPORTER_URL", strings.TrimSuffix(base, "/"))
}

// configureCanalQuery points the canal-query this process spawns at the same
// collector, over OTLP/HTTP, when this process exports. It is set with
// canal-query's own prefix (canalquery.EnvPrefix), so it reaches canal-query
// only, and anfra's own OTEL_* variables still say what the user set. A
// prefixed endpoint the user set wins, with its protocol: canal-query is then
// configured already, as over gRPC to port 4317.
//
// canal-query exports traces only, as anfra does: its metrics and logs would be
// refused by a backend that takes only traces. Its exports give up after
// exportTimeout too, so stopping it never waits on a collector that is down.
func configureCanalQuery(exporting bool, base, traces string) {
	p := canalquery.EnvPrefix
	if !exporting || os.Getenv(p+"OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv(p+"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
		return
	}
	if base == "" {
		base = strings.TrimSuffix(strings.TrimSuffix(traces, "/"), "/v1/traces")
	}
	for name, value := range map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":        strings.TrimSuffix(base, "/"),
		"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL": "http/protobuf",
		"OTEL_EXPORTER_OTLP_PROTOCOL":        "http/protobuf",
		"OTEL_EXPORTER_OTLP_TIMEOUT":         strconv.Itoa(int(exportTimeout.Milliseconds())),
		"OTEL_METRICS_EXPORTER":              "none",
		"OTEL_LOGS_EXPORTER":                 "none",
	} {
		if os.Getenv(p+name) == "" {
			_ = os.Setenv(p+name, value)
		}
	}
}

// newResource describes this process. OTEL_SERVICE_NAME and
// OTEL_RESOURCE_ATTRIBUTES override what is set here (WithFromEnv comes last).
//
// Every process detail but its command line, which is the command's arguments:
// inputs, recorded only as they say (anfra.command.input).
func newResource(ctx context.Context, service, version string) (*resource.Resource, error) {
	r, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcessPID(),
		resource.WithProcessExecutableName(),
		resource.WithProcessExecutablePath(),
		resource.WithProcessOwner(),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
		resource.WithProcessRuntimeDescription(),
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
