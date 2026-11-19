package telemetry

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/holistics/anfra/internal/repoenv"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// loadRepoEnv makes a repo whose .env is content, and loads it, as anfra does
// when it starts in the repo.
func loadRepoEnv(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".anfra"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, repoenv.FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := repoenv.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range l.Set {
		t.Cleanup(func() { _ = os.Unsetenv(name) })
	}
}

// The endpoint in the repo's .env turns export on when the environment sets
// none, for the sidecars too, and says it came from the file.
func TestTheEndpointInTheRepoEnv(t *testing.T) {
	clearEnv(t)
	loadRepoEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:14318\n")
	if got := Current(); got.URL != "http://localhost:14318" || got.From != "OTEL_EXPORTER_OTLP_ENDPOINT in .env" {
		t.Errorf("Current = %+v", got)
	}
	setup(t)
	if got := os.Getenv("H_OTEL_INTERNAL_EXPORTER_URL"); got != "http://localhost:14318" {
		t.Errorf("anfra-node's endpoint = %q", got)
	}
	if got := os.Getenv(canalQueryEnv + "OTEL_EXPORTER_OTLP_ENDPOINT"); got != "http://localhost:14318" {
		t.Errorf("canal-query's endpoint = %q", got)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "op")
	span.End()
	if !span.SpanContext().IsSampled() {
		t.Error("the endpoint in .env did not turn export on")
	}
}

// The environment wins over the repo's .env.
func TestTheEnvironmentWinsOverTheRepoEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
	loadRepoEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:14318\n")
	if got := Current(); got.URL != "http://collector:4318" || got.From != "OTEL_EXPORTER_OTLP_ENDPOINT" {
		t.Errorf("Current = %+v, want the variable to win", got)
	}
}

// A collector that takes the connection and never answers costs a command its
// flush, and no more.
func TestACollectorThatHangsDoesNotHoldUpExit(t *testing.T) {
	clearEnv(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close() // held open, never answered
		}
	}()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://"+ln.Addr().String())
	shutdown, err := Setup(context.Background(), "anfra", "test", caller.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "op")
	span.End()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_ = shutdown(ctx)
	if took := time.Since(start); took > exportTimeout+500*time.Millisecond {
		t.Errorf("shutdown took %v, want it bounded by the export timeout (%v)", took, exportTimeout)
	}
}

func TestAWarningForGRPC(t *testing.T) {
	clearEnv(t)
	if w := ProtocolWarning(); w != "" {
		t.Errorf("unset: %q", w)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")
	if w := ProtocolWarning(); !strings.Contains(w, "OTEL_EXPORTER_OTLP_PROTOCOL=grpc") {
		t.Errorf("grpc: %q", w)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "http/protobuf")
	if w := ProtocolWarning(); w != "" {
		t.Errorf("traces' own http/protobuf wins: %q", w)
	}
}

// A process keeps the endpoint it started with: Installed doesn't follow a
// change to the variables, so a server's failure hint names where it sends.
func TestInstalledIsReadOnce(t *testing.T) {
	clearEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:14318")
	setup(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:24318")
	if got := Installed().URL; got != "http://localhost:14318" {
		t.Errorf("Installed = %q, want the endpoint the process started with", got)
	}
	if got := Current().URL; got != "http://localhost:24318" {
		t.Errorf("Current = %q, want the variables as they are now", got)
	}
}

// A backend's key in OTEL_EXPORTER_OTLP_HEADERS goes with each export, also
// when both are in the repo's .env (docs/usage/configs.md).
func TestHeadersInTheRepoEnvGoWithEachExport(t *testing.T) {
	clearEnv(t)
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case got <- r.Header.Get("X-Api-Key"):
		default:
		}
	}))
	defer srv.Close()
	loadRepoEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT="+srv.URL+"\nOTEL_EXPORTER_OTLP_HEADERS=x-api-key=secret\n")

	shutdown, err := Setup(context.Background(), "anfra", "test", caller.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "op")
	span.End()
	_ = shutdown(context.Background())
	select {
	case key := <-got:
		if key != "secret" {
			t.Errorf("X-Api-Key = %q, want the header from OTEL_EXPORTER_OTLP_HEADERS", key)
		}
	default:
		t.Fatal("nothing was exported")
	}
}

// Data on by default, with traces leaving the machine, is warned of, naming
// the host and how to choose; setting the switch, either way, stops it, and a
// loopback endpoint needs none.
func TestDataByDefaultIsWarnedOfForAnotherMachine(t *testing.T) {
	remote := Destination{URL: "https://otel.example.com", From: "OTEL_EXPORTER_OTLP_ENDPOINT"}
	codes := func(dest Destination, env map[string]string) []string {
		data, err := caller.Resolve(func(k string) string { return env[k] })
		if err != nil {
			t.Fatal(err)
		}
		var c []string
		for _, w := range GuardWarnings(dest, data) {
			c = append(c, w.Code)
			if w.Code == "data_by_default" && (!strings.Contains(w.Message, "otel.example.com") || !strings.Contains(w.Message, "ANFRA_EXPERIMENTAL_TRACE_DATA=0")) {
				t.Errorf("message = %q, want the host and the switch", w.Message)
			}
		}
		return c
	}
	if got := codes(remote, nil); !slices.Equal(got, []string{"data_by_default"}) {
		t.Errorf("by default, remote: %v", got)
	}
	for _, v := range []string{"0", "1"} {
		if got := codes(remote, map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": v}); len(got) != 0 {
			t.Errorf("set to %s, remote: %v, want none", v, got)
		}
	}
	if got := codes(Destination{URL: "http://localhost:4318", From: "OTEL_EXPORTER_OTLP_ENDPOINT"}, nil); len(got) != 0 {
		t.Errorf("by default, loopback: %v, want none", got)
	}
}

