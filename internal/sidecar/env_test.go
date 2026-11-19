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

// The data settings anfra sets in its environment reach each sidecar, and a
// sidecar's prefix changes one for that sidecar alone.
func TestTheDataSettingsReachTheSidecar(t *testing.T) {
	environ := []string{
		"H_OTEL_DATA_INPUTS=off",
		"H_OTEL_DATA_ERRORS=masked",
		"ANFRA_NODE_ENV_H_OTEL_DATA_ERRORS=raw",
	}
	if got, want := childEnv(environ, "ANFRA_NODE_ENV_", nil, nil), []string{"H_OTEL_DATA_INPUTS=off", "H_OTEL_DATA_ERRORS=raw"}; !slices.Equal(got, want) {
		t.Errorf("anfra-node:\n got %q\nwant %q", got, want)
	}
	if got, want := childEnv(environ, "ANFRA_CANAL_QUERY_ENV_", nil, nil), []string{"H_OTEL_DATA_INPUTS=off", "H_OTEL_DATA_ERRORS=masked"}; !slices.Equal(got, want) {
		t.Errorf("canal-query:\n got %q\nwant %q", got, want)
	}
}
