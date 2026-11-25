package query

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/apptracing"
	caller "github.com/holistics/anfra/shared/telemetry"
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
	Type  string      `json:"type" enum:"compiled"`
	SQL   string      `json:"sql"`
	AQL   string      `json:"aql,omitempty" doc:"the final AQL (with the Query Input applied). Absent for a SQL query."`
	Input *QueryInput `json:"input,omitempty" doc:"the query's input as it runs, every default filled in, its seeds included. Send it back with the same query to get the same answer again. Absent for a SQL query."`
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
	Type     string     `json:"type" enum:"summary"`
	RowCount int        `json:"row_count"`
	HasMore  bool       `json:"has_more" doc:"whether there are rows after this page; for a pivot, rows of its grid. Without input.pagination, every row was sent, and this is false."`
	Pivot    *PivotPage `json:"pivot,omitempty" doc:"for a page of a pivot: its grid's size, and whether there are columns after this page's. Absent for any other query."`
	// BytesScanned is what the warehouse read to run the query, as it reports
	// it: BigQuery and Athena.
	BytesScanned *int64      `json:"bytes_scanned,omitempty" doc:"the bytes the warehouse read to run the query, where it reports them (BigQuery, Athena)"`
	Cache        CacheResult `json:"cache,omitzero" doc:"what the query cache did for this query. Absent when the query doesn't use the cache: it sets no cache.duration and the repo no default, or 0s."`
	Timings      Timings     `json:"timings" doc:"the query's steps, and when each started. The steps are compiling_query (for AQL only), connecting_to_dw, executing_query, fetching_query_result, and retrying_query, which is a wait before the query is tried again. connecting_to_dw, executing_query, and fetching_query_result may have child steps."`
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
	in QueryRequestParams
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
func newQueryRun(ctx context.Context, cc command.CommandContext, in QueryRequestParams) (QueryRun, error) {
	named(ctx, in)
	in, err := in.withRepoDefaults(cc)
	if err != nil {
		return QueryRun{}, err
	}
	r := queryRun{cc: cc, in: in, limit: query.NoLimit}
	if in.Lang == "sql" {
		sql, err := in.ungoverned(ctx, cc)
		if err != nil {
			return QueryRun{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{CanalQuery: true}); err != nil {
			return QueryRun{}, err
		}
		r.sql = sql
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
		r.src, r.aql, r.limit = src, aql, limit
	}
	return QueryRun{Page: r.page, Stream: r.stream}, nil
}

// named records the query's kind, and its dataset as the data settings allow,
// on the span as soon as the query is read, so a query that fails before it
// compiles still says what it asked of which dataset. The query observer
// records them again once it compiles.
func named(ctx context.Context, in QueryRequestParams) {
	kind := "aql"
	if in.Lang == "sql" {
		kind = "sql"
	}
	var dataset any
	if kind == "aql" && in.Dataset != "" {
		dataset = in.Dataset
	}
	apptracing.SetAttributes(ctx, caller.QueryKind.Is(kind), caller.Dataset.Is(dataset))
}

// sink receives a run's result as it is produced.
type sink struct {
	status    func(phase string) error
	compiled  func(sql, aql string, input *QueryInput) error
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

// statement is a query as the query cache sees it: what runs, in its data
// source, and what describes it.
type statement struct {
	query       query.DataSourceQuery // the SQL, in its data source
	aql         string                // the AQL that compiled, its Query Input applied; "" for SQL
	compilerKey string                // anfra-node's key; "" for SQL, or from an anfra-node without one
	dataset     string                // the dataset an AQL query ran against; "" for SQL
}

// tags are what a clean selects the statement's cached results by: its data
// source, and an AQL query's dataset (docs/designs/query-cache.md#tags). The
// models it read come when anfra-node says which they are.
func (st statement) tags() []querycache.Tag {
	tags := []querycache.Tag{{Kind: querycache.TagDataSource, Value: st.query.DataSource()}}
	if st.dataset != "" {
		tags = append(tags, querycache.Tag{Kind: querycache.TagDataset, Value: st.dataset})
	}
	return tags
}

// compile is the AQL query compiled, its step on t's timeline: nil for a SQL
// query, which runs as written.
func (r queryRun) compile(ctx context.Context, t *tracker) (*anfranode.CompileToSQLResult, error) {
	if r.in.Lang == "sql" {
		return nil, nil
	}
	if err := t.begin("compiling_query", 0); err != nil {
		return nil, err
	}
	compiled, err := compileAQL(ctx, r.cc, r.src, r.in.Dataset, r.aql, r.in.run())
	if err != nil {
		return nil, err
	}
	return &compiled, nil
}

// inDataSource is the query in its data source: an AQL query's SQL in the one
// its dataset targets, or a SQL query in the one it named, as it was accepted.
func (r queryRun) inDataSource(ctx context.Context, compiled *anfranode.CompileToSQLResult) (query.DataSourceQuery, error) {
	if compiled == nil {
		return r.sql.DataSourceQuery, nil
	}
	return query.InDataSource(ctx, r.src.DataSources, *compiled)
}

// ran is what a run produced besides its rows: its timings, what the cache did,
// and for a page, whether more follow and, for a pivot's, its grid's size.
type ran struct {
	timings Timings
	cache   CacheResult
	hasMore bool
	pivot   *PivotPage
}

// execute runs the query: compiled, when it is AQL, then answered through the
// query cache, its result truncated at truncate rows (query.NoLimit: none) and
// failing past the host's maximum. The invocation's query observer is told of
// it as it goes (observation).
func (r queryRun) execute(ctx context.Context, truncate int, s sink) (_ ran, err error) {
	t := newTracker(s)
	o := observe(r.cc, t)
	defer func() { o.finished(ctx, err) }()

	compiled, err := r.compile(ctx, t)
	if err != nil {
		return ran{}, err
	}
	// What runs, and what describes it: the SQL as written, or as compiled.
	sql, dataSource, dbType := r.in.Query, r.in.DataSource, r.sql.DBType()
	var (
		aql, compilerKey, dataset string
		input                     *QueryInput
		columns                   []anfranode.ExploreColumn
	)
	if compiled != nil {
		sql, dataSource, dbType = compiled.SQL, compiled.DataSource.Name, compiled.DataSource.DBType
		aql, compilerKey, dataset = compiled.AQL, compiled.CacheKey, r.in.Dataset
		input, columns = r.in.Input.filled(compiled.Seeds), compiled.Columns
	}

	// Announced before it reaches its data source, so a query on one the repo
	// lacks still shows its SQL.
	if err := s.compiled(sql, aql, input); err != nil {
		return ran{}, err
	}
	o.compiled(ctx, command.CompiledQuery{DataSource: dataSource, DataSourceType: dbType, Dataset: dataset, AQL: aql, SQL: sql})

	dq, err := r.inDataSource(ctx, compiled)
	if err != nil {
		return ran{}, asQueryFailed(err)
	}
	st := statement{query: dq, aql: aql, compilerKey: compilerKey, dataset: dataset}
	// The page read as it was compiled, outermost: the cache keeps the rows as
	// the warehouse answered them, and its hit is read the same way.
	p := newPageReading(r.in.Input.Pagination, compiled)
	h := p.handler(o.handler(ctx, canalquery.Handler{
		Header: func(hd canalquery.Header) error { return s.header(describeColumns(hd, columns), hd.Fields) },
		Row:    s.row,
	}))
	bounds := query.Bounded(truncate, r.cc.QueryLimits)
	cache, err := st.runCached(ctx, r, t, r.cc.Clients.CanalQuery, bounds, h)
	if err != nil {
		return ran{}, asQueryFailed(err)
	}
	return ran{timings: t.timings(), cache: cache, hasMore: p.hasMore(), pivot: p.pivot()}, nil
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

// compiled tells the observer the query q, its id aside, is about to be sent:
// from its AQL, for AQL; its SQL as written, for SQL.
func (o *observation) compiled(ctx context.Context, q command.CompiledQuery) {
	q.ID = o.id
	o.obs.QueryCompiled(ctx, q)
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

// asQueryFailed is err as query_failed, with canal's message, when it is the
// data source not running the query — what canal-query scopes "User": the
// query rejected, the data source unreachable or refusing its credentials.
// canal-query's other failures are the server's — its own ("Server"), or a
// request the engine built wrong ("Client") — and stay unclassified, an
// internal error, as does anything else; an error with a code already, such
// as result_too_large, passes as it is.
func asQueryFailed(err error) error {
	if _, formal := err.(*apperr.Error); formal {
		return err
	}
	e, ok := errors.AsType[*canalquery.Error](err)
	if !ok || e.Scope != "User" {
		return err
	}
	return apperr.Encapsulate(err, errcode.QueryFailed, e.Message)
}

// page is the query answered whole: one page, within the host's page bytes.
// With a page asked for, the page is compiled into the SQL, which says
// whether more follow (pageReading). Without, it is the first MaxPageSize
// rows, and one more row asked for says whether there are more.
func (r queryRun) page(ctx context.Context) (QueryResult, error) {
	keep, truncate := math.MaxInt, query.NoLimit
	if r.in.Input.pageSize() == 0 {
		keep, truncate = MaxPageSize, MaxPageSize+1
		if r.limit != query.NoLimit && r.limit <= MaxPageSize {
			keep, truncate = r.limit, r.limit
		}
	}
	maxBytes := r.cc.QueryLimits.PageBytes()
	result := QueryResult{Result: QueryRows{Records: []canalquery.Row{}}}
	size := 0
	ran, err := r.execute(ctx, truncate, sink{
		status:    func(string) error { return nil },
		warehouse: func(string) error { return nil },
		scanned:   func(int64) {},
		compiled: func(sql, aql string, input *QueryInput) error {
			result.SQL, result.AQL, result.Input = sql, aql, input
			return nil
		},
		header: func(columns []Column, fields []string) error {
			result.Columns, result.Result.Fields = columns, fields
			return nil
		},
		row: func(row canalquery.Row) error {
			if len(result.Result.Records) == keep {
				result.HasMore = true // the row past the page
				return nil
			}
			if size += len(row); size > maxBytes {
				return query.NewPageTooLarge(maxBytes)
			}
			result.Result.Records = append(result.Result.Records, row)
			return nil
		},
	})
	if err != nil {
		return QueryResult{}, err
	}
	if r.in.Input.pageSize() > 0 {
		result.HasMore, result.Pivot = ran.hasMore, ran.pivot
	}
	result.Timings, result.Cache = ran.timings, ran.cache
	return result, nil
}

// stream is the query's events: compiled, header, then its rows in batches,
// then summary. With page_size, the rows are that page; without, every row, up
// to the host's maximum (query.Limits).
func (r queryRun) stream(ctx context.Context, events apikit.Events[QueryEvent]) error {
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
	ran, err := r.execute(ctx, r.limit, sink{
		status: events.Status,
		compiled: func(sql, aql string, input *QueryInput) error {
			return events.Emit(QueryCompiled{Type: "compiled", SQL: sql, AQL: aql, Input: input})
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
	return events.Emit(QuerySummary{Type: "summary", RowCount: count, HasMore: ran.hasMore, Pivot: ran.pivot,
		BytesScanned: scanned, Cache: ran.cache, Timings: ran.timings})
}
