package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// Compile must send the sidecar the repo's cache identity and the Query Input
// untouched, and surface the Executed AQL it returns.
func TestCompileSendsRepoIDAndInput(t *testing.T) {
	dir := t.TempDir()
	r := repo.Resolve(dir)
	if err := os.MkdirAll(r.ConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "data_sources:\n  demo_pg:\n    type: postgresql\n"
	if err := os.WriteFile(filepath.Join(r.ConfigDir, "data_sources.yml"), []byte(manifest), 0o644); err != nil {
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
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"sql":"SELECT 1","aql":"explore { filters { a.b is 'x' } }","dataSource":{"name":"demo_pg","dbtype":"postgresql"}}}`))
	}))
	defer srv.Close()

	input := json.RawMessage(`{"filters":[{"field":"a.b","operator":"is","values":["x"]}]}`)
	res, err := Compile(context.Background(), sidecar.NewAnfraNodeClientHTTP(srv.URL), r, "ecommerce", "explore { }", input)
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
	if res.AQL != "explore { filters { a.b is 'x' } }" {
		t.Errorf("executed AQL = %q", res.AQL)
	}
}
