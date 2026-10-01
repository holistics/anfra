package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
)

func TestParseInput(t *testing.T) {
	tests := []struct {
		name    string
		in      any
		want    string
		wantErr bool
	}{
		{name: "absent", in: nil, want: ""},
		{name: "empty string", in: "  ", want: ""},
		{name: "JSON string from the CLI", in: ` {"filters":[]} `, want: `{"filters":[]}`},
		{name: "object from /call", in: map[string]any{"filters": []any{}}, want: `{"filters":[]}`},
		{name: "not JSON", in: "filters", wantErr: true},
		{name: "JSON but not an object", in: "[1]", wantErr: true},
		{name: "JSON null", in: "null", wantErr: true},
		{name: "wrong type over /call", in: float64(1), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInput(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseInput(%v) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if string(got) != tt.want {
				t.Errorf("ParseInput(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name           string
		page, pageSize int
		want           *sidecar.Pagination
		wantErr        bool
	}{
		{name: "no paging", want: nil},
		{name: "a page size alone is the first page", pageSize: 20, want: &sidecar.Pagination{Page: 1, PageSize: 20}},
		{name: "page and size", page: 3, pageSize: 20, want: &sidecar.Pagination{Page: 3, PageSize: 20}},
		{name: "a page without a size", page: 2, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePagination(tt.page, tt.pageSize)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Without paging or a timezone, the request carries neither, so the sidecar's
// defaults (all rows, its own timezone) apply.
func TestCompileOmitsUnsetExecutionOptions(t *testing.T) {
	b, err := json.Marshal(sidecar.CompileToSQLRequest{RepoID: "r"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"pagination"`, `"options"`, `"input"`} {
		if strings.Contains(string(b), key) {
			t.Errorf("request %s should omit %s", b, key)
		}
	}
}

// Compile must send the sidecar the repo's cache identity and the Query Input
// untouched, and surface the Executed AQL it returns.
func TestCompileSendsRepoIDAndInput(t *testing.T) {
	dir := t.TempDir()
	r := repo.Resolve(dir)
	if err := os.MkdirAll(r.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "data_sources:\n  demo_pg:\n    type: postgresql\n"
	if err := os.WriteFile(filepath.Join(r.ConfigDir, "data_sources.yml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Method string                      `json:"method"`
		Params sidecar.CompileToSQLRequest `json:"params"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := json.NewDecoder(req.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"sql":"SELECT 1","aql":"explore { filters { a.b is 'x' } }","columns":[{"name":"b","fieldName":"b","modelId":"a"}],"dataSource":{"name":"demo_pg","dbtype":"postgresql"}}}`))
	}))
	defer srv.Close()

	input := json.RawMessage(`{"filters":[{"field":"a.b","operator":"is","values":["x"]}]}`)
	run := Run{Input: input, Pagination: &sidecar.Pagination{Page: 2, PageSize: 10}, Timezone: "Asia/Tokyo"}
	res, err := Compile(context.Background(), sidecar.NewAnfraNodeClientHTTP(srv.URL), r, "ecommerce", "explore { }", run)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	if got.Method != "aql.compile_to_sql" {
		t.Errorf("method = %q", got.Method)
	}
	if got.Params.RepoID == "" || got.Params.RepoID != r.ID {
		t.Errorf("repoId = %q, want %q", got.Params.RepoID, r.ID)
	}
	if string(got.Params.Input) != string(input) {
		t.Errorf("input = %s, want %s", got.Params.Input, input)
	}
	if p := got.Params.Pagination; p == nil || p.Page != 2 || p.PageSize != 10 {
		t.Errorf("pagination = %+v, want page 2 of 10", p)
	}
	if o := got.Params.Options; o == nil || o.TimezoneRegion != "Asia/Tokyo" {
		t.Errorf("options = %+v, want timezoneRegion Asia/Tokyo", o)
	}
	if string(res.Columns) != `[{"name":"b","fieldName":"b","modelId":"a"}]` {
		t.Errorf("columns = %s, want them passed through", res.Columns)
	}
	if res.AQL != "explore { filters { a.b is 'x' } }" {
		t.Errorf("executed AQL = %q", res.AQL)
	}
}
