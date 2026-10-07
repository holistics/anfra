package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	querycmd "github.com/holistics/anfra/internal/command/query"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
)

// Every command, in every mode, against real sidecars: what it answers, and
// with which status, or how it fails. The answer's type is the compiler's to
// hold; this holds the behaviour. Every command is covered, and every command
// that can answer invalid does so in some case.
//
// It needs the sidecars' binaries and the dev warehouse:
//
//	docker compose -f docker-compose.dev.yml up -d postgres
//	ANFRA_NODE_BIN=… ANFRA_CANAL_QUERY_BIN=… go test ./internal/app -run TestCommandsAgainstRealSidecars
//
// Without the binaries it is skipped.
func TestCommandsAgainstRealSidecars(t *testing.T) {
	if os.Getenv("ANFRA_NODE_BIN") == "" || os.Getenv("ANFRA_CANAL_QUERY_BIN") == "" {
		t.Skip("set ANFRA_NODE_BIN and ANFRA_CANAL_QUERY_BIN to run commands against real sidecars")
	}
	t.Setenv("HOME", t.TempDir()) // the repo's state, the catalog among it, goes under HOME
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fixture, err := filepath.Abs("testdata/repo")
	if err != nil {
		t.Fatal(err)
	}
	valid := repo.Resolve(fixture)
	broken := repo.Resolve(brokenCopy(t, fixture))
	clients := startSidecars(ctx, t, valid)

	const ok, invalid = StatusOK, StatusInvalid
	q := func(text string) map[string]any { return map[string]any{"dataset": "ecommerce", "query": text} }
	cases := []struct {
		name    string
		repo    repo.Repo
		clients command.Clients
		req     Request
		status  Status         // the answer's, when it answers
		fails   apperr.AnyCode // the code it fails with, when it fails
	}{
		{"version", valid, clients, Request{Command: "version"}, ok, nil},
		{"status, healthy", valid, clients, Request{Command: "status"}, ok, nil},
		{"status, not running", valid, command.Clients{}, Request{Command: "status"}, invalid, nil},
		{"query", valid, clients, Request{Command: "query", Args: q("products | select(products.id, products.name)")}, ok, nil},
		{"query, invalid", valid, clients, Request{Command: "query", Args: q("nosuch | select(x.y)")}, "", query.QueryInvalid},
		{"query compile", valid, clients, Request{Command: "query.compile", Args: q("products | select(products.id)")}, ok, nil},
		{"query compile, invalid", valid, clients, Request{Command: "query.compile", Args: q("nosuch | select(x.y)")}, "", query.QueryInvalid},
		{"query validate, valid", valid, clients, Request{Command: "query.validate", Args: q("products | select(products.id)")}, ok, nil},
		{"query validate, invalid", valid, clients, Request{Command: "query.validate", Args: q("nosuch | select(x.y)")}, invalid, nil},
		{"query, SQL", valid, clients, Request{Command: "query", Args: map[string]any{"lang": "sql", "data_source": "demo", "query": "select id, name from products order by id"}}, ok, nil},
		{"query, SQL the database refuses", valid, clients, Request{Command: "query", Args: map[string]any{"lang": "sql", "data_source": "demo", "query": "select nosuch from products"}}, "", errcode.QueryFailed},
		{"query, SQL on a data source it cannot reach", valid, clients, Request{Command: "query", Args: map[string]any{"lang": "sql", "data_source": "unreachable", "query": "select 1"}}, "", errcode.QueryFailed},
		{"query compile, SQL", valid, clients, Request{Command: "query.compile", Args: map[string]any{"lang": "sql", "data_source": "demo", "query": "select 1"}}, ok, nil},
		{"query, shaped", valid, clients, Request{Command: "query", Args: shaped(nil)}, ok, nil},
		{"query compile, shaped", valid, clients, Request{Command: "query.compile", Args: shaped(nil)}, ok, nil},
		{"query, a Query Input entry anfra-node refuses", valid, clients, Request{Command: "query", Args: shaped(map[string]any{
			"input": map[string]any{"filters": []any{map[string]any{"field": "products.nope", "operator": "is", "values": []any{"x"}}}}})}, "", apperr.ValidationFailed},
		{"query, a pivot paged", valid, clients, Request{Command: "query", Args: map[string]any{"dataset": "ecommerce", "page_size": 2,
			"query": "explore { dimensions { rows { name: products.name } columns { id: products.id } } measures { n: count(products.id) } }"}}, "", apperr.ValidationFailed},
		{"ingest", valid, clients, Request{Command: "ingest"}, ok, nil},
		{"search, after ingest", valid, clients, Request{Command: "search", Args: map[string]any{"query": []any{"products"}}}, ok, nil},
		{"show, the repo", valid, clients, Request{Command: "show"}, ok, nil},
		{"show, a dataset", valid, clients, Request{Command: "show", Args: map[string]any{"fqn": "ecommerce"}}, ok, nil},
		{"show, a dataset that is not there", valid, clients, Request{Command: "show", Args: map[string]any{"fqn": "nosuch"}}, "", apperr.ValidationFailed},
		{"validate, valid", valid, clients, Request{Command: "validate"}, ok, nil},
		{"validate, invalid", broken, clients, Request{Command: "validate"}, invalid, nil},
	}

	// A shaped run answers what ran and what each column is, and only its page.
	t.Run("query, shaped, its answer", func(t *testing.T) {
		cc := command.CommandContext{Clients: clients, Repo: valid, DataPerms: dataperm.Unrestricted()}
		res, err := Dispatch(ctx, cc, Request{Command: "query", Args: shaped(nil)})
		if err != nil {
			t.Fatal(err)
		}
		r := res.Data.(querycmd.QueryResult)
		if !strings.Contains(r.AQL, "Widget") || len(r.Result.Records) != 1 {
			t.Errorf("the input was not applied: aql %q, %d rows", r.AQL, len(r.Result.Records))
		}
		want := []anfranode.ExploreColumn{
			{Name: "name", FieldName: "name", ModelID: "products", Label: r.Columns[0].Label},
			{Name: "n", FieldName: "id", ModelID: "products", Label: r.Columns[1].Label, IsMeasure: true, Aggregation: "count"},
		}
		if !reflect.DeepEqual(r.Columns, want) {
			t.Errorf("columns =\n  %+v\nwant\n  %+v", r.Columns, want)
		}

		// anfra-node's refusals land on the arg at fault.
		for path, args := range map[string]map[string]any{
			"input.filters[0].field": shaped(map[string]any{"input": map[string]any{
				"filters": []any{map[string]any{"field": "products.nope", "operator": "is", "values": []any{"x"}}}}}),
			"page_size": {"dataset": "ecommerce", "page_size": 2,
				"query": "explore { dimensions { rows { name: products.name } columns { id: products.id } } measures { n: count(products.id) } }"},
		} {
			_, err := Dispatch(ctx, cc, Request{Command: "query", Args: args})
			if d, _ := apperr.DetailsOf(err, apperr.ValidationFailed); len(d.Violations) != 1 || d.Violations[0].Field != path {
				t.Errorf("got %v, %+v; want a violation on %s", err, d.Violations, path)
			}
		}

		res, err = Dispatch(ctx, cc, Request{Command: "query", Args: map[string]any{"dataset": "ecommerce", "page": 2, "page_size": 2,
			"query": "explore { dimensions { name: products.name } }"}})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(res.Data.(querycmd.QueryResult).Result.Records); n != 1 {
			t.Errorf("page 2 of 3 rows by 2: %d rows, want 1", n)
		}
	})

	// show answers the repo with every dataset in full, and a dataset in full; a broken file comes
	// with it, and does not stop what compiles from being shown.
	t.Run("show, its answers", func(t *testing.T) {
		cc := command.CommandContext{Clients: clients, Repo: valid, DataPerms: dataperm.Unrestricted()}
		res, err := Dispatch(ctx, cc, Request{Command: "show"})
		if err != nil {
			t.Fatal(err)
		}
		r := res.Data.(anfranode.ShowResult)
		if r.Object.Repo == nil || len(r.Object.Repo.Datasets) != 1 || r.Object.Repo.Datasets[0].Fqn != "ecommerce" || len(r.Object.Repo.Datasets[0].Models) != 1 {
			t.Errorf("the repo: %+v", r.Object)
		}
		res, err = Dispatch(ctx, cc, Request{Command: "show", Args: map[string]any{"fqn": "ecommerce"}})
		if err != nil {
			t.Fatal(err)
		}
		d := res.Data.(anfranode.ShowResult).Object.Dataset
		if d == nil || len(d.Models) != 1 || d.Models[0].Fqn != "products" || len(d.Models[0].Fields) != 2 {
			t.Fatalf("the dataset: %+v", d)
		}
		if f := d.Models[0].Fields[0]; f.Fqn != "products.id" || f.Role != "dimension" || f.Type != "number" || f.Label != "ID" {
			t.Errorf("products.id: %+v", f)
		}

		cc.Repo = broken
		res, err = Dispatch(ctx, cc, Request{Command: "show"})
		if err != nil {
			t.Fatal(err)
		}
		r = res.Data.(anfranode.ShowResult)
		if len(r.Object.Repo.Datasets) != 1 || len(r.Diagnostics) == 0 || r.Diagnostics[0].FilePath != "broken.model.aml" {
			t.Errorf("a broken repo: %+v, %+v", r.Object.Repo, r.Diagnostics)
		}
	})

	ran := map[string]bool{}
	answered := map[string]map[Status]bool{}
	for _, tc := range cases {
		ran[tc.req.Command] = true
		t.Run(tc.name, func(t *testing.T) {
			res, err := Dispatch(ctx, command.CommandContext{Clients: tc.clients, Repo: tc.repo, DataPerms: dataperm.Unrestricted()}, tc.req)
			if tc.fails != nil {
				if !errors.Is(err, tc.fails) {
					t.Fatalf("got %v, want %s", err, tc.fails.Code().Qualified())
				}
				if diags, ok := apperr.DetailsOf(err, query.QueryInvalid); ok && len(diags.Diagnostics) == 0 {
					t.Error("query_invalid carries no diagnostics")
				}
				return
			}
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if res.Status != tc.status {
				t.Errorf("status %s, want %s", res.Status, tc.status)
			}
			if answered[tc.req.Command] == nil {
				answered[tc.req.Command] = map[Status]bool{}
			}
			answered[tc.req.Command][res.Status] = true
		})
	}
	for _, c := range Commands {
		if !ran[c.Name()] {
			t.Errorf("no case runs %s", c.Name())
		}
		if c.CanBeInvalid() && !answered[c.Name()][StatusInvalid] {
			t.Errorf("%s can answer invalid, but no case does", c.Name())
		}
	}
}

