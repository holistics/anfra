package telemetry

import (
	"context"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	caller "github.com/holistics/anfra/shared/telemetry"
)

const (
	canalQueryEnv = "ANFRA_CANAL_QUERY_ENV_"
	canalLakeEnv  = "ANFRA_CANAL_LAKE_ENV_"
)

// clearEnv unsets every variable Setup reads, restoring them after the test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OTEL_SDK_DISABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"H_OTEL_ENABLED", "H_OTEL_INTERNAL_EXPORTER_URL", "H_OTEL_TRACES_EXPORTER",
		"OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL",
		"OTEL_EXPORTER_OTLP_TIMEOUT", "OTEL_EXPORTER_OTLP_TRACES_TIMEOUT", "OTEL_BSP_EXPORT_TIMEOUT",
		canalQueryEnv + "OTEL_EXPORTER_OTLP_ENDPOINT", canalQueryEnv + "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		canalQueryEnv + "OTEL_EXPORTER_OTLP_PROTOCOL", canalQueryEnv + "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL",
		canalQueryEnv + "OTEL_EXPORTER_OTLP_TIMEOUT", canalQueryEnv + "OTEL_METRICS_EXPORTER", canalQueryEnv + "OTEL_LOGS_EXPORTER",
		canalLakeEnv + "OTEL_EXPORTER_OTLP_ENDPOINT", canalLakeEnv + "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("ANFRA_HOME", t.TempDir()) // no machine settings file
	before := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(before) })
}

func setup(t *testing.T) bool {
	t.Helper()
	shutdown, err := Setup(context.Background(), "anfra", "test", caller.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	_, installed := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	return installed
}

// Without an endpoint, spans still get ids, for the log, and pass them on, but
// are not sampled, so nothing is exported; anfra-node traces for its ids too,
// exporting nothing.
func TestIdsWithoutAnEndpoint(t *testing.T) {
	clearEnv(t)
	if !setup(t) {
		t.Fatal("no tracer provider was installed with no endpoint set")
	}
	_, span := otel.Tracer("test").Start(context.Background(), "op")
	defer span.End()
	if sc := span.SpanContext(); !sc.IsValid() || sc.IsSampled() {
		t.Errorf("span context = %+v, want valid ids, not sampled", sc)
	}
	if os.Getenv("H_OTEL_ENABLED") != "1" || os.Getenv("H_OTEL_TRACES_EXPORTER") != "none" {
		t.Errorf("anfra-node: H_OTEL_ENABLED=%q H_OTEL_TRACES_EXPORTER=%q, want 1 and none",
			os.Getenv("H_OTEL_ENABLED"), os.Getenv("H_OTEL_TRACES_EXPORTER"))
	}
	if os.Getenv("H_OTEL_INTERNAL_EXPORTER_URL") != "" {
		t.Error("anfra-node was given an exporter URL with no endpoint set")
	}
	if got := os.Getenv(canalQueryEnv + "OTEL_EXPORTER_OTLP_ENDPOINT"); got != "" {
		t.Errorf("canal-query was given endpoint %q with no endpoint set", got)
	}
}

// Without an endpoint, a caller's sampled trace samples nothing either: its
// spans keep the caller's trace id, for the log, but record nothing, so no
// work goes into what would never be exported, and the sidecars are told not
// to record (the flag they're sent is unsampled).
func TestACallersSampledTraceRecordsNothingWithoutAnEndpoint(t *testing.T) {
	clearEnv(t)
	setup(t)
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	_, span := otel.Tracer("test").Start(trace.ContextWithRemoteSpanContext(context.Background(), parent), "op")
	defer span.End()
	sc := span.SpanContext()
	if sc.TraceID() != parent.TraceID() || !sc.SpanID().IsValid() {
		t.Errorf("span context = %+v, want the caller's trace id and an id of its own", sc)
	}
	if span.IsRecording() || sc.IsSampled() {
		t.Errorf("recording %v, sampled %v; want neither", span.IsRecording(), sc.IsSampled())
	}
}

func TestOffWhenDisabled(t *testing.T) {
	clearEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	t.Setenv("OTEL_SDK_DISABLED", "true")
	if setup(t) {
		t.Fatal("a tracer provider was installed with OTEL_SDK_DISABLED=true")
	}
}

func TestExportsToTheEndpoint(t *testing.T) {
	cases := []struct{ name, env, value, node string }{
		{"base", "OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318/", "http://localhost:4318"},
		{"traces", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://collector:4318/v1/traces", "http://collector:4318"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(c.env, c.value)
			if !setup(t) {
				t.Fatal("no tracer provider was installed")
			}
			if got := os.Getenv("H_OTEL_ENABLED"); got != "1" {
				t.Errorf("H_OTEL_ENABLED = %q, want 1", got)
			}
			if got := os.Getenv("H_OTEL_TRACES_EXPORTER"); got != "" {
				t.Errorf("H_OTEL_TRACES_EXPORTER = %q, want it unset, to export", got)
			}
			if got := os.Getenv("H_OTEL_INTERNAL_EXPORTER_URL"); got != c.node {
				t.Errorf("H_OTEL_INTERNAL_EXPORTER_URL = %q, want %q", got, c.node)
			}
			canalExportsNothing(t)
		})
	}
}

