package telemetry

import (
	"context"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// clearEnv unsets every variable Setup reads, restoring them after the test.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"OTEL_SDK_DISABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"H_OTEL_ENABLED", "H_OTEL_INTERNAL_EXPORTER_URL",
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	before := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(before) })
}

func setup(t *testing.T) bool {
	t.Helper()
	shutdown, err := Setup(context.Background(), "anfra", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	_, installed := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	return installed
}

func TestOffWithoutAnEndpoint(t *testing.T) {
	clearEnv(t)
	if setup(t) {
		t.Fatal("a tracer provider was installed with no endpoint set")
	}
	if os.Getenv("H_OTEL_ENABLED") != "" {
		t.Fatal("anfra-node was told to export with no endpoint set")
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
			if got := os.Getenv("H_OTEL_INTERNAL_EXPORTER_URL"); got != c.node {
				t.Errorf("H_OTEL_INTERNAL_EXPORTER_URL = %q, want %q", got, c.node)
			}
		})
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
