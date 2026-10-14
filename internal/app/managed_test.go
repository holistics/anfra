package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
)

// fakeNode is an anfra-node that answers every call with result and records each
// call's method and params, as anfra-node reads them off the wire.
type fakeNode struct {
	mu     sync.Mutex
	calls  []string
	params []map[string]any
}

func (f *fakeNode) serve(t *testing.T, result any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := jsonkit.UnmarshalRead(r.Body, &req); err != nil {
			t.Errorf("decode rpc request: %v", err)
			return
		}
		f.mu.Lock()
		f.calls, f.params = append(f.calls, req.Method), append(f.params, req.Params)
		f.mu.Unlock()
		_ = jsonkit.MarshalWrite(w, map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func managedRepo(t *testing.T) repo.Repo {
	t.Helper()
	r, err := repo.Managed("0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b",
		"0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b@0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A managed repo reaches anfra-node as its revision, never a path, with the
// host's data sources' dialects.
func TestAManagedRepoIsSentAsItsRevision(t *testing.T) {
	var node fakeNode
	srv := node.serve(t, anfranode.CompileToSQLResult{SQL: "select 1", AQL: "q",
		DataSource: anfranode.CompileDataSource{Name: "warehouse", DBType: "bigquery"}})
	cc := command.CommandContext{
		Clients:     command.Clients{Node: anfranode.NewClientHTTP(srv.URL)},
		Repo:        managedRepo(t),
		DataPerms:   dataperm.Unrestricted(),
		DataSources: datasource.Set{"warehouse": {Name: "warehouse", DBType: "bigquery"}},
	}
	if _, err := Invoke(context.Background(), cc, "query.compile", []byte(`{"query":"q","dataset":"ecommerce"}`)); err != nil {
		t.Fatalf("query.compile: %v", err)
	}
	if len(node.params) != 1 || node.calls[0] != "aql.compile_to_sql" {
		t.Fatalf("calls = %v", node.calls)
	}
	p := node.params[0]
	if p["revision"] != "0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b@0123456789abcdef0123456789abcdef01234567" ||
		p["repoId"] != "0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b" {
		t.Errorf("repo on the wire: revision %v, repoId %v", p["revision"], p["repoId"])
	}
	if _, sent := p["repoPath"]; sent {
		t.Errorf("a managed repo sent repoPath %v", p["repoPath"])
	}
	if ds, ok := p["dataSources"].(map[string]any)["warehouse"].(map[string]any); !ok || ds["dbtype"] != "bigquery" {
		t.Errorf("dataSources on the wire = %v, want the host's", p["dataSources"])
	}
}

// A managed repo has no data_sources.yml: a command that compiles needs the
// host's data sources, and without them is refused before anfra-node is called.
func TestAManagedRepoNeedsTheHostsDataSources(t *testing.T) {
	var node fakeNode
	srv := node.serve(t, map[string]any{})
	cc := command.CommandContext{
		Clients:   command.Clients{Node: anfranode.NewClientHTTP(srv.URL)},
		Repo:      managedRepo(t),
		DataPerms: dataperm.Unrestricted(),
	}
	for cmd, input := range map[string]string{
		"query.compile":  `{"query":"q","dataset":"ecommerce"}`,
		"query.validate": `{"query":"q","dataset":"ecommerce"}`,
		"show":           `{"fqn":"ecommerce"}`,
	} {
		if _, err := Invoke(context.Background(), cc, cmd, []byte(input)); err == nil || !strings.Contains(err.Error(), "Invocation.DataSources") {
			t.Errorf("%s: %v, want a refusal naming Invocation.DataSources", cmd, err)
		}
	}
	if len(node.calls) != 0 {
		t.Errorf("anfra-node was called: %v", node.calls)
	}
}

// validate needs no data sources: a managed repo validates with the revision
// alone.
func TestAManagedRepoValidatesWithoutDataSources(t *testing.T) {
	var node fakeNode
	srv := node.serve(t, map[string]any{"compileErrors": []any{}, "reports": []any{}})
	cc := command.CommandContext{
		Clients:   command.Clients{Node: anfranode.NewClientHTTP(srv.URL)},
		Repo:      managedRepo(t),
		DataPerms: dataperm.Unrestricted(),
	}
	if _, err := Invoke(context.Background(), cc, "validate", []byte(`{}`)); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(node.params) != 1 || node.params[0]["revision"] == nil {
		t.Errorf("aml.validate params = %v", node.params)
	}
}

// The catalog commands reach anfra-node with the repo as every repo-scoped call
// names it: a managed repo by its revision, never a path.
func TestAManagedRepoHasACatalog(t *testing.T) {
	var node fakeNode
	srv := node.serve(t, map[string]any{"results": []any{map[string]any{"target": map[string]any{"parent": "orders", "child": "amount"}, "query": "q"}}})
	cc := command.CommandContext{
		Clients: command.Clients{
			Node:       anfranode.NewClientHTTP(srv.URL),
			CanalQuery: canalquery.NewClient(srv.URL, false),
		},
		Repo:      managedRepo(t),
		DataPerms: dataperm.Unrestricted(),
	}
	for cmd, input := range map[string]string{
		"ingest":  `{}`,
		"search":  `{"query":["revenue"]}`,
		"lineage": `{"targets":[{"parent":"orders","child":"amount"}]}`,
	} {
		if _, err := Invoke(context.Background(), cc, cmd, []byte(input)); err != nil {
			t.Errorf("%s: %v", cmd, err)
		}
	}
	if want := []string{"catalog.ingest", "catalog.search", "catalog.lineage"}; len(node.calls) != len(want) {
		t.Fatalf("calls = %v, want one each of %v", node.calls, want)
	}
	for i, p := range node.params {
		c, _ := p["context"].(map[string]any)
		if c["revision"] != "0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b@0123456789abcdef0123456789abcdef01234567" ||
			c["repoId"] != "0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b" || c["canalQueryBaseUrl"] != srv.URL {
			t.Errorf("%s context = %v", node.calls[i], c)
		}
		if _, sent := c["repoPath"]; sent {
			t.Errorf("%s: a managed repo sent repoPath %v", node.calls[i], c["repoPath"])
		}
	}
}

// countingSources counts the connections asked for.
type countingSources struct {
	datasource.Set
	connections int
}

func (c *countingSources) Connection(ctx context.Context, name string) (datasource.DataSource, error) {
	c.connections++
	return c.Set.Connection(ctx, name)
}

// Compiling a SQL query is returning it as it is: it checks the data source
// exists, and never asks for its connection.
func TestCompilingSQLAsksForNoConnection(t *testing.T) {
	sources := &countingSources{Set: datasource.Set{"warehouse": {Name: "warehouse", DBType: "postgresql"}}}
	cc := command.CommandContext{Repo: managedRepo(t), DataPerms: dataperm.Unrestricted(), DataSources: sources}
	if _, err := Invoke(context.Background(), cc, "query.compile", []byte(`{"query":"select 1","lang":"sql","data_source":"warehouse"}`)); err != nil {
		t.Fatalf("query.compile in SQL: %v", err)
	}
	_, err := Invoke(context.Background(), cc, "query.compile", []byte(`{"query":"select 1","lang":"sql","data_source":"gone"}`))
	if err == nil || !strings.Contains(err.Error(), "no data source gone") {
		t.Errorf("an unknown data source: %v", err)
	}
	if sources.connections != 0 {
		t.Errorf("compiling SQL asked for %d connections, want none", sources.connections)
	}
}
