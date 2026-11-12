package query

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// progressLines are what canal-query reports of a BigQuery run before its
// result, child steps and a retry among them.
var progressLines = []string{
	`{"__holistics_progress__":true,"kind":"step","at_ms":1,"step":"connecting_to_dw"}`,
	`{"__holistics_progress__":true,"kind":"step","at_ms":2,"step":"connecting_to_dw.open_connection"}`,
	`{"__holistics_progress__":true,"kind":"retry","at_ms":3,"attempt":2,"delay_ms":1000}`,
	`{"__holistics_progress__":true,"kind":"step","at_ms":4,"step":"connecting_to_dw"}`,
	`{"__holistics_progress__":true,"kind":"step","at_ms":5,"step":"executing_query"}`,
	`{"__holistics_progress__":true,"kind":"step","at_ms":6,"step":"executing_query.submit_query"}`,
	`{"__holistics_progress__":true,"kind":"dw_query_id","at_ms":7,"dw_query_id":"job_42"}`,
	`{"__holistics_progress__":true,"kind":"step","at_ms":8,"step":"executing_query.poll_query"}`,
	`{"__holistics_progress__":true,"kind":"dw_bytes_processed","at_ms":9,"dw_bytes_processed":2048}`,
	`{"__holistics_progress__":true,"kind":"step","at_ms":10,"step":"fetching_query_result"}`,
}

// sqlRun is a SQL query on a canal-query that reports progressLines, then
// answers rows rows of one column; or, with failure, fails with it after its
// progress. What it was sent is kept.
func sqlRun(t *testing.T, rows int, limits query.Limits, failure ...string) (QueryRun, *map[string]any) {
	t.Helper()
	return sqlRunObserved(t, rows, limits, nil, failure...)
}

// sqlRunObserved is sqlRun, its invocation's queries told to obs.
func sqlRunObserved(t *testing.T, rows int, limits query.Limits, obs command.QueryObserver, failure ...string) (QueryRun, *map[string]any) {
	t.Helper()
	cc, sent := canalContext(t, rows, limits, obs, failure...)
	run, err := newQueryRun(context.Background(), cc, QueryRunInput{QueryInput: QueryInput{Query: "select id from t", Lang: "sql", DataSource: "warehouse"}})
	if err != nil {
		t.Fatal(err)
	}
	return run, sent
}

// canalContext is an invocation on a canal-query that reports progressLines,
// then answers rows rows of one column, or fails with failure; its queries told
// to obs. What canal-query was sent is kept.
func canalContext(t *testing.T, rows int, limits query.Limits, obs command.QueryObserver, failure ...string) (command.CommandContext, *map[string]any) {
	t.Helper()
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = jsonkit.Unmarshal(body, &sent)
		truncate, _ := sent["truncate_rows"].(float64)
		n := rows
		if truncate >= 0 && int(truncate) < n {
			n = int(truncate)
		}
		var b strings.Builder
		for _, l := range progressLines {
			b.WriteString(l + "\n")
		}
		// As long as its progress says it took: canal-query's times are real.
		time.Sleep(15 * time.Millisecond)
		if len(failure) > 0 {
			b.WriteString(`{"__holistics_trailer__":true,"metadata":{},"http_code":422,"error":` + failure[0] + "}\n")
			_, _ = w.Write([]byte(b.String()))
			return
		}
		b.WriteString(`{"__holistics_header__":true,"fields":["id"],"types":["int8"],"arrow_types":[{"type":"int","bit_width":64,"is_signed":true,"label":"int64"}]}` + "\n")
		for i := range n {
			fmt.Fprintf(&b, "[\"%d\"]\n", i)
		}
		b.WriteString(`{"__holistics_trailer__":true,"metadata":{},"http_code":200}` + "\n")
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	cc := command.CommandContext{
		Clients:     command.Clients{CanalQuery: canalquery.NewClient(srv.URL, false).ForTenant("org_1")},
		DataSources: datasource.Set{"warehouse": {Name: "warehouse", DBType: "postgresql", Connection: map[string]any{"password": "secret"}}},
		QueryLimits: limits,
		Observers:   command.Observers{Queries: obs},
	}
	return cc, &sent
}

