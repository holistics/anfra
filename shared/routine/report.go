package routine

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Outcomes of a run, the outcome attribute of anfra.routine.runs.
const (
	outcomeOK    = "ok"
	outcomeError = "error"
	outcomePanic = "panic"
)

// The instruments report to the global meter provider, including one installed
// after this package is initialised; without one, they are no-ops. The routine
// attribute is a routine's name: a fixed set, adding no cardinality as an
// installation grows.
var (
	meter = otel.Meter("github.com/holistics/anfra/shared/routine")

	runs = must(meter.Int64Counter("anfra.routine.runs",
		metric.WithDescription("Runs of a routine, by outcome: ok, error or panic.")))
	duration = must(meter.Float64Histogram("anfra.routine.duration", metric.WithUnit("s"),
		metric.WithDescription("How long a run of a periodic routine or a task took.")))
	lastSuccess = must(meter.Int64Gauge("anfra.routine.last_success", metric.WithUnit("s"),
		metric.WithDescription("When a routine's run last succeeded, in Unix seconds.")))
	restarts = must(meter.Int64Counter("anfra.routine.restarts",
		metric.WithDescription("Restarts of a loop after it returned early.")))
	active = must(meter.Int64UpDownCounter("anfra.routine.active",
		metric.WithDescription("Loops, workers and tasks running.")))
)

// must is i: the API returns a usable instrument even with an error, which
// only an invalid name gives, and these names are fixed.
func must[I any](i I, _ error) I { return i }

func nameAttr(name string) attribute.KeyValue { return attribute.String("routine", name) }

func recordRun(ctx context.Context, name, outcome string, took time.Duration) {
	countRun(ctx, name, outcome)
	duration.Record(ctx, took.Seconds(), metric.WithAttributes(nameAttr(name)))
	if outcome == outcomeOK {
		lastSuccess.Record(ctx, time.Now().Unix(), metric.WithAttributes(nameAttr(name)))
	}
}

func countRun(ctx context.Context, name, outcome string) {
	runs.Add(ctx, 1, metric.WithAttributes(nameAttr(name), attribute.String("outcome", outcome)))
}

func countRestart(ctx context.Context, name string) {
	restarts.Add(ctx, 1, metric.WithAttributes(nameAttr(name)))
}

func addActive(ctx context.Context, name string, n int64) {
	active.Add(ctx, n, metric.WithAttributes(nameAttr(name)))
}