// Data .env turns on, with traces leaving the machine, is warned of, naming
// both; so is plain http:// to another machine. Nothing is said for a
// loopback endpoint.
func TestGuardWarnings(t *testing.T) {
	clearEnv(t)
	t.Setenv("ANFRA_EXPERIMENTAL_TRACE_DATA", "")
	_ = os.Unsetenv("ANFRA_EXPERIMENTAL_TRACE_DATA") // unset, so the file sets it
	loadRepoEnv(t, "ANFRA_EXPERIMENTAL_TRACE_DATA=1\n")
	data, err := caller.Resolve(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	codes := func(dest Destination) []string {
		var c []string
		for _, w := range GuardWarnings(dest, data) {
			c = append(c, w.Code)
			if w.Code == "data_from_env_file" && (!strings.Contains(w.Message, "ANFRA_EXPERIMENTAL_TRACE_DATA=1") || !strings.Contains(w.Message, "otel.example.com")) {
				t.Errorf("message = %q, want the file's setting and the host", w.Message)
			}
		}
		return c
	}
	if got := codes(Destination{URL: "http://localhost:4318", From: "OTEL_EXPORTER_OTLP_ENDPOINT"}); len(got) != 0 {
		t.Errorf("loopback: %v, want none", got)
	}
	if got := codes(Destination{URL: "https://otel.example.com", From: "OTEL_EXPORTER_OTLP_ENDPOINT in .env"}); !slices.Equal(got, []string{"data_from_env_file"}) {
		t.Errorf("https elsewhere: %v", got)
	}
	if got := codes(Destination{URL: "http://otel.example.com:4318", From: "OTEL_EXPORTER_OTLP_ENDPOINT"}); !slices.Equal(got, []string{"data_from_env_file", "telemetry_unencrypted"}) {
		t.Errorf("http elsewhere: %v", got)
	}
	// The user's own network: the stack's collector on its Docker network, or
	// a private address.
	for _, u := range []string{"http://otel-collector:4318", "http://10.0.0.5:4318", "http://192.168.1.20:4318"} {
		for _, w := range GuardWarnings(Destination{URL: u, From: "OTEL_EXPORTER_OTLP_ENDPOINT"}, data) {
			if w.Code == "telemetry_unencrypted" {
				t.Errorf("%s: %s, want no telemetry_unencrypted", u, w.Message)
			}
		}
	}
}

// An endpoint the repo's .env sets to another machine gets a hint to check
// it; one from the environment, or on this machine, doesn't.
func TestStatusHintsOnAnEndpointFromTheFile(t *testing.T) {
	none := caller.Resolution{}
	clearEnv(t)
	loadRepoEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT=https://otel.example.com\n")
	dest := Current()
	if h := strings.Join(dest.StatusHints(none), ""); !strings.Contains(h, "otel.example.com") || !strings.Contains(h, "sets OTEL_EXPORTER_OTLP_ENDPOINT on line 1") {
		t.Errorf("from .env, remote: hint = %q", h)
	}
	clearEnv(t)
	loadRepoEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318\n")
	if h := Current().StatusHints(none); len(h) > 0 {
		t.Errorf("from .env, loopback: hint = %q, want none", h)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://other.example.com")
	if h := Current().StatusHints(none); len(h) > 0 {
		t.Errorf("from the environment: hint = %q, want none", h)
	}
}

// A key in the endpoint's URL never reaches what a person or an agent reads.
func TestRedactedHidesAKeyInTheURL(t *testing.T) {
	got := Destination{URL: "https://user:secret@otel.example.com/v1?api_key=secret2"}.Redacted() //nolint:gosec // G101: a made-up key, to see it hidden
	if strings.Contains(got, "secret") || !strings.Contains(got, "otel.example.com") {
		t.Errorf("Redacted = %q", got)
	}
}

// A host given as a number is an address, not a local name.
func TestANumberIsNotALocalName(t *testing.T) {
	for _, h := range []string{"3627732462", "0xd83ad5ae"} {
		if isLocalNetwork(h) {
			t.Errorf("isLocalNetwork(%q) = true", h)
		}
	}
	if !isLocalNetwork("otel-collector") {
		t.Error("isLocalNetwork(otel-collector) = false")
	}
}

// Data on, with traces leaving the machine, gets a hint naming what set it,
// from the environment as from the file; none for loopback, or with data off.
func TestStatusHintsOnData(t *testing.T) {
	clearEnv(t)
	t.Setenv("ANFRA_EXPERIMENTAL_TRACE_DATA", "1") // the environment's
	data, err := caller.Resolve(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	remote := Destination{URL: "https://otel.example.com", From: "OTEL_EXPORTER_OTLP_ENDPOINT"}
	if h := strings.Join(remote.StatusHints(data), ""); !strings.Contains(h, "ANFRA_EXPERIMENTAL_TRACE_DATA") || !strings.Contains(h, "otel.example.com") {
		t.Errorf("on, remote: hints = %q", h)
	}
	if h := (Destination{URL: "http://localhost:4318", From: "OTEL_EXPORTER_OTLP_ENDPOINT"}).StatusHints(data); len(h) > 0 {
		t.Errorf("on, loopback: hints = %q, want none", h)
	}
	t.Setenv("ANFRA_EXPERIMENTAL_TRACE_DATA", "0")
	if data, err = caller.Resolve(os.Getenv); err != nil {
		t.Fatal(err)
	}
	if h := remote.StatusHints(data); len(h) > 0 {
		t.Errorf("nothing on: hints = %q, want none", h)
	}
}
