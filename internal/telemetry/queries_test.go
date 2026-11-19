package telemetry

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// observe runs one query's calls on Queries, inside a span, and answers the
// span.
func observe(t *testing.T, ctx context.Context, q command.CompiledQuery) sdktrace.ReadOnlySpan {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, span := tp.Tracer("test").Start(ctx, "op")
	var obs Queries
	obs.QueryCompiled(ctx, q)
	obs.QueryOnWarehouse(ctx, q.ID, "wh-9")
	scanned := int64(2048)
	obs.QueryFinished(ctx, q.ID, command.FinishedQuery{Rows: 42, BytesScanned: &scanned, Err: apperr.New(errcode.QueryFailed, "relation \"secret\" does not exist"),
		Timings: command.Timings{Steps: []command.TimedStep{{Step: "compiling_query"}, {Step: "executing_query", StartMs: 5}}, EndMs: 9}})
	span.End()
	return rec.Ended()[0]
}

func attrs(s sdktrace.ReadOnlySpan) map[string]string {
	m := map[string]string{}
	for _, kv := range s.Attributes() {
		m[string(kv.Key)] = kv.Value.String()
	}
	return m
}

// with is ctx with data on or off, and payloads cut at limit bytes.
func with(data bool, limit int) context.Context {
	return caller.WithSettings(context.Background(), caller.NewSettings(data, limit))
}

func TestAQueryIsWrittenOntoItsSpan(t *testing.T) {
	s := observe(t, with(true, 0), command.CompiledQuery{ID: "q-1", DataSource: "demo", DataSourceType: "postgresql",
		Dataset: "ecommerce", AQL: "ecommerce | select(x)", SQL: "SELECT 'secret'"})
	got := attrs(s)
	want := map[string]string{
		"anfra.query.id": "q-1", "anfra.query.kind": "aql", "anfra.dataset": "ecommerce", "anfra.data_source": "demo",
		"db.system.name": "postgresql", "anfra.warehouse.query_id": "wh-9",
		"anfra.query.bytes_scanned": "2048", "error.type": errcode.QueryFailed.String(),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if v, ok := got["db.response.returned_rows"]; ok {
		t.Errorf("a failed query recorded db.response.returned_rows = %s", v)
	}
	if got["db.query.text"] != "SELECT 'secret'" {
		t.Errorf("db.query.text = %q, want it as sent with data on", got["db.query.text"])
	}
	var steps []string
	for _, e := range s.Events() {
		steps = append(steps, e.Name)
	}
	if len(steps) != 2 || steps[0] != "compiling_query" || steps[1] != "executing_query" {
		t.Errorf("events = %v", steps)
	}
	if _, ok := got["exception.message"]; ok {
		t.Error("the observer recorded the error's message, which the op's span records")
	}
	if d := s.Events()[1].Time.Sub(s.Events()[0].Time); d.Milliseconds() != 5 {
		t.Errorf("executing_query started %v after compiling_query, want 5ms", d)
	}
}

func TestASQLQueryHasNoDataset(t *testing.T) {
	got := attrs(observe(t, context.Background(), command.CompiledQuery{ID: "q-2", DataSource: "demo", SQL: "SELECT 1"}))
	if got["anfra.query.kind"] != "sql" {
		t.Errorf("kind = %q", got["anfra.query.kind"])
	}
	if _, ok := got["anfra.dataset"]; ok {
		t.Error("a SQL query was given a dataset")
	}
}

// The names a query ran on are inputs: off by default.
func TestAQuerysNamesFollowInputs(t *testing.T) {
	got := attrs(observe(t, context.Background(), command.CompiledQuery{ID: "q-4", DataSource: "demo", Dataset: "ecommerce", SQL: "SELECT 1"}))
	for _, k := range []string{"anfra.dataset", "anfra.data_source"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s recorded with data off", k)
		}
	}
	if got["anfra.query.id"] != "q-4" {
		t.Errorf("anfra.query.id = %q, want it whatever data says", got["anfra.query.id"])
	}
}

// The SQL is data: recorded, cut at the limit, only when data is.
func TestTheSQLFollowsInputs(t *testing.T) {
	got := attrs(observe(t, with(true, 8), command.CompiledQuery{ID: "q-3", DataSource: "demo", SQL: "SELECT 1234567"}))
	if got["db.query.text"] != "SELECT 1…" || got["anfra.query.text_truncated"] != "true" {
		t.Errorf("data on: got %v", got)
	}
	got = attrs(observe(t, with(false, 0), command.CompiledQuery{ID: "q-5", DataSource: "demo", SQL: "SELECT 1"}))
	if _, ok := got["db.query.text"]; ok {
		t.Errorf("off: db.query.text = %q", got["db.query.text"])
	}
}

// A query that succeeded records the rows it read.
func TestASucceededQueryRecordsItsRows(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	Queries{}.QueryFinished(ctx, "q-6", command.FinishedQuery{Rows: 42})
	span.End()
	if got := attrs(rec.Ended()[0])["db.response.returned_rows"]; got != "42" {
		t.Errorf("db.response.returned_rows = %q, want 42", got)
	}
}