// canalExportsNothing checks canal-query and canal-lake are each given an
// empty endpoint, set with its prefix, over the one it would inherit: the
// release anfra ships exports over gRPC, which the telemetry stack's OTLP/HTTP
// endpoint refuses.
func canalExportsNothing(t *testing.T) {
	t.Helper()
	for sidecar, prefix := range map[string]string{"canal-query": canalQueryEnv, "canal-lake": canalLakeEnv} {
		for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
			if got, set := os.LookupEnv(prefix + name); !set || got != "" {
				t.Errorf("%s's %s = %q (set %v), want it set empty", sidecar, name, got, set)
			}
		}
	}
}

func TestKeepsAnfraNodesOwnSettings(t *testing.T) {
	clearEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	t.Setenv("H_OTEL_ENABLED", "0")
	setup(t)
	if got := os.Getenv("H_OTEL_ENABLED"); got != "0" {
		t.Errorf("H_OTEL_ENABLED = %q, want anfra-node's own 0 kept", got)
	}
	if got := os.Getenv("H_OTEL_INTERNAL_EXPORTER_URL"); got != "" {
		t.Errorf("H_OTEL_INTERNAL_EXPORTER_URL = %q, want it left unset", got)
	}
}

// canal-query exporting over gRPC, set with its prefix, is left as it is,
// and canal-lake, without its own, still exports nothing.
func TestKeepsCanalQuerysOwnSettings(t *testing.T) {
	clearEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
	t.Setenv(canalQueryEnv+"OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
	setup(t)
	if got := os.Getenv(canalQueryEnv + "OTEL_EXPORTER_OTLP_ENDPOINT"); got != "http://localhost:4317" {
		t.Errorf("canal-query's endpoint = %q, want its own kept", got)
	}
	if got := os.Getenv(canalQueryEnv + "OTEL_EXPORTER_OTLP_PROTOCOL"); got != "" {
		t.Errorf("canal-query's protocol = %q, want it left unset, for its gRPC default", got)
	}
	if got, set := os.LookupEnv(canalLakeEnv + "OTEL_EXPORTER_OTLP_ENDPOINT"); !set || got != "" {
		t.Errorf("canal-lake's endpoint = %q (set %v), want it set empty", got, set)
	}
}

// Every sidecar this process spawns is told the data setting, even at its
// default, since one without it records everything as it is.
func TestTheSidecarsAreToldTheDataSetting(t *testing.T) {
	clearEnv(t)
	t.Setenv("H_OTEL_DATA", "")
	setup(t)
	if got := os.Getenv("H_OTEL_DATA"); got != "0" {
		t.Errorf("by default, H_OTEL_DATA = %q, want 0", got)
	}
	shutdown, err := Setup(context.Background(), "anfra", "test", caller.NewSettings(true, 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	if got := os.Getenv("H_OTEL_DATA"); got != "1" {
		t.Errorf("with data on, H_OTEL_DATA = %q, want 1", got)
	}
}
