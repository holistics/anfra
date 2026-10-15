package query

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// spySources records what is asked of the data sources, answering from set, or
// with err for every Connection.
type spySources struct {
	set         datasource.Set
	err         error
	dialects    int
	connections []string
}

func (s *spySources) Dialects(ctx context.Context) ([]datasource.Dialect, error) {
	s.dialects++
	return s.set.Dialects(ctx)
}

func (s *spySources) Connection(ctx context.Context, name string) (datasource.DataSource, error) {
	s.connections = append(s.connections, name)
	if s.err != nil {
		return datasource.DataSource{}, s.err
	}
	return s.set.Connection(ctx, name)
}

var warehouse = datasource.Set{
	"warehouse": {Name: "warehouse", DBType: "postgresql", Connection: map[string]any{"password": "secret"}},
	"archive":   {Name: "archive", DBType: "bigquery", Connection: map[string]any{"key": "secret"}},
}

// Compiling asks for every data source's dialect and never for a connection.
func TestCompilingAsksOnlyForDialects(t *testing.T) {
	spy := &spySources{set: warehouse}
	req, err := CompileRequest(context.Background(), Source{Repo: repo.Local{Dir: "/r", ID: "r"}.Repo(), DataSources: spy}, "d", "q")
	if err != nil {
		t.Fatal(err)
	}
	if spy.dialects != 1 || len(spy.connections) != 0 {
		t.Errorf("compiling asked for %d dialects and connections %v; want dialects once, no connection", spy.dialects, spy.connections)
	}
	if len(req.DataSources) != 2 || req.DataSources["archive"].DBType != "bigquery" {
		t.Errorf("dialects sent = %v", req.DataSources)
	}
}

// Running asks for exactly one connection, the compiled query's data source's,
// and no dialects.
func TestRunningAsksForOneConnection(t *testing.T) {
	spy := &spySources{set: warehouse}
	compiled := anfranode.CompileToSQLResult{SQL: "select 1", DataSource: anfranode.CompileDataSource{Name: "warehouse", DBType: "postgresql"}}
	canal := canalquery.NewClient("http://127.0.0.1:1", false).ForTenant("org_1") // nothing listens: the run fails after the data source is fetched
	_ = Stream(context.Background(), canal, spy, compiled, Unbounded, canalquery.Handler{Row: func(canalquery.Row) error { return nil }})
	if spy.dialects != 0 || len(spy.connections) != 1 || spy.connections[0] != "warehouse" {
		t.Errorf("running asked for %d dialects and connections %v; want the warehouse connection only", spy.dialects, spy.connections)
	}
}

// A query runs with its data source's connection and settings.
func TestRunsWithTheDataSourcesSettings(t *testing.T) {
	ds := datasource.DataSource{Name: "w", DBType: "postgresql", Connection: map[string]any{"host": "h"}, Settings: map[string]any{"require_ssl": true}}
	st, err := on(ds, canalquery.Statement{SQL: "select 1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.DBConfig["host"] != "h" || st.DBSetting["require_ssl"] != true {
		t.Errorf("statement = %+v", st)
	}
}

// A data source the repo does not have is named in the refusal; a host's own
// refusal (an authorization failure, say) reaches the caller as it is.
func TestConnectionRefusals(t *testing.T) {
	compiled := anfranode.CompileToSQLResult{SQL: "select 1", DataSource: anfranode.CompileDataSource{Name: "gone"}}
	canal := canalquery.NewClient("http://127.0.0.1:1", false).ForTenant("org_1")
	if err := Stream(context.Background(), canal, &spySources{set: warehouse}, compiled, Unbounded, canalquery.Handler{}); err == nil || !strings.Contains(err.Error(), `"gone" is not defined`) {
		t.Errorf("an unknown data source: %v", err)
	}
	denied := errors.New("the host refuses this data source")
	if err := Stream(context.Background(), canal, &spySources{set: warehouse, err: denied}, compiled, Unbounded, canalquery.Handler{}); !errors.Is(err, denied) {
		t.Errorf("the host's refusal: %v, want it as it is", err)
	}
}