// served is the code a client is answered with for err: its outermost, as
// apperr renders it, not one found deeper in its chain.
func served(err error) string { return apperr.From(err).Code.Public().String() }

// Answered whole without page_size, a query is its first MaxPageSize rows; the
// row past them, asked for, says there are more.
func TestAPageIsTheFirstRows(t *testing.T) {
	run, sent := sqlRun(t, MaxPageSize+5, query.Limits{})
	r, err := run.Page(context.Background())
	if err != nil || len(r.Result.Records) != MaxPageSize || !r.HasMore || r.SQL != "select id from t" || len(r.Columns) != 1 {
		t.Fatalf("got %d rows, has_more %v, sql %q, %d columns, %v", len(r.Result.Records), r.HasMore, r.SQL, len(r.Columns), err)
	}
	if (*sent)["truncate_rows"] != float64(MaxPageSize+1) {
		t.Errorf("asked canal for %v rows, want one past the page", (*sent)["truncate_rows"])
	}
	run, _ = sqlRun(t, 3, query.Limits{})
	if r, err := run.Page(context.Background()); err != nil || len(r.Result.Records) != 3 || r.HasMore {
		t.Errorf("a small result: %d rows, has_more %v, %v", len(r.Result.Records), r.HasMore, err)
	}
	run, _ = sqlRun(t, 0, query.Limits{})
	if r, err := run.Page(context.Background()); err != nil || r.Result.Records == nil {
		t.Errorf("an empty result: %#v, %v; want no rows, as an empty list", r.Result.Records, err)
	}
}

// A page over the host's page bytes fails, rather than being cut short.
func TestAPageOverItsBytesFails(t *testing.T) {
	run, _ := sqlRun(t, 3, query.Limits{MaxPageBytes: 12}) // a row is 5 bytes: ["0"]
	if _, err := run.Page(context.Background()); served(err) != "page_too_large" {
		t.Errorf("err = %v, served as %s; want page_too_large", err, served(err))
	}
}

// streamed is a run's stream, each event by its type.
func streamed(t *testing.T, run QueryRun) ([]string, []any, error) {
	t.Helper()
	var types []string
	var events []any
	err := run.Stream(context.Background(), apikit.NewEvents[QueryEvent](func(e any) error {
		b, _ := jsonkit.Marshal(e)
		var m map[string]any
		_ = jsonkit.Unmarshal(b, &m)
		types = append(types, m["type"].(string))
		events = append(events, e)
		return nil
	}))
	return types, events, err
}

// Streamed without page_size, a query sends every row, in batches, then their
// count: every row up to the host's maximums, past which it fails. Ahead of
// them, its phases as canal-query reports its steps — each once, a child step
// in its parent's — and the warehouse's id of the query; with the rows' count,
// what the warehouse scanned.
func TestAStreamSendsEveryRow(t *testing.T) {
	run, sent := sqlRun(t, 2*batchRows+5, query.Limits{})
	types, events, err := streamed(t, run)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(types, ","), "compiled,status,status,status,status,warehouse_query,status,header,rows,rows,rows,summary"; got != want {
		t.Errorf("events = %s, want %s", got, want)
	}
	var phases []string
	for _, e := range events {
		if s, ok := e.(apikit.StreamStatus); ok {
			phases = append(phases, s.Phase)
		}
	}
	if got := strings.Join(phases, ","); got != "connecting_to_dw,retrying_query,connecting_to_dw,executing_query,fetching_query_result" {
		t.Errorf("phases = %s", got)
	}
	if w := events[5].(QueryWarehouse); w.ID != "job_42" {
		t.Errorf("warehouse query = %+v", w)
	}
	if s := events[len(events)-1].(QuerySummary); s.RowCount != 2*batchRows+5 || s.HasMore || s.BytesScanned == nil || *s.BytesScanned != 2048 {
		t.Errorf("summary = %+v", s)
	}
	options, _ := (*sent)["stream_result_options"].(map[string]any)
	if (*sent)["truncate_rows"] != float64(query.DefaultMaxResultRows+1) || (*sent)["max_result_bytes"] != float64(query.DefaultMaxResultBytes) || options["progress"] != true {
		t.Errorf("asked canal for %v rows, %v bytes, progress %v; want every row and byte up to one past the maximums, with progress",
			(*sent)["truncate_rows"], (*sent)["max_result_bytes"], options["progress"])
	}
}

