package command

import (
	"context"

	"github.com/google/uuid"
)

// Timings are a run's timeline: when each of its steps started, in order, in
// milliseconds since the run started, and when it ended. The steps are named by
// what ran (a query's are its answer's to document); a step run again is there
// again. They are what is known — starts — not what is derived from them: a step
// ends where the next step at its level or above starts (a child step,
// "<parent>.<child>", is within its parent), or at EndMs.
type Timings struct {
	Steps []TimedStep `json:"steps" doc:"each step as it started, in order; a step run again is here again"`
	EndMs int64       `json:"end_ms" doc:"when the run ended, in ms since it started: its total time, and where its last steps end"`
}

// TimedStep is when a step of the run started.
type TimedStep struct {
	Step    string `json:"step" doc:"the step's name, as what ran names it; a child step as <parent>.<child>, within its parent"`
	StartMs int64  `json:"start_ms" doc:"when it started, in ms since the run started"`
}

// Observers are who the engine tells of what a command does, as it does it:
// one per capability, each nil when nobody listens. The host sets them on its
// Invocation; a command reaches them through its CommandContext.
type Observers struct {
	// Queries is told of each query a command runs on a warehouse.
	Queries QueryObserver
}

// QueryObserver is told of each query a command runs on a warehouse, as it
// runs: for a host that records them. It is called from the run's goroutine,
// with the run's ctx, and must not hold it up: a host that writes what it is
// told does so elsewhere.
type QueryObserver interface {
	// QueryCompiled: the query is about to be sent to its data source.
	QueryCompiled(ctx context.Context, q CompiledQuery)
	// QueryOnWarehouse: the warehouse has given the query its own id.
	QueryOnWarehouse(ctx context.Context, id QueryID, warehouseQueryID string)
	// QueryFinished: the query ended, succeeded or not — once, after
	// QueryCompiled.
	QueryFinished(ctx context.Context, id QueryID, f FinishedQuery)
}

// QueryID is a query run's id, minted by the engine as the query starts: a
// UUIDv7, whose time is when the query started. It matches a query's calls to
// its observer — a command may run several — and a host may key its record of
// the query by it, so that one id names the query everywhere.
type QueryID string

// CompiledQuery is a query as it is sent to its data source.
type CompiledQuery struct {
	ID         QueryID
	DataSource string // its name
	Dataset    string // the AQL dataset it was compiled from; "" for SQL against a data source
	// AQL is the AQL that runs: the query with its Query Input applied, as the
	// compiled event carries it; "" for SQL.
	AQL string
	SQL string // as sent
}

// FinishedQuery is how a query ended.
type FinishedQuery struct {
	Rows         int    // read from the warehouse
	BytesScanned *int64 // where the warehouse reports it
	// Timings are its run's: compiling, when it was AQL, then canal-query's
	// steps, and its end — failed or not.
	Timings Timings
	Err     error // nil when it succeeded; as its caller is answered otherwise
}

// QueryObserver is the invocation's query observer, or one that hears nothing:
// a command calls it without asking whether anyone listens.
func (cc CommandContext) QueryObserver() QueryObserver {
	if cc.Observers.Queries == nil {
		return noQueryObserver{}
	}
	return cc.Observers.Queries
}

// NewQueryID mints a query run's id: a UUIDv7, as of now.
func NewQueryID() QueryID {
	return QueryID(uuid.Must(uuid.NewV7()).String())
}

type noQueryObserver struct{}

func (noQueryObserver) QueryCompiled(context.Context, CompiledQuery)          {}
func (noQueryObserver) QueryOnWarehouse(context.Context, QueryID, string)     {}
func (noQueryObserver) QueryFinished(context.Context, QueryID, FinishedQuery) {}
