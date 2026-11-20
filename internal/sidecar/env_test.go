package sidecar

import (
	"slices"
	"testing"
)

// A sidecar inherits the machine's environment but not anfra's own names or
// the per-process ones; its prefix sets those for it alone, and what its
// supervisor sets wins over both, except its defaults, which the prefix changes.
func TestChildEnv(t *testing.T) {
	environ := []string{
		"HOME=/home/me",
		"AWS_PROFILE=prod",
		"ANFRA_HOME=/home/me/.anfra",   // anfra's own
		"ANFRA_CANAL_QUERY_URL=",       // anfra's own, though about the sidecar
		"PPROF_ADDR=localhost:6060",    // per process
		"OTEL_SERVICE_NAME=anfra-prod", // per process
		"TRACEPARENT=00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", // the caller's call
		"BAGGAGE=gen_ai.conversation.id=s-1",
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318",
		"ANFRA_CANAL_QUERY_ENV_PPROF_ADDR=localhost:6061",
		"ANFRA_CANAL_QUERY_ENV_AWS_PROFILE=warehouse",           // over the inherited one
		"ANFRA_CANAL_QUERY_ENV_PORT=1",                          // the supervisor's own: ignored
		"ANFRA_CANAL_QUERY_ENV_=x",                              // no name
		"ANFRA_NODE_ENV_NODE_OPTIONS=--max-old-space-size=4096", // another sidecar's
		"ANFRA_CANAL_QUERY_ENV_ANFRA_CACHE_SIZE=0",              // over the supervisor's default
	}
	got := childEnv(environ, "ANFRA_CANAL_QUERY_ENV_",
		[]string{"ANFRA_CACHE_SIZE=1", "ANFRA_POOL_SIZE=2"},
		[]string{"PORT=4000", "ANFRA_REPO_ID=r"})
	want := []string{
		"HOME=/home/me",
		"AWS_PROFILE=warehouse",
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318",
		"ANFRA_CACHE_SIZE=0",
		"ANFRA_POOL_SIZE=2",
		"PPROF_ADDR=localhost:6061",
		"PORT=4000",
		"ANFRA_REPO_ID=r",
	}
	if !slices.Equal(got, want) {
		t.Errorf("childEnv:\n got %q\nwant %q", got, want)
	}

	// Without a prefix, nothing is forwarded.
	got = childEnv(environ, "", nil, nil)
	want = []string{"HOME=/home/me", "AWS_PROFILE=prod", "OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318"}
	if !slices.Equal(got, want) {
		t.Errorf("childEnv without a prefix:\n got %q\nwant %q", got, want)
	}
}

// A prefixed empty value replaces the inherited one, so it can turn off what
// a sidecar would otherwise inherit, as canal-query's endpoint.
func TestAnEmptyPrefixedValueWins(t *testing.T) {
	environ := []string{"OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:4318", "ANFRA_CANAL_QUERY_ENV_OTEL_EXPORTER_OTLP_ENDPOINT="}
	if got, want := childEnv(environ, "ANFRA_CANAL_QUERY_ENV_", nil, nil), []string{"OTEL_EXPORTER_OTLP_ENDPOINT="}; !slices.Equal(got, want) {
		t.Errorf("childEnv:\n got %q\nwant %q", got, want)
	}
}

// The data setting anfra sets in its environment reaches each sidecar, and a
// sidecar's prefix changes it for that sidecar alone.
func TestTheDataSettingReachesTheSidecar(t *testing.T) {
	environ := []string{"H_OTEL_DATA=0", "ANFRA_NODE_ENV_H_OTEL_DATA=1"}
	if got, want := childEnv(environ, "ANFRA_NODE_ENV_", nil, nil), []string{"H_OTEL_DATA=1"}; !slices.Equal(got, want) {
		t.Errorf("anfra-node:\n got %q\nwant %q", got, want)
	}
	if got, want := childEnv(environ, "ANFRA_CANAL_QUERY_ENV_", nil, nil), []string{"H_OTEL_DATA=0"}; !slices.Equal(got, want) {
		t.Errorf("canal-query:\n got %q\nwant %q", got, want)
	}
}
