package main

import (
	"bytes"
	"context"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/telemetry"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// telemetryInfo says when OTEL_SDK_DISABLED turned telemetry off, which would
// otherwise look the same as no endpoint at all.
func TestTelemetryInfoNamesWhatTurnedItOff(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	setup := func() {
		t.Helper()
		if _, err := telemetry.Setup(context.Background(), "anfra-test", "0.0.0", caller.Defaults()); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { t.Setenv("OTEL_SDK_DISABLED", ""); setup() })

	t.Setenv("OTEL_SDK_DISABLED", "")
	setup()
	if got := telemetryInfo(); got.State != command.TelemetryOff || got.SetBy != "" {
		t.Errorf("with no endpoint, telemetry = %+v, want off with no set_by", got)
	}

	t.Setenv("OTEL_SDK_DISABLED", "true")
	setup()
	if got := telemetryInfo(); got.State != command.TelemetryOff || got.SetBy != "OTEL_SDK_DISABLED" {
		t.Errorf("with OTEL_SDK_DISABLED=true, telemetry = %+v, want off, set by OTEL_SDK_DISABLED", got)
	}
}

// Every command is the root of its trace, a core command or a CLI-only one,
// with the op's span under it, and a failure the caller caused marked with its
// code but not as an error.
func TestEveryCommandIsTheRootOfItsTrace(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Setenv("ANFRA_NO_UPDATE_NOTIFIER", "1")
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	runArgs := func(args ...string) []sdktrace.ReadOnlySpan {
		t.Helper()
		t.Cleanup(resetOutput)
		before := len(rec.Ended())
		run(t.Context(), args, &bytes.Buffer{}, &bytes.Buffer{}, runOptions{})
		return rec.Ended()[before:]
	}
	roots := func(spans []sdktrace.ReadOnlySpan) []sdktrace.ReadOnlySpan {
		var r []sdktrace.ReadOnlySpan
		for _, s := range spans {
			if !s.Parent().IsValid() {
				r = append(r, s)
			}
		}
		return r
	}

	// A core command: one root, and runCommand starts no second one.
	spans := runArgs("version", "--yaml")
	r := roots(spans)
	if len(r) != 1 || r[0].Name() != "anfra version" {
		t.Fatalf("version's roots = %v, want one, anfra version", names(r))
	}
	if n := len(slices.DeleteFunc(names(spans), func(n string) bool { return n != "anfra version" })); n != 1 {
		t.Errorf("version's spans = %v, want one anfra version", names(spans))
	}

	// A CLI-only command, which is no op, has its root too.
	r = roots(runArgs("experimental", "telemetry", "init", "--yaml"))
	if len(r) != 1 || r[0].Name() != "anfra experimental telemetry init" {
		t.Fatalf("telemetry init's roots = %v, want one, anfra experimental telemetry init", names(r))
	}

	// The caller's mistake: as any exit but 0, on a CLI span (OpenTelemetry's
	// CLI spans), its code and an error status.
	r = roots(runArgs("experimental", "telemetry", "init", "extra"))
	if len(r) != 1 {
		t.Fatalf("a refused telemetry init's roots = %v, want one", names(r))
	}
	attrs := map[string]string{}
	for _, a := range r[0].Attributes() {
		attrs[string(a.Key)] = a.Value.String()
	}
	if attrs["error.type"] == "" || attrs["process.exit.code"] != "1" || r[0].Status().Code != otelcodes.Error {
		t.Errorf("refused: error.type %q, exit code %q, status %v; want a code, 1 and an error status",
			attrs["error.type"], attrs["process.exit.code"], r[0].Status())
	}
}

// Every span of a command in a repo names the repo by its repo id, the name of
// its state folder, with data on or off, never by its path, from the root on,
// a CLI-only command's included; a command outside a repo names none.
func TestTheRepoIsOnEverySpan(t *testing.T) {
	t.Setenv("ANFRA_NO_UPDATE_NOTIFIER", "1")
	withTraceData(t, map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": "1"})
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(caller.Spans{}), sdktrace.WithSpanProcessor(caller.Filter(rec, caller.Defaults()))))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	rootAttrs := func(args ...string) map[string]string {
		t.Helper()
		t.Cleanup(resetOutput)
		before := len(rec.Ended())
		run(t.Context(), args, &bytes.Buffer{}, &bytes.Buffer{}, runOptions{})
		attrs := map[string]string{}
		for _, s := range rec.Ended()[before:] {
			if !s.Parent().IsValid() {
				for _, a := range s.Attributes() {
					attrs[string(a.Key)] = a.Value.String()
				}
			}
		}
		return attrs
	}

	r := inRepo(t)
	attrs := rootAttrs("experimental", "telemetry", "init", "--yaml")
	if attrs["anfra.repo.id"] != r.ID || filepath.Base(r.DataDir) != r.ID || attrs["anfra.repo.name"] != "" {
		t.Errorf("in a repo, the root's attributes = %v, want the repo id %s, its state folder's name", attrs, r.ID)
	}
	for k, v := range attrs {
		if strings.Contains(v, r.Dir) {
			t.Errorf("%s records the repo's path: %q", k, v)
		}
	}

	withTraceData(t, map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": "0"})
	if attrs := rootAttrs("experimental", "telemetry", "init", "--yaml"); attrs["anfra.repo.id"] != r.ID {
		t.Errorf("with data off, the root's attributes = %v, want the repo id all the same", attrs)
	}

	t.Chdir(t.TempDir())
	if attrs := rootAttrs("experimental", "telemetry", "init", "--yaml"); attrs["anfra.repo.id"] != "" {
		t.Errorf("outside a repo, the root's attributes = %v, want no repo", attrs)
	}
}

// A command's ops name the repo too: the CommandContext's Attribution, which
// serve's requests carry as the CLI's do.
func TestCommandContextNamesTheRepo(t *testing.T) {
	r := inRepo(t)
	got := hostContext{repo: r}.commandContext(command.Clients{}).Attribution
	if got["anfra.repo.id"] != r.ID || len(got) != 1 {
		t.Errorf("Attribution = %v", got)
	}
	var buf bytes.Buffer
	lg := slog.New(caller.LogHandler(slog.NewJSONHandler(&buf, nil)))
	lg.InfoContext(caller.WithAttributes(t.Context(), got), "Hello")
	if want := `"anfra_repo_id":"` + filepath.Base(r.DataDir) + `"`; !strings.Contains(buf.String(), want) || strings.Contains(buf.String(), "anfra_repo_name") {
		t.Errorf("log record = %s, want %s, the state folder's name, and no name", buf.String(), want)
	}
	if got := repoAttribution(repo.Resolve(t.TempDir())); got != nil {
		t.Errorf("outside a repo: %v, want none", got)
	}
}

func names(spans []sdktrace.ReadOnlySpan) []string {
	var n []string
	for _, s := range spans {
		n = append(n, s.Name())
	}
	return n
}

// withTraceData resolves the data settings from env alone, for the rest of the
// test.
func withTraceData(t *testing.T, env map[string]string) {
	t.Helper()
	prev := traceData
	t.Cleanup(func() { traceData = prev })
	r, err := caller.Resolve(func(name string) string { return env[name] })
	if err != nil {
		t.Fatal(err)
	}
	traceData = r
}

// A command's arguments are on its root span with data on, a refused run's
// included; with data off, they aren't.
func TestTheRootRecordsTheArgumentsWithDataOn(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Setenv("ANFRA_NO_UPDATE_NOTIFIER", "1")
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(caller.Filter(rec, caller.Defaults()))))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	input := func(args ...string) string {
		t.Helper()
		t.Cleanup(resetOutput)
		before := len(rec.Ended())
		run(t.Context(), args, &bytes.Buffer{}, &bytes.Buffer{}, runOptions{})
		for _, s := range rec.Ended()[before:] {
			if s.Parent().IsValid() {
				continue
			}
			for _, e := range s.Events() {
				if e.Name == caller.CommandInput.String() {
					for _, a := range e.Attributes {
						if string(a.Key) == caller.Payload.String() {
							return a.Value.AsString()
						}
					}
				}
			}
		}
		return ""
	}

	withTraceData(t, map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": "0"})
	if got := input("version", "--yaml"); got != "" {
		t.Errorf("with data off, anfra.command.input = %s", got)
	}
	withTraceData(t, map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": "1"})
	if got := input("version", "--yaml"); got != `["version","--yaml"]` {
		t.Errorf("anfra.command.input = %s", got)
	}
	if got := input("experimental", "telemetry", "init", "--no-such-flag"); got != `["experimental","telemetry","init","--no-such-flag"]` {
		t.Errorf("a refused run's anfra.command.input = %s", got)
	}
}

// Every attribute and event the CLI's spans carry is declared in the registry
// (shared/telemetry), with everything recorded, so a new key, anfra's or a
// library's, fails here until someone classifies it.
func TestEveryRecordedKeyIsDeclared(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Setenv("ANFRA_NO_UPDATE_NOTIFIER", "1")
	withTraceData(t, map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": "1"})
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(caller.Spans{}), sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	inRepo(t)
	for _, args := range [][]string{
		{"version", "--yaml"},
		{"status", "--yaml"},
		{"experimental", "telemetry", "init", "--yaml"},
		{"experimental", "telemetry", "init", "extra"},
		{"no-such-command"},
	} {
		run(t.Context(), args, &bytes.Buffer{}, &bytes.Buffer{}, runOptions{})
		resetOutput()
	}
	undeclared := map[string]bool{}
	for _, s := range rec.Ended() {
		for _, a := range s.Attributes() {
			if !caller.Declared(string(a.Key)) {
				undeclared[string(a.Key)] = true
			}
		}
		for _, e := range s.Events() {
			if !caller.Declared(e.Name) {
				undeclared["event "+e.Name] = true
			}
			for _, a := range e.Attributes {
				if !caller.Declared(string(a.Key)) {
					undeclared[string(a.Key)] = true
				}
			}
		}
	}
	if len(undeclared) > 0 {
		t.Errorf("keys recorded but not declared in shared/telemetry: %v", slices.Sorted(maps.Keys(undeclared)))
	}
	if len(rec.Ended()) == 0 {
		t.Fatal("no spans recorded")
	}
}

// A command that reads stdin says so on its span, never what stdin held.
func TestStdinIsNotedNotRecorded(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = prev })
	_, _ = w.WriteString("secret-query")
	_ = w.Close()

	ctx, span := tp.Tracer("test").Start(t.Context(), "anfra query")
	values := map[string]any{}
	if err := applyStdin(ctx, []command.Arg{{Name: "query", Stdin: true}}, values); err != nil {
		t.Fatal(err)
	}
	span.End()
	if values["query"] != "secret-query" {
		t.Fatalf("query = %v", values["query"])
	}
	for _, a := range rec.Ended()[0].Attributes() {
		if strings.Contains(a.Value.String(), "secret-query") {
			t.Errorf("%s records stdin: %s", a.Key, a.Value.String())
		}
	}
	if got := rec.Ended()[0].Attributes(); len(got) != 1 || string(got[0].Key) != caller.CommandStdin.String() {
		t.Errorf("attributes = %v, want anfra.command.stdin alone", got)
	}
}

