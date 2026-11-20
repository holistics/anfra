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
// an endpoint, nothing is sampled, even under a sampled caller's span, and
// nothing is exported: each span still has its ids, and passes them on
// (traceparent). With one, a span is sampled when it has no caller's, or when
// its caller's is.
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
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0" // the SDK resource's own, so their schema URLs agree

	"github.com/holistics/anfra/internal/sidecar/canallake"
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
	// Exporting nothing samples nothing, even under a sampled caller's span:
	// spans still get ids, for log records.
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.NeverSample()),
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
	configureCanal()
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

// configureCanal keeps the canal-query and canal-lake this process spawns
// from exporting, each unless its own endpoint is set with its prefix
// (canalquery.EnvPrefix, canallake.EnvPrefix). The release anfra ships
// (manifest.yml's anfra_canal) exports traces, metrics and logs over gRPC
// whenever OTEL_EXPORTER_OTLP_ENDPOINT is set, whatever its protocol and
// OTEL_<SIGNAL>_EXPORTER say, so at an OTLP/HTTP collector, as the telemetry
// stack is, it logs a failed export every few seconds. So each gets an empty
// endpoint, set with its prefix, which wins over the one it would inherit from
// anfra. A prefixed endpoint the user set, for a collector that takes gRPC on
// port 4317, is left as it is. A release that exports over OTLP/HTTP can be
// pointed at anfra's collector instead.
func configureCanal() {
	for _, p := range []string{canalquery.EnvPrefix, canallake.EnvPrefix} {
		if os.Getenv(p+"OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv(p+"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
			continue
		}
		for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
			_ = os.Setenv(p+name, "")
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
