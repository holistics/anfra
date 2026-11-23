package telemetry_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/baggage"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/shared/telemetry"
)

// exportedAttrs starts a span in ctx, with settings, as a host that records
// its caller does (Spans, then Filter), and returns what is exported of it.
func exportedAttrs(t *testing.T, ctx context.Context, settings telemetry.Settings) map[string]string {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(telemetry.Spans{}),
		sdktrace.WithSpanProcessor(telemetry.Filter(rec, settings)),
	)
	_, span := tp.Tracer("t").Start(telemetry.WithSettings(ctx, settings), "op")
	span.End()
	attrs := map[string]string{}
	for _, a := range rec.Ended()[0].Attributes() {
		attrs[string(a.Key)] = a.Value.String()
	}
	return attrs
}

// withBaggage is ctx with a caller's baggage of members, key=value each.
func withBaggage(t *testing.T, members ...string) context.Context {
	t.Helper()
	b, err := baggage.Parse(strings.Join(members, ","))
	if err != nil {
		t.Fatal(err)
	}
	return baggage.ContextWithBaggage(context.Background(), b)
}

// allowed is the members of a caller's baggage that settings let into anfra.
func allowed(t *testing.T, settings telemetry.Settings, members ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range telemetry.AllowedBaggage(baggage.FromContext(withBaggage(t, members...)), settings).Members() {
		out[m.Key()] = m.Value()
	}
	return out
}

// By default only the caller's session gets in.
func TestByDefaultOnlyTheSessionGetsIn(t *testing.T) {
	got := allowed(t, telemetry.Defaults(), "gen_ai.conversation.id=conv-1", "tenant.id=acme")
	if len(got) != 1 || got["gen_ai.conversation.id"] != "conv-1" {
		t.Errorf("got %v, want the session alone", got)
	}
}

// The members the settings include get in; never one of anfra's own keys, nor
// a long or odd value.
func TestTheIncludedBaggageGetsIn(t *testing.T) {
	settings := telemetry.Defaults().WithBaggageInclude([]string{"tenant.id", "user.*", "anfra.*", "error.type", "session.id", "note"})
	got := allowed(t, settings,
		"tenant.id=acme", "user.id=u-1", "user.role=admin", "other=x",
		"anfra.command.id=forged", "error.type=forged", "session.id=forged",
		"note="+strings.Repeat("x", 200))
	want := map[string]string{"tenant.id": "acme", "user.id": "u-1", "user.role": "admin"}
	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A span records the baggage that got in, as metadata, so even with data off,
// still never one of anfra's own keys. The session is session.id too.
func TestASpanRecordsTheBaggageThatGotIn(t *testing.T) {
	attrs := exportedAttrs(t, withBaggage(t, "gen_ai.conversation.id=conv-1", "tenant.id=acme", "anfra.command.id=forged"), telemetry.NewSettings(false, 0))
	if attrs["gen_ai.conversation.id"] != "conv-1" || attrs["session.id"] != "conv-1" || attrs["tenant.id"] != "acme" {
		t.Errorf("got %v", attrs)
	}
	if v, ok := attrs["anfra.command.id"]; ok {
		t.Errorf("anfra's own key recorded from baggage: %q", v)
	}
}

// The ids a host names its caller by go in its baggage too, for the calls it
// makes, as far as the settings let them in: by default the session alone, of
// anfra's own keys never another; with none, nothing.
func TestTheCallersIDsGoOnInItsBaggage(t *testing.T) {
	fields := map[string]string{"gen_ai.conversation.id": "s-1", "gen_ai.agent.name": "codex"}
	b := baggage.FromContext(telemetry.WithCallerBaggage(context.Background(), fields, telemetry.Defaults()))
	if b.Len() != 1 || b.Member("gen_ai.conversation.id").Value() != "s-1" {
		t.Errorf("default: baggage %q, want the session alone", b)
	}
	none := telemetry.Defaults().WithBaggageInclude([]string{})
	if b = baggage.FromContext(telemetry.WithCallerBaggage(context.Background(), fields, none)); b.Len() != 0 {
		t.Errorf("none: baggage %q", b)
	}
	if b = baggage.FromContext(telemetry.WithCallerBaggage(context.Background(), nil, telemetry.Defaults())); b.Len() != 0 {
		t.Errorf("no ids: baggage %q", b)
	}
	b = baggage.FromContext(telemetry.WithCallerBaggage(withBaggage(t, "gen_ai.conversation.id=conv-1"), map[string]string{"gen_ai.conversation.id": ""}, telemetry.Defaults()))
	if b.Member("gen_ai.conversation.id").Value() != "conv-1" {
		t.Errorf("an empty id replaced the caller's: baggage %q", b)
	}
}

// What a host names its caller by in code is recorded whatever its keys, and
// wins over the caller's baggage.
func TestAHostsOwnKeysAreRecorded(t *testing.T) {
	ctx := withBaggage(t, "gen_ai.conversation.id=from-baggage")
	ctx = telemetry.WithAttributes(ctx, map[string]string{"gen_ai.conversation.id": "from-host", "workspace.id": "w-1"})
	attrs := exportedAttrs(t, ctx, telemetry.Defaults())
	if attrs["gen_ai.conversation.id"] != "from-host" || attrs["workspace.id"] != "w-1" {
		t.Errorf("got %v", attrs)
	}
}

// The include setting takes keys and patterns, separated by commas, and
// refuses what isn't one.
func TestBaggageIncludeIsResolved(t *testing.T) {
	resolve := func(v string) (telemetry.Resolution, error) {
		return telemetry.Resolve(func(name string) string {
			if name == telemetry.BaggageIncludeVar {
				return v
			}
			return ""
		})
	}
	r, err := resolve("")
	if err != nil || !slices.Equal(r.Settings().BaggageInclude(), []string{"gen_ai.conversation.id"}) || r.BaggageInclude.From != "" {
		t.Errorf("unset: %v, %v", r.BaggageInclude, err)
	}
	r, err = resolve(" tenant.id, user.* ,")
	if err != nil || !slices.Equal(r.Settings().BaggageInclude(), []string{"tenant.id", "user.*"}) || r.BaggageInclude.From != telemetry.BaggageIncludeVar {
		t.Errorf("set: %v, %v", r.BaggageInclude, err)
	}
	r, err = resolve(" none ")
	if err != nil || len(r.Settings().BaggageInclude()) != 0 || r.BaggageInclude.From != telemetry.BaggageIncludeVar {
		t.Errorf("none: %v, %v", r.BaggageInclude, err)
	}
	if got := allowed(t, r.Settings(), "gen_ai.conversation.id=conv-1"); len(got) != 0 {
		t.Errorf("none let in %v", got)
	}
	for _, bad := range []string{",", "a[", "a b", "a/b", "none,gen_ai.conversation.id", strings.Repeat("k,", 17)} {
		var invalid *telemetry.InvalidSettingError
		if _, err := resolve(bad); !errors.As(err, &invalid) {
			t.Errorf("%q: %v, want it refused", bad, err)
		}
	}
}