// A command run by a coding agent names the agent's session; one run outside
// any agent names none.
func TestACommandNamesItsAgentsSession(t *testing.T) {
	got := sessionAttribution(func(k string) string {
		return map[string]string{"CLAUDE_CODE_SESSION_ID": "s-1"}[k]
	})
	if got["gen_ai.conversation.id"] != "s-1" || got["gen_ai.agent.name"] != "claude-code" {
		t.Errorf("got %v", got)
	}
	if got := sessionAttribution(func(string) string { return "" }); got != nil {
		t.Errorf("outside an agent: got %v", got)
	}
	got = sessionAttribution(func(k string) string {
		return map[string]string{"ANFRA_SESSION_ID": "run-1", "CLAUDE_CODE_SESSION_ID": "s-1"}[k]
	})
	if got["gen_ai.conversation.id"] != "s-1" || got["gen_ai.agent.name"] != "claude-code" {
		t.Errorf("set by the launcher, under a known agent: got %v, want the agent's", got)
	}
	got = sessionAttribution(func(k string) string { return map[string]string{"ANFRA_SESSION_ID": "run-1"}[k] })
	if _, named := got["gen_ai.agent.name"]; got["gen_ai.conversation.id"] != "run-1" || named {
		t.Errorf("set by the launcher, no known agent: got %v", got)
	}
}
