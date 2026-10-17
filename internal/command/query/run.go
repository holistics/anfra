package query

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
)

// MaxPageSize is the most rows a page holds: page_size's maximum, and the page
// a query without one is answered with when it is not streamed.
const MaxPageSize = 10_000

// QueryRun is how query answers: as a page (QueryResult), or as a stream of its
// events (QueryEvent).
type QueryRun = apikit.Run[QueryResult, QueryEvent]

// QueryEvent is one of query's own events, in a stream: compiled, then
// warehouse_query once the warehouse has the query, then header, then rows in
// batches, then summary. Which phase the query is in — compiling_query, then
// canal-query's connecting_to_dw, executing_query, fetching_query_result, and
// retrying_query — is the stream's status.
type QueryEvent interface{ queryEvent() }

// QueryCompiled is the SQL the query runs, as soon as it is known.
type QueryCompiled struct {
	Type string `json:"type" enum:"compiled"`
	SQL  string `json:"sql"`
	AQL  string `json:"aql,omitempty" doc:"the AQL that runs: the query with its Query Input applied; absent for a SQL query"`
}

// QueryWarehouse is the query's id on the warehouse, as soon as it has one: to
// find it there — in its query history, its billing — or to cancel it there.
type QueryWarehouse struct {
	Type string `json:"type" enum:"warehouse_query"`
	ID   string `json:"id" doc:"the warehouse's own id of the query; for Postgres, the process id of the backend running it"`
}

// QueryHeader is what each column of the result is, once the result's shape is
// known: before its rows.
type QueryHeader struct {
	Type    string   `json:"type" enum:"header"`
	Columns []Column `json:"columns" doc:"one per field, in order"`
	Fields  []string `json:"fields"`
}

// QueryRecords is a batch of the result's rows, as a page's records are.
type QueryRecords struct {
	Type    string           `json:"type" enum:"rows"`
	Records []canalquery.Row `json:"records" doc:"one array per row, its values in fields' order, written as a page's records are"`
}

// QuerySummary ends the result: how many rows it held, and, for a page,
// whether there are more.
type QuerySummary struct {
	Type     string `json:"type" enum:"summary"`
	RowCount int    `json:"row_count"`
	HasMore  bool   `json:"has_more" doc:"a page that may not be the last: there may be rows after it. Without page_size, every row was sent, and this is false"`
	// BytesScanned is what the warehouse read to run the query, as it reports
	// it: BigQuery and Athena.
	BytesScanned *int64  `json:"bytes_scanned,omitempty" doc:"the bytes the warehouse read to run the query, where it reports them (BigQuery, Athena)"`
	Timings      Timings `json:"timings" doc:"the query's steps: compiling_query, for AQL; canal-query's own (connecting_to_dw, executing_query, fetching_query_result, and their children, as <parent>.<child>); retrying_query, the wait before canal-query attempts the query again"`
}

func (QueryCompiled) queryEvent()  {}
func (QueryWarehouse) queryEvent() {}
func (QueryHeader) queryEvent()    {}
func (QueryRecords) queryEvent()   {}
func (QuerySummary) queryEvent()   {}

// queryEvents are query's events, for its op to declare.
var queryEvents = []any{QueryCompiled{}, QueryWarehouse{}, QueryHeader{}, QueryRecords{}, QuerySummary{}}

// A stream's rows are sent in batches of at most batchRows rows, or once a
// batch holds batchBytes: it holds one batch at a time, whatever it delivers.
const (
	batchRows  = 1000
	batchBytes = 1 << 20
)

// queryRun is a query the handler has accepted: what it resolved before any
// work, which runs when the run is read.
type queryRun struct {
	cc command.CommandContext
	in QueryRunInput
	// sql is a SQL query, on its data source; src and aql an AQL query's, its
	// limit: directive taken out into limit.
	sql   query.UngovernedSQL
	src   query.Source
	aql   string
	limit int
}

