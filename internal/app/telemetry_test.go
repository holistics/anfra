package app

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/shared/telemetry"
)

// The invocation's Attribution is on its op's span, and on a span the command
// starts itself, named by the op it is the step of.
func TestAttributionIsOnEverySpanOfTheOp(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(telemetry.Spans{}), sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev); _ = tp.Shutdown(context.Background()) })

	orig := Commands
	t.Cleanup(func() { Commands = orig })
	Commands = append(append([]command.Command{}, orig...), command.Define(command.Def[command.NoInput, string]{
		Name:  "telemetry_probe",
		Short: "A probe",
		Run: func(ctx context.Context, _ command.CommandContext, _ command.NoInput) (string, error) {
			_, s := otel.Tracer("probe").Start(ctx, "inside")
			s.End()
			return "ok", nil
		},
	}))

	cc := command.CommandContext{DataPerms: dataperm.Ungoverned(), Attribution: attribution.Fields{"gen_ai.conversation.id": "s-1"}}
	if _, err := Dispatch(context.Background(), cc, Request{Command: "telemetry_probe"}); err != nil {
		t.Fatal(err)
	}

	spans := rec.Ended()
	if len(spans) != 2 {
		t.Fatalf("got %d spans", len(spans))
	}
	for _, s := range spans {
		got := map[string]string{}
		for _, kv := range s.Attributes() {
			got[string(kv.Key)] = kv.Value.String()
		}
		if got["gen_ai.conversation.id"] != "s-1" {
			t.Errorf("%s: got %v", s.Name(), got)
		}
		if s.Name() != "inside" && got["anfra.op"] != "core.telemetry_probe" {
			t.Errorf("%s: anfra.op = %q", s.Name(), got["anfra.op"])
		}
	}
}