// A stream past the host's maximum rows fails, after the rows within it.
func TestAStreamPastTheMaximumFails(t *testing.T) {
	run, _ := sqlRun(t, 5, query.Limits{MaxResultRows: 3})
	if _, _, err := streamed(t, run); served(err) != "result_too_large" {
		t.Errorf("err = %v, served as %s; want result_too_large", err, served(err))
	}
}

// A result past the host's maximum bytes, which canal-query counts, fails as
// one past its rows does, naming the maximum.
func TestAResultPastTheMaximumBytesFails(t *testing.T) {
	failure := `{"scope":"User","type":"ResultTooLarge","message":"the query result exceeds the maximum size of 1048576 bytes (max_result_bytes): filter or limit the query"}`
	run, sent := sqlRun(t, 5, query.Limits{MaxResultBytes: 1 << 20}, failure)
	_, err := run.Page(context.Background())
	if served(err) != "result_too_large" {
		t.Fatalf("err = %v, served as %s; want result_too_large", err, served(err))
	}
	if d, ok := apperr.DetailsOf(err, query.ResultTooLarge); !ok || d.MaxBytes != 1<<20 || d.MaxRows != 0 {
		t.Errorf("details = %+v, %v; want max_bytes 1048576", d, ok)
	}
	if (*sent)["max_result_bytes"] != float64(1<<20) {
		t.Errorf("asked canal for %v bytes at most, want 1048576", (*sent)["max_result_bytes"])
	}
}

// What the data source refuses is query_failed, with its message.
func TestADataSourceFailureIsQueryFailed(t *testing.T) {
	run, _ := sqlRun(t, 5, query.Limits{}, `{"scope":"User","type":"QueryError","message":"relation \"t\" does not exist"}`)
	if _, _, err := streamed(t, run); served(err) != "query_failed" || !strings.Contains(apperr.From(err).Message, "does not exist") {
		t.Errorf("err = %v, served as %s", err, served(err))
	}
}

// What the query cannot run on is refused when the run is made, before any work.
func TestARunRefusesBeforeItWorks(t *testing.T) {
	cc := command.CommandContext{DataSources: datasource.Set{}}
	_, err := newQueryRun(context.Background(), cc, QueryRunInput{QueryInput: QueryInput{Query: "select 1", Lang: "sql", DataSource: "nope"}})
	if field(err) != "data_source" {
		t.Errorf("err = %v, want data_source refused", err)
	}
}

