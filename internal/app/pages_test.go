package app

import (
	"context"
	"encoding/json/jsontext"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	querycmd "github.com/holistics/anfra/internal/command/query"
	"github.com/holistics/anfra/internal/dataperm"
	localquerycache "github.com/holistics/anfra/internal/querycache/local"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canallake"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
)

// A page, as anfra-node compiles it and canal-query answers it, read as the
// SDK reads it, streamed: a query's page, its rows and whether more follow;
// a pivot's page, its grid's rows with their header rows, totals and
// placeholders, its counts taken out of its rows. Each twice: from the
// warehouse, then from the query cache, whose canal-lake answers the same
// rows with types of its own.
//
// It needs the sidecars' binaries and the dev warehouse, as
// TestCommandsAgainstRealSidecars does, and is skipped without them.
func TestPagesAgainstRealSidecars(t *testing.T) {
	if os.Getenv("ANFRA_NODE_BIN") == "" || os.Getenv("ANFRA_CANAL_BIN") == "" {
		t.Skip("set ANFRA_NODE_BIN and ANFRA_CANAL_BIN to page against real sidecars")
	}
	t.Setenv("HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fixture, err := filepath.Abs("testdata/repo")
	if err != nil {
		t.Fatal(err)
	}
	cc := pagingContext(ctx, t, repo.Resolve(fixture).Repo())

	const (
		names = "explore { dimensions { name: products.name } }"
		pivot = "explore { dimensions { rows { name: products.name } columns { id: products.id } } measures { n: count(products.id) } }"
		total = "explore { dimensions { rows { rollup(name: products.name) } columns { id: products.id } } measures { n: count(products.id) } }"
	)
	page := func(n, size int) map[string]any { return map[string]any{"page": n, "pageSize": size} }

	for _, tc := range []struct {
		name  string
		query string
		page  map[string]any
		check func(t *testing.T, s streamedPage)
	}{
		{"a query's page, more after it", names, page(1, 2), func(t *testing.T, s streamedPage) {
			if len(s.rows) != 2 || !s.summary.HasMore || s.summary.Pivot != nil {
				t.Errorf("%d rows, more %v, pivot %+v", len(s.rows), s.summary.HasMore, s.summary.Pivot)
			}
		}},
		{"a query's last page", names, page(2, 2), func(t *testing.T, s streamedPage) {
			if len(s.rows) != 1 || s.summary.HasMore {
				t.Errorf("%d rows, more %v", len(s.rows), s.summary.HasMore)
			}
		}},
		{"a query's page ending the result", names, page(1, 3), func(t *testing.T, s streamedPage) {
			if len(s.rows) != 3 || s.summary.HasMore {
				t.Errorf("%d rows, more %v", len(s.rows), s.summary.HasMore)
			}
		}},
		{"a pivot's page", pivot, page(1, 2), func(t *testing.T, s streamedPage) {
			if p := s.summary.Pivot; p == nil || *p != (querycmd.PivotPage{Rows: 3, Columns: 3}) || !s.summary.HasMore {
				t.Errorf("pivot %+v, more %v", p, s.summary.HasMore)
			}
			if got := s.values("__pivot_row__"); slices.Max(got) != "2" || !slices.Contains(got, "0") {
				t.Errorf("grid rows %v, want the header rows (0) and rows 1 and 2", got)
			}
		}},
		{"a pivot's last page", pivot, page(2, 2), func(t *testing.T, s streamedPage) {
			if got := s.values("__pivot_row__"); s.summary.HasMore || !slices.Contains(got, "3") || slices.Contains(got, "2") {
				t.Errorf("grid rows %v, more %v", got, s.summary.HasMore)
			}
		}},
		{"a page of a pivot's columns", pivot, map[string]any{"page": 1, "pageSize": 3, "columns": page(1, 2)}, func(t *testing.T, s streamedPage) {
			if p := s.summary.Pivot; p == nil || !p.HasMoreColumns || p.Columns != 3 {
				t.Errorf("pivot %+v", p)
			}
			// The grid is a diagonal (each product, its own id): a grid row whose
			// cell is on another page of columns has a placeholder, column 0.
			if got := s.values("__pivot_column__"); !slices.Contains(got, "0") || slices.Contains(got, "3") {
				t.Errorf("columns %v, want a placeholder (0), and no column 3", got)
			}
		}},
		{"a rolled-up pivot's later page, its grand total on it", total, page(2, 2), func(t *testing.T, s streamedPage) {
			if p := s.summary.Pivot; p == nil || p.Rows != 3 {
				t.Errorf("pivot %+v, want 3 rows, the grand total not counted", p)
			}
			if got := s.values("__pivot_row__"); !slices.Contains(got, "1") || !slices.Contains(got, "4") {
				t.Errorf("grid rows %v, want the grand total (1) and the page's last row (4)", got)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"dataset": "ecommerce", "query": tc.query, "input": map[string]any{"pagination": tc.page}, "cache": map[string]any{"refresh": true}}
			miss := streamQuery(ctx, t, cc, args)
			if miss.summary.Cache.Hit || miss.summary.Cache.CachedAt.IsZero() {
				t.Fatalf("not cached: %+v", miss.summary.Cache)
			}
			for _, f := range miss.fields {
				if strings.HasPrefix(f, "__pivot_row_count") || strings.HasPrefix(f, "__pivot_column_count") {
					t.Errorf("a count column is a field: %v", miss.fields)
				}
			}
			tc.check(t, miss)

			delete(args, "cache")
			hit := streamQuery(ctx, t, cc, args)
			if !hit.summary.Cache.Hit {
				t.Fatalf("not a hit: %+v", hit.summary.Cache)
			}
			if !slices.Equal(hit.fields, miss.fields) || !slices.Equal(hit.rows, miss.rows) || hit.summary.HasMore != miss.summary.HasMore ||
				(hit.summary.Pivot == nil) != (miss.summary.Pivot == nil) || hit.summary.Pivot != nil && *hit.summary.Pivot != *miss.summary.Pivot {
				t.Errorf("the hit differs:\n miss %v %v %+v\n hit  %v %v %+v", miss.fields, miss.rows, miss.summary.Pivot, hit.fields, hit.rows, hit.summary.Pivot)
			}
			tc.check(t, hit)
		})
	}
}

// pagingContext is a command context with real sidecars and a real query
// cache: anfra-node, canal-lake, canal-query writing to it, and the local
// query cache over a store in a temp folder, caching every query for an hour.
func pagingContext(ctx context.Context, t *testing.T, r repo.Repo) command.CommandContext {
	t.Helper()
	local, _ := r.Local()
	cfg := sidecar.Config{RepoID: local.ID, CompileCachePath: filepath.Join(t.TempDir(), "compile-cache")}
	dir := filepath.Join(t.TempDir(), "query")
	lake, err := canallake.New(cfg, localquerycache.LakeDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := lake.Start(ctx); err != nil {
		t.Fatalf("start canal-lake: %v", err)
	}
	t.Cleanup(func() { lake.Close(context.WithoutCancel(ctx)) })
	cfg.CanalLakeSocket = lake.SocketPath()
	node := anfranode.New(cfg)
	if err := node.Start(ctx); err != nil {
		t.Fatalf("start anfra-node: %v", err)
	}
	t.Cleanup(func() { node.Close(context.WithoutCancel(ctx)) })
	canal := canalquery.New(cfg)
	if err := canal.Start(ctx); err != nil {
		t.Fatalf("start canal-query: %v", err)
	}
	t.Cleanup(func() { canal.Close(context.WithoutCancel(ctx)) })
	qc := localquerycache.New(dir, localquerycache.Options{Lake: lake, Default: command.QueryCacheDefaults{Duration: time.Hour}})
	return command.CommandContext{Clients: command.Clients{Node: node.Client(), CanalQuery: canal.Client()}, TenantID: "local", Repo: r,
		AllowLocalRepo: true, DataPerms: dataperm.Ungoverned(), QueryCache: qc}
}

// streamedPage is a query's stream, as the SDK reads it: its fields, its rows
// as JSON, and its summary.
type streamedPage struct {
	fields  []string
	rows    []string
	summary querycmd.QuerySummary
}

// values are a column's values in each row, as text.
func (s streamedPage) values(field string) []string {
	i := slices.Index(s.fields, field)
	var out []string
	for _, r := range s.rows {
		var row []jsontext.Value
		_ = jsonkit.Unmarshal([]byte(r), &row)
		v := string(row[i])
		if row[i].Kind() == '"' {
			_ = jsonkit.Unmarshal(row[i], &v)
		}
		if strings.Contains(v, ".") { // a decimal, such as 1.000000000: as its integer
			v = strings.TrimSuffix(strings.TrimRight(v, "0"), ".")
		}
		out = append(out, v)
	}
	return out
}

// streamQuery runs query streamed, as the core API streams it to the SDK.
func streamQuery(ctx context.Context, t *testing.T, cc command.CommandContext, args map[string]any) streamedPage {
	t.Helper()
	reg, rt := dispatch()
	o, _ := reg.Lookup(command.OpName("query"))
	raw, err := jsonkit.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var s streamedPage
	summarized := false
	err = o.InvokeStream(ctx, rt, cc, raw, func(e any) error {
		switch ev := e.(type) {
		case querycmd.QueryHeader:
			s.fields = ev.Fields
		case querycmd.QueryRecords:
			for _, r := range ev.Records {
				s.rows = append(s.rows, string(r))
			}
		case querycmd.QuerySummary:
			s.summary, summarized = ev, true
		}
		return nil
	})
	if err != nil || !summarized {
		t.Fatalf("stream: %v, summarized %v", err, summarized)
	}
	return s
}
