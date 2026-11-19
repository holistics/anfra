package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/apptracing"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// Queries is the query observer the CLI and `anfra serve` set: it writes what
// it is told of each query onto the span current when it is told, its error's
// code, and the query's steps as span events (docs/designs/telemetry.md). Its names and SQL
// go on as the call's data settings say; its rows never do.
type Queries struct{}

var _ command.QueryObserver = Queries{}

// QueryCompiled names the query, and with data on its data source, dataset
// and SQL.
func (Queries) QueryCompiled(ctx context.Context, q command.CompiledQuery) {
	kind := "sql"
	if q.Dataset != "" || q.AQL != "" {
		kind = "aql"
	}
	var dataset any
	if q.Dataset != "" {
		dataset = q.Dataset
	}
	apptracing.SetAttributes(ctx,
		caller.QueryID.Is(string(q.ID)),
		caller.QueryKind.Is(kind),
		caller.Dataset.Is(dataset),
		caller.DataSource.Is(q.DataSource),
		caller.DBSystem.Is(q.DataSourceType),
	)
	if !caller.SettingsOf(ctx).RecordsData() {
		return
	}
	sql, wasCut := apptracing.CutPayload(ctx, q.SQL)
	apptracing.SetAttributes(ctx, caller.QueryText.Is(sql))
	if wasCut {
		apptracing.SetAttributes(ctx, caller.QueryTextTruncated.Is(true))
	}
}

// QueryOnWarehouse names the warehouse's own id for the query.
func (Queries) QueryOnWarehouse(ctx context.Context, _ command.QueryID, warehouseQueryID string) {
	apptracing.SetAttributes(ctx, caller.WarehouseQueryID.Is(warehouseQueryID))
}

// QueryFinished records the rows read when the query succeeded, the bytes
// scanned, the error's code when it failed, and each step of the run, at the
// time it started. Only the code: the op's span records the error itself, as
// it ends (apptracing.End), so a query's failure is recorded once, as its op's
// errors setting says. A failed query read no rows to count, so it records
// none rather than 0.
func (Queries) QueryFinished(ctx context.Context, _ command.QueryID, f command.FinishedQuery) {
	var code, rows any
	if f.Err != nil {
		code = apperr.From(f.Err).Code.String()
	} else {
		rows = f.Rows
	}
	apptracing.SetAttributes(ctx,
		caller.ReturnedRows.Is(rows),
		caller.BytesScanned.Is(f.BytesScanned),
		caller.ErrorType.Is(code),
	)
	if !trace.SpanFromContext(ctx).IsRecording() {
		return
	}
	start := time.Now().Add(-time.Duration(f.Timings.EndMs) * time.Millisecond)
	for _, s := range f.Timings.Steps {
		apptracing.AddEvent(ctx, s.Step, start.Add(time.Duration(s.StartMs)*time.Millisecond))
	}
}
