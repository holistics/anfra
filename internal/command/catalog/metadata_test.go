package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
)

type postgresSources struct {
	dialects []datasource.Dialect
	names    []string
	modes    []datasource.DataPermMode
	err      error
}

func (s *postgresSources) Dialects(context.Context) ([]datasource.Dialect, error) {
	return s.dialects, nil
}
func (s *postgresSources) Connection(_ context.Context, name string, mode datasource.DataPermMode) (datasource.DataSource, error) {
	s.names = append(s.names, name)
	s.modes = append(s.modes, mode)
	return datasource.DataSource{Name: name, DBType: "postgresql", Connection: map[string]any{"password": "secret"}, Settings: map[string]any{"require_ssl": true}}, s.err
}

func TestMetadataUsesFirstPostgresSourceAndExistingQueryAccess(t *testing.T) {
	sources := &postgresSources{dialects: []datasource.Dialect{{Name: "duck", DBType: "duckdb"}, {Name: "z", DBType: "postgresql"}, {Name: "a", DBType: "postgresql"}}}
	requests := 0
	rowCount := 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var sent map[string]any
		if err := jsonkit.UnmarshalRead(r.Body, &sent); err != nil {
			t.Error(err)
			return
		}
		if sent["tenant_id"] != "org" || sent["dbconfig"].(map[string]any)["password"] != "secret" || sent["dbsetting"].(map[string]any)["require_ssl"] != true {
			t.Error("connection settings or tenant were not forwarded")
		}
		_, _ = fmt.Fprintln(w, `{"__holistics_header__":true,"fields":["database_name","schema_name","table_name","column_name","data_type"],"types":["text","text","text","text","text"]}`)
		for range rowCount {
			_, _ = fmt.Fprintln(w, `["analytics","public","orders","id","bigint"]`)
		}
		_, _ = fmt.Fprintln(w, `{"__holistics_trailer__":true,"metadata":{"record_count":1},"http_code":200}`)
	}))
	defer srv.Close()
	cc := command.CommandContext{DataSources: sources, DataPerms: dataperm.Ungoverned(), TenantID: "org", Clients: command.Clients{CanalQuery: canalquery.NewClient(srv.URL, false)}}
	result, err := loadMetadata(context.Background(), cc)
	if err != nil || result == nil {
		t.Fatalf("load: %+v, %v", result, err)
	}
	if result.Postgres.SourceKey != "z" || requests != 4 || len(result.Postgres.Snapshot.Tables) != 1 {
		t.Fatalf("source = %q, requests = %d, tables = %v", result.Postgres.SourceKey, requests, result.Postgres.Snapshot.Tables)
	}
	for i, name := range sources.names {
		if name != "z" || sources.modes[i] != datasource.DataPermModeUngoverned {
			t.Errorf("connection = %s, %v", name, sources.modes[i])
		}
	}
	encoded, err := jsonkit.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "require_ssl") {
		t.Fatalf("metadata contains connection settings: %v", err)
	}

	sources.err = errors.New("access denied")
	if metadata, err := loadMetadata(context.Background(), cc); metadata != nil || err == nil {
		t.Fatalf("denied: %v, %v", metadata, err)
	}
	if requests != 4 {
		t.Fatal("a denied connection reached canal")
	}
	sources.err = nil
	cc.DataPerms = dataperm.Governed(nil)
	connections := len(sources.names)
	if metadata, err := loadMetadata(context.Background(), cc); metadata != nil || err == nil || len(sources.names) != connections {
		t.Fatalf("governed invocation reached a connection: %v, %v", metadata, err)
	}
	cc.DataPerms = dataperm.Ungoverned()
	cc.QueryLimits = query.Limits{MaxResultRows: 1}
	rowCount = 2
	if metadata, err := loadMetadata(context.Background(), cc); metadata != nil || !errors.Is(err, query.ResultTooLarge) {
		t.Fatalf("oversized metadata was accepted: %v, %v", metadata, err)
	}
	// A source of another type causes no warehouse access.
	sources.dialects = []datasource.Dialect{{Name: "duck", DBType: "duckdb"}}
	if metadata, err := loadMetadata(context.Background(), cc); metadata != nil || err != nil || requests != 5 {
		t.Fatalf("no Postgres: %v, %v", metadata, err)
	}
}
