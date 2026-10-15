package query

import (
	"context"
	"errors"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
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

// QueryEvent is one of query's own events, in a stream: compiled, then header,
// then rows in batches, then summary. Which phase the query is in — compiling,
// executing — is the stream's status.
type QueryEvent interface{ queryEvent() }

// QueryCompiled is the SQL the query runs, as soon as it is known.
type QueryCompiled struct {
	Type string `json:"type" enum:"compiled"`
	SQL  string `json:"sql"`
	AQL  string `json:"aql,omitempty" doc:"the AQL that runs: the query with its Query Input applied; absent for a SQL query"`
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
}

func (QueryCompiled) queryEvent() {}
func (QueryHeader) queryEvent()   {}
func (QueryRecords) queryEvent()  {}
func (QuerySummary) queryEvent()  {}

// queryEvents are query's events, for its op to declare.
var queryEvents = []any{QueryCompiled{}, QueryHeader{}, QueryRecords{}, QuerySummary{}}

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
	// ds is a SQL query's data source; src and aql an AQL query's, its limit:
	// directive taken out into limit.
	ds    datasource.DataSource
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
		ds, err := in.dataSource(ctx, cc)
		if err != nil {
			return QueryRun{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{CanalQuery: true}); err != nil {
			return QueryRun{}, err
		}
		q.ds = ds
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
	status   func(phase string) error
	compiled func(sql, aql string) error
	header   func(columns []Column, fields []string) error
	row      func(canalquery.Row) error
}

// execute compiles the query, when it is AQL, and runs it, its result truncated
// at truncate rows (query.NoLimit: none) and failing past the host's maximum.
func (q queryRun) execute(ctx context.Context, truncate int, s sink) error {
	canal := q.cc.Clients.CanalQuery
	bounds := query.RowBounds{Truncate: truncate, Max: q.cc.QueryLimits.ResultRows()}
	if q.in.Lang == "sql" {
		if err := s.compiled(q.in.Query, ""); err != nil {
			return err
		}
		if err := s.status("executing"); err != nil {
			return err
		}
		err := query.StreamSQL(ctx, canal, q.ds, q.in.Query, bounds, canalquery.Handler{
			Header: func(h canalquery.Header) error { return s.header(describeColumns(h, nil), h.Fields) },
			Row:    s.row,
		})
		if err != nil {
			return asQueryFailed(err)
		}
		return nil
	}
	if err := s.status("compiling"); err != nil {
		return err
	}
	compiled, err := compileAQL(ctx, q.cc, q.src, q.in.Dataset, q.aql, q.in.run())
	if err != nil {
		return err
	}
	if err := s.compiled(compiled.SQL, compiled.AQL); err != nil {
		return err
	}
	if err := s.status("executing"); err != nil {
		return err
	}
	err = query.Stream(ctx, canal, q.src.DataSources, compiled, bounds, canalquery.Handler{
		Header: func(h canalquery.Header) error { return s.header(describeColumns(h, compiled.Columns), h.Fields) },
		Row:    s.row,
	})
	if err != nil {
		return asQueryFailed(err)
	}
	return nil
}

// asQueryFailed is err as query_failed, with canal's message, when it is
// canal-query failing to run the query — the data source's failure. A
// client-scope error from canal is a request the engine built wrong — the
// engine's bug, not the data source's — and stays unclassified, as does
// anything else: our own errors, such as result_too_large, pass as they are.
func asQueryFailed(err error) error {
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
	err := q.execute(ctx, truncate, sink{
		status: func(string) error { return nil },
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
	return r, nil
}

// stream is the query's events: compiled, header, then its rows in batches,
// then summary. With page_size, the rows are that page; without, every row, up
// to the host's maximum (query.Limits).
func (q queryRun) stream(ctx context.Context, events apikit.Events[QueryEvent]) error {
	batch, size, count := []canalquery.Row{}, 0, 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := events.Emit(QueryRecords{Type: "rows", Records: batch})
		batch, size = []canalquery.Row{}, 0
		return err
	}
	err := q.execute(ctx, q.limit, sink{
		status: events.Status,
		compiled: func(sql, aql string) error {
			return events.Emit(QueryCompiled{Type: "compiled", SQL: sql, AQL: aql})
		},
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
	return events.Emit(QuerySummary{Type: "summary", RowCount: count, HasMore: q.in.PageSize > 0 && count == q.in.PageSize})
}