// shaped is a query on the fixture with everything that shapes a run, with
// over's args over it.
func shaped(over map[string]any) map[string]any {
	args := map[string]any{
		"dataset":   "ecommerce",
		"query":     "explore { dimensions { name: products.name } measures { n: count(products.id) } }",
		"page_size": 10,
		"timezone":  "Asia/Ho_Chi_Minh",
		"input": map[string]any{
			"filters": []any{map[string]any{"field": "products.name", "operator": "is", "values": []any{"Widget"}}},
			"sorts":   []any{map[string]any{"field": "name", "direction": "desc"}},
		},
	}
	for k, v := range over {
		args[k] = v
	}
	return args
}

// startSidecars spawns anfra-node and canal-query from their binaries, as the
// one-shot CLI does, for the whole test.
func startSidecars(ctx context.Context, t *testing.T, r repo.Repo) command.Clients {
	t.Helper()
	cfg := sidecar.Config{RepoID: r.ID, CompileCachePath: filepath.Join(r.CacheDir(), "compile-cache")}
	node := anfranode.New(cfg)
	if err := node.Start(ctx); err != nil {
		t.Fatalf("start anfra-node: %v", err)
	}
	t.Cleanup(node.Close)
	canal := canalquery.New(cfg)
	if err := canal.Start(ctx); err != nil {
		t.Fatalf("start canal-query: %v", err)
	}
	t.Cleanup(canal.Close)
	return command.Clients{Node: node.Client(), CanalQuery: canal.Client()}
}

// brokenCopy is the fixture plus a model that does not parse.
func brokenCopy(t *testing.T, fixture string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "broken")
	if err := os.CopyFS(dir, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.model.aml"), []byte("Model broken { type: \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