// A run's timings are its steps as they started, in order — child steps, the
// retry and the step run again after it included — on one timeline, and its
// end: in a stream's summary and in a page alike.
func TestTimings(t *testing.T) {
	want := "connecting_to_dw,connecting_to_dw.open_connection,retrying_query,connecting_to_dw," +
		"executing_query,executing_query.submit_query,executing_query.poll_query,fetching_query_result"
	check := func(form string, tm Timings) {
		t.Helper()
		var names []string
		last := int64(0)
		for _, s := range tm.Steps {
			names = append(names, s.Step)
			if s.StartMs < last {
				t.Errorf("%s: %s starts at %d, before the step ahead of it (%d)", form, s.Step, s.StartMs, last)
			}
			last = s.StartMs
		}
		if got := strings.Join(names, ","); got != want {
			t.Errorf("%s: steps = %s, want %s", form, got, want)
		}
		if tm.EndMs < last {
			t.Errorf("%s: ends at %d, before its last step starts (%d)", form, tm.EndMs, last)
		}
	}
	run, _ := sqlRun(t, 3, query.Limits{})
	_, events, err := streamed(t, run)
	if err != nil {
		t.Fatal(err)
	}
	check("stream", events[len(events)-1].(QuerySummary).Timings)
	run, _ = sqlRun(t, 3, query.Limits{})
	r, err := run.Page(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	check("page", r.Timings)
}

// told keeps what a query observer is told, in order.
type told struct{ calls []string }

// QueryCompiled notes whether the id is a UUIDv7, which a host may key its
// record by.
func (o *told) QueryCompiled(_ context.Context, q command.CompiledQuery) {
	id, err := uuid.Parse(string(q.ID))
	o.calls = append(o.calls, fmt.Sprintf("compiled %t on %s: %s", err == nil && id.Version() == 7, q.DataSource, q.SQL))
}

func (o *told) QueryOnWarehouse(_ context.Context, id command.QueryID, wh string) {
	o.calls = append(o.calls, fmt.Sprintf("on the warehouse %t as %s", id != "", wh))
}

func (o *told) QueryFinished(_ context.Context, id command.QueryID, f command.FinishedQuery) {
	scanned := int64(-1)
	if f.BytesScanned != nil {
		scanned = *f.BytesScanned
	}
	code := ""
	if f.Err != nil {
		code = served(f.Err)
	}
	o.calls = append(o.calls, fmt.Sprintf("finished %t: %d rows, %d bytes, %d steps, %s", id != "",
		f.Rows, scanned, len(f.Timings.Steps), code))
}

// The invocation's query observer is told of a query as it runs — compiled,
// on the warehouse, finished — in a page and a stream alike; and of a failed
// one too, with its timings and the error its caller is answered.
func TestTheQueryObserverIsToldOfAQuery(t *testing.T) {
	want := []string{
		"compiled true on warehouse: select id from t",
		"on the warehouse true as job_42",
		"finished true: 3 rows, 2048 bytes, 8 steps, ",
	}
	for _, form := range []string{"page", "stream"} {
		obs := &told{}
		run, _ := sqlRunObserved(t, 3, query.Limits{}, obs)
		var err error
		if form == "page" {
			_, err = run.Page(context.Background())
		} else {
			_, _, err = streamed(t, run)
		}
		if err != nil || strings.Join(obs.calls, "|") != strings.Join(want, "|") {
			t.Errorf("%s: told %q, %v; want %q", form, obs.calls, err, want)
		}
	}

	obs := &told{}
	run, _ := sqlRunObserved(t, 3, query.Limits{}, obs, `{"message":"relation t does not exist","scope":"User"}`)
	if _, err := run.Page(context.Background()); err == nil {
		t.Fatal("a failing query succeeded")
	}
	if last := obs.calls[len(obs.calls)-1]; last != "finished true: 0 rows, 2048 bytes, 8 steps, query_failed" {
		t.Errorf("a failed query: told %q", obs.calls)
	}
}

// An export's query is told to the observer as a query run's is.
func TestTheQueryObserverIsToldOfAnExport(t *testing.T) {
	obs := &told{}
	cc, _ := canalContext(t, 3, query.Limits{}, obs)
	cc.Exports = &memStore{}
	if _, err := runExport(context.Background(), cc, ExportInput{QueryInput: QueryInput{Query: "select id from t", Lang: "sql", DataSource: "warehouse"}}); err != nil {
		t.Fatal(err)
	}
	want := "compiled true on warehouse: select id from t|on the warehouse true as job_42|finished true: 3 rows, 2048 bytes, 8 steps, "
	if got := strings.Join(obs.calls, "|"); got != want {
		t.Errorf("told %q, want %q", got, want)
	}
}