// newQueryRun accepts a query: what it refuses — a missing sidecar, an unknown
// data source, a malformed limit: — it refuses here, before any work, as an
// ordinary error rather than in a stream.
func newQueryRun(ctx context.Context, cc command.CommandContext, in QueryRunInput) (QueryRun, error) {
	q := queryRun{cc: cc, in: in, limit: query.NoLimit}
	if in.Lang == "sql" {
		sql, err := in.ungoverned(ctx, cc)
		if err != nil {
			return QueryRun{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{CanalQuery: true}); err != nil {
			return QueryRun{}, err
		}
		q.sql = sql
	} else {
		aql, limit, err := in.aql()
		if err != nil {
			return QueryRun{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true, CanalQuery: true}); err != nil {
			return QueryRun{}, err
		}
		src, err := sourceOf(cc)
		if err != nil {
			return QueryRun{}, err
		}
		q.src, q.aql, q.limit = src, aql, limit
	}
	return QueryRun{Page: q.page, Stream: q.stream}, nil
}

// sink receives a run's result as it is produced.
type sink struct {
	status    func(phase string) error
	compiled  func(sql, aql string) error
	warehouse func(id string) error
	scanned   func(bytes int64)
	header    func(columns []Column, fields []string) error
	row       func(canalquery.Row) error
}

// Timings and TimedStep are a run's timeline, command's (CommandContext's
// observers report them too), named here as the query's answer has always
// named them. A query's steps are documented on its answer's fields.
type (
	Timings   = command.Timings
	TimedStep = command.TimedStep
)

// tracker follows a run: the phase it is in, for the stream's status, and its
// timeline, for Timings. canal-query's times count from when it started
// answering; canalAt puts them on the run's timeline.
type tracker struct {
	s       sink
	start   time.Time
	canalAt int64
	current string
	steps   []TimedStep
}

func newTracker(s sink) *tracker { return &tracker{s: s, start: time.Now(), steps: []TimedStep{}} }

func (t *tracker) since() int64 { return time.Since(t.start).Milliseconds() }

// begin records a step starting at, and reports the phase it is in — its top
// step — unless the run is in it already.
func (t *tracker) begin(step string, at int64) error {
	t.steps = append(t.steps, TimedStep{Step: step, StartMs: at})
	phase, _, _ := strings.Cut(step, ".")
	if phase == t.current {
		return nil
	}
	t.current = phase
	return t.s.status(phase)
}

// requested marks the request to canal-query going: its times count from here.
func (t *tracker) requested() { t.canalAt = t.since() }

// progress is canal-query's progress: its steps, and a retry as retrying_query;
// its warehouse query id, and what it scanned.
func (t *tracker) progress(p canalquery.Progress) error {
	at := t.canalAt + p.AtMs
	switch p.Kind {
	case "step":
		return t.begin(p.Step, at)
	case "retry":
		return t.begin("retrying_query", at)
	case "dw_query_id":
		return t.s.warehouse(p.DwQueryID)
	case "dw_bytes_processed":
		if p.DwBytesProcessed != nil {
			t.s.scanned(*p.DwBytesProcessed)
		}
	}
	return nil
}

func (t *tracker) timings() Timings { return Timings{Steps: t.steps, EndMs: t.since()} }

// execute compiles the query, when it is AQL, and runs it, its result truncated
// at truncate rows (query.NoLimit: none) and failing past the host's maximum;
// and answers its timings. The invocation's query observer is told of it as it
// goes (observation).
func (q queryRun) execute(ctx context.Context, truncate int, s sink) (_ Timings, err error) {
	canal := q.cc.Clients.CanalQuery
	bounds := query.Bounded(truncate, q.cc.QueryLimits)
	t := newTracker(s)
	o := observe(q.cc, t)
	defer func() { o.finished(ctx, err) }()
	handler := func(columns []anfranode.ExploreColumn) canalquery.Handler {
		return o.handler(ctx, canalquery.Handler{
			Header: func(h canalquery.Header) error { return s.header(describeColumns(h, columns), h.Fields) },
			Row:    s.row,
		})
	}
	if q.in.Lang == "sql" {
		if err := s.compiled(q.in.Query, ""); err != nil {
			return Timings{}, err
		}
		o.compiled(ctx, q.in.DataSource, "", q.in.Query)
		t.requested()
		if err := q.sql.Stream(ctx, canal, bounds, handler(nil)); err != nil {
			return Timings{}, asQueryFailed(err)
		}
		return t.timings(), nil
	}
	if err := t.begin("compiling_query", 0); err != nil {
		return Timings{}, err
	}
	compiled, err := compileAQL(ctx, q.cc, q.src, q.in.Dataset, q.aql, q.in.run())
	if err != nil {
		return Timings{}, err
	}
	if err := s.compiled(compiled.SQL, compiled.AQL); err != nil {
		return Timings{}, err
	}
	o.compiled(ctx, compiled.DataSource.Name, q.in.Dataset, compiled.SQL)
	t.requested()
	if err := query.Stream(ctx, canal, q.src.DataSources, compiled, bounds, handler(compiled.Columns)); err != nil {
		return Timings{}, asQueryFailed(err)
	}
	return t.timings(), nil
}

// observation is a query as the invocation's observer is told of it: once
// compiled, its warehouse id when the warehouse gives it, and its end — however
// it ends, but only after it was compiled.
type observation struct {
	obs     command.QueryObserver
	id      command.QueryID
	t       *tracker
	sent    bool
	rows    int
	scanned *int64
}

func observe(cc command.CommandContext, t *tracker) *observation {
	return &observation{obs: cc.QueryObserver(), id: command.NewQueryID(), t: t}
}

// compiled tells the observer the query is about to be sent.
func (o *observation) compiled(ctx context.Context, dataSource, dataset, sql string) {
	o.obs.QueryCompiled(ctx, command.CompiledQuery{ID: o.id, DataSource: dataSource, Dataset: dataset, SQL: sql})
	o.sent = true
}

// handler is h with canal-query's progress followed: its steps timed by the
// observation's tracker (which reports the run's phases to its sink), and the
// warehouse's id and bytes scanned told to the observer; and the rows read
// counted.
func (o *observation) handler(ctx context.Context, h canalquery.Handler) canalquery.Handler {
	row := h.Row
	h.Progress = func(p canalquery.Progress) error {
		switch {
		case p.Kind == "dw_query_id":
			o.obs.QueryOnWarehouse(ctx, o.id, p.DwQueryID)
		case p.Kind == "dw_bytes_processed" && p.DwBytesProcessed != nil:
			n := *p.DwBytesProcessed
			o.scanned = &n
		}
		return o.t.progress(p)
	}
	h.Row = func(r canalquery.Row) error {
		o.rows++
		return row(r)
	}
	return h
}

// finished tells the observer how the query ended, when it was told of it.
func (o *observation) finished(ctx context.Context, err error) {
	if o.sent {
		o.obs.QueryFinished(ctx, o.id, command.FinishedQuery{Rows: o.rows, BytesScanned: o.scanned, Timings: o.t.timings(), Err: err})
	}
}

// asQueryFailed is err as query_failed, with canal's message, when it is
// canal-query failing to run the query — the data source's failure. A
// client-scope error from canal is a request the engine built wrong — the
// engine's bug, not the data source's — and stays unclassified, as does
// anything else; an error with a code already, such as result_too_large,
// passes as it is.
func asQueryFailed(err error) error {
	if _, formal := err.(*apperr.Error); formal {
		return err
	}
	e, ok := errors.AsType[*canalquery.Error](err)
	if !ok || e.Scope == "Client" {
		return err
	}
	return apperr.Encapsulate(err, errcode.QueryFailed, e.Message)
}

// page is the query answered whole: one page, at most MaxPageSize rows and the
// host's page bytes. With page_size, the page is compiled into the SQL, and a
// full page may have more after it. Without, it is the first MaxPageSize rows,
// and one more row asked for says whether there are more.
func (q queryRun) page(ctx context.Context) (QueryResult, error) {
	keep, truncate := q.in.PageSize, query.NoLimit
	if keep == 0 {
		keep, truncate = MaxPageSize, MaxPageSize+1
		if q.limit != query.NoLimit && q.limit <= MaxPageSize {
			keep, truncate = q.limit, q.limit
		}
	}
	maxBytes := q.cc.QueryLimits.PageBytes()
	r := QueryResult{Result: QueryRows{Records: []canalquery.Row{}}}
	size := 0
	timings, err := q.execute(ctx, truncate, sink{
		status:    func(string) error { return nil },
		warehouse: func(string) error { return nil },
		scanned:   func(int64) {},
		compiled: func(sql, aql string) error {
			r.SQL, r.AQL = sql, aql
			return nil
		},
		header: func(columns []Column, fields []string) error {
			r.Columns, r.Result.Fields = columns, fields
			return nil
		},
		row: func(row canalquery.Row) error {
			if len(r.Result.Records) == keep {
				r.HasMore = true // the row past the page
				return nil
			}
			if size += len(row); size > maxBytes {
				return query.NewPageTooLarge(maxBytes)
			}
			r.Result.Records = append(r.Result.Records, row)
			return nil
		},
	})
	if err != nil {
		return QueryResult{}, err
	}
	if q.in.PageSize > 0 {
		r.HasMore = len(r.Result.Records) == q.in.PageSize
	}
	r.Timings = timings
	return r, nil
}

// stream is the query's events: compiled, header, then its rows in batches,
// then summary. With page_size, the rows are that page; without, every row, up
// to the host's maximum (query.Limits).
func (q queryRun) stream(ctx context.Context, events apikit.Events[QueryEvent]) error {
	batch, size, count := []canalquery.Row{}, 0, 0
	var scanned *int64
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := events.Emit(QueryRecords{Type: "rows", Records: batch})
		batch, size = []canalquery.Row{}, 0
		return err
	}
	timings, err := q.execute(ctx, q.limit, sink{
		status: events.Status,
		compiled: func(sql, aql string) error {
			return events.Emit(QueryCompiled{Type: "compiled", SQL: sql, AQL: aql})
		},
		warehouse: func(id string) error {
			return events.Emit(QueryWarehouse{Type: "warehouse_query", ID: id})
		},
		scanned: func(bytes int64) { scanned = &bytes },
		header: func(columns []Column, fields []string) error {
			return events.Emit(QueryHeader{Type: "header", Columns: columns, Fields: fields})
		},
		row: func(row canalquery.Row) error {
			batch, size, count = append(batch, row), size+len(row), count+1
			if len(batch) >= batchRows || size >= batchBytes {
				return flush()
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	return events.Emit(QuerySummary{Type: "summary", RowCount: count, HasMore: q.in.PageSize > 0 && count == q.in.PageSize,
		BytesScanned: scanned, Timings: timings})
}
