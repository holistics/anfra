package query

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/jsonkit"
)

// sqlRun is a SQL query on a canal-query that answers rows rows of one column,
// with what it asked canal for kept in truncate.
func sqlRun(t *testing.T, rows int, limits query.Limits) (QueryRun, *float64) {
	t.Helper()
	var truncate float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var sent map[string]any
		_ = jsonkit.Unmarshal(body, &sent)
		truncate, _ = sent["truncate_rows"].(float64)
		n := rows
		if truncate >= 0 && int(truncate) < n {
			n = int(truncate)
		}
		var b strings.Builder
		b.WriteString(`{"__holistics_header__":true,"fields":["id"],"types":["int8"],"arrow_types":[{"type":"int","bit_width":64,"is_signed":true,"label":"int64"}]}` + "\n")
		for i := range n {
			fmt.Fprintf(&b, "[\"%d\"]\n", i)
		}
		b.WriteString(`{"__holistics_trailer__":true,"metadata":{},"http_code":200}` + "\n")
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	cc := command.CommandContext{
		Clients:     command.Clients{CanalQuery: canalquery.NewClient(srv.URL, false)},
		DataSources: datasource.Set{"warehouse": {Name: "warehouse", DBType: "postgresql", Connection: map[string]any{"password": "secret"}}},
		QueryLimits: limits,
	}
	run, err := newQueryRun(context.Background(), cc, QueryRunInput{QueryInput: QueryInput{Query: "select id from t", Lang: "sql", DataSource: "warehouse"}})
	if err != nil {
		t.Fatal(err)
	}
	return run, &truncate
}

// Answered whole without page_size, a query is its first MaxPageSize rows; the
// row past them, asked for, says there are more.
func TestAPageIsTheFirstRows(t *testing.T) {
	run, truncate := sqlRun(t, MaxPageSize+5, query.Limits{})
	r, err := run.Page(context.Background())
	if err != nil || len(r.Result.Records) != MaxPageSize || !r.HasMore || r.SQL != "select id from t" || len(r.Columns) != 1 {
		t.Fatalf("got %d rows, has_more %v, sql %q, %d columns, %v", len(r.Result.Records), r.HasMore, r.SQL, len(r.Columns), err)
	}
	if *truncate != MaxPageSize+1 {
		t.Errorf("asked canal for %v rows, want one past the page", *truncate)
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
	if _, err := run.Page(context.Background()); !errors.Is(err, query.PageTooLarge) {
		t.Errorf("err = %v, want page_too_large", err)
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
// count: every row up to the host's maximum, past which it fails.
func TestAStreamSendsEveryRow(t *testing.T) {
	run, truncate := sqlRun(t, 2*batchRows+5, query.Limits{})
	types, events, err := streamed(t, run)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(types, ","); got != "compiled,status,header,rows,rows,rows,summary" {
		t.Errorf("events = %s", got)
	}
	if s := events[len(events)-1].(QuerySummary); s.RowCount != 2*batchRows+5 || s.HasMore {
		t.Errorf("summary = %+v", s)
	}
	if *truncate != query.DefaultMaxResultRows+1 {
		t.Errorf("asked canal for %v rows, want every row, up to one past the maximum", *truncate)
	}
}

// A stream past the host's maximum rows fails, after the rows within it.
func TestAStreamPastTheMaximumFails(t *testing.T) {
	run, _ := sqlRun(t, 5, query.Limits{MaxResultRows: 3})
	if _, _, err := streamed(t, run); !errors.Is(err, query.ResultTooLarge) {
		t.Errorf("err = %v, want result_too_large", err)
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
