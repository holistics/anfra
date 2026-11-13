package query

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/querycache"
	sqlitestore "github.com/holistics/anfra/internal/querycache/store/sqlite"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// standInCache is a query cache over a real store, its settings and failures
// set by the test, what it was told kept.
type standInCache struct {
	settings   command.QueryCacheSettings
	store      *sqlitestore.Store
	managerErr error
	lakeErr    error
	reported   []error
}

func (c *standInCache) Summarize(context.Context) querycache.Summary { return querycache.Summary{} }
func (c *standInCache) Settings() command.QueryCacheSettings         { return c.settings }
func (c *standInCache) StartLake(context.Context) error              { return c.lakeErr }
func (c *standInCache) Contents(context.Context) (command.QueryCacheContents, error) {
	return command.QueryCacheContents{}, nil
}
func (c *standInCache) Report(_ context.Context, err error) { c.reported = append(c.reported, err) }
func (c *standInCache) Manager(context.Context) (*querycache.Manager, error) {
	if c.managerErr != nil {
		return nil, c.managerErr
	}
	return querycache.NewManager(querycache.Config{Store: c.store, Namespace: "prod/"})
}

func newStandIn(ctx context.Context, t *testing.T) *standInCache {
	t.Helper()
	s, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "store.sqlite"), sqlitestore.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &standInCache{store: s}
}

// prepared is a SQL statement on a data source of identity.
func prepared(ctx context.Context, t *testing.T, identity string) query.Prepared {
	t.Helper()
	sources := datasource.Set{"w": {Name: "w", DBType: "duckdb", Connection: map[string]any{}, Identity: identity}}
	sql, err := query.ConnectUngoverned(ctx, dataperm.Ungoverned(), sources, "w", "select 1")
	if err != nil {
		t.Fatal(err)
	}
	return sql.Prepared
}

// The cache answers a query by its duration and the repo's settings: not at
// all without one; with a warning, running it uncached, when it can't; and
// otherwise with a write of its result, committed when the query succeeds,
// abandoned when it fails.
func TestStartCache(t *testing.T) {
	ctx := context.Background()
	k := keyInput{sql: "select 1", truncate: 10}
	run := func(qc command.QueryCache, opts CacheOptions) queryRun {
		var cc command.CommandContext
		if qc != nil {
			cc.QueryCache = qc
		}
		cc.TenantID = "acme"
		return queryRun{cc: cc, in: QueryRequestParams{Cache: opts}}
	}

	t.Run("no cache, or no duration, or 0s: no cache in the answer", func(t *testing.T) {
		if c := run(nil, CacheOptions{Duration: "1h"}).startCache(ctx, prepared(ctx, t, "id1"), k, "", "select 1"); c.result != nil || c.write != nil {
			t.Errorf("without a cache: %+v", c)
		}
		qc := newStandIn(ctx, t)
		for _, opts := range []CacheOptions{{}, {Duration: "0s"}} {
			if c := run(qc, opts).startCache(ctx, prepared(ctx, t, "id1"), k, "", "select 1"); c.result != nil || c.write != nil {
				t.Errorf("%+v: %+v", opts, c)
			}
		}
		qc.settings.Default.Duration = time.Hour
		if c := run(qc, CacheOptions{Duration: "0s"}).startCache(ctx, prepared(ctx, t, "id1"), k, "", "select 1"); c.result != nil {
			t.Errorf("0s over the repo's default: %+v", c.result)
		}
	})

	t.Run("turned off: a warning saying by what", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		qc.settings.Disabled = "ANFRA_CACHE_QUERY_ENABLED=0 turns the query cache off for this process."
		c := run(qc, CacheOptions{Duration: "1h"}).startCache(ctx, prepared(ctx, t, "id1"), k, "", "select 1")
		if c.write != nil || c.result.Warning == nil || c.result.Warning.Code != cacheDisabled || !strings.Contains(c.result.Warning.Message, "ANFRA_CACHE_QUERY_ENABLED=0") {
			t.Errorf("%+v", c.result)
		}
	})

	t.Run("no key, or no store: unavailable, the host told why", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		c := run(qc, CacheOptions{Duration: "1h"}).startCache(ctx, prepared(ctx, t, ""), k, "", "select 1")
		if c.write != nil || c.result.Warning == nil || c.result.Warning.Code != cacheUnavailable || len(qc.reported) != 1 || !errors.Is(qc.reported[0], querycache.ErrNoDataSourceIdentity) {
			t.Errorf("no identity: %+v, reported %v", c.result, qc.reported)
		}
		qc = newStandIn(ctx, t)
		qc.managerErr = errors.New("the store is locked")
		c = run(qc, CacheOptions{Duration: "1h"}).startCache(ctx, prepared(ctx, t, "id1"), k, "", "select 1")
		if c.write != nil || c.result.Warning == nil || c.result.Warning.Code != cacheUnavailable || c.result.Key == "" || len(qc.reported) != 1 {
			t.Errorf("no store: %+v, reported %v", c.result, qc.reported)
		}
	})

	t.Run("read only: the key, nothing written", func(t *testing.T) {
		c := run(newStandIn(ctx, t), CacheOptions{Duration: "1h", ReadOnly: true}).startCache(ctx, prepared(ctx, t, "id1"), k, "", "select 1")
		if c.write != nil || c.result == nil || c.result.Key == "" || c.result.Warning != nil {
			t.Errorf("%+v", c.result)
		}
	})

	t.Run("a miss: written under the namespace, committed when it succeeds", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		qc.settings.Default.Duration = time.Hour // the query sets none
		c := run(qc, CacheOptions{}).startCache(ctx, prepared(ctx, t, "id1"), k, "", "select 1")
		if c.write == nil || !strings.HasPrefix(c.write.LakeKey(), "prod/") {
			t.Fatalf("no write under the namespace: %+v", c)
		}
		h := c.handler(canalquery.Handler{Header: func(canalquery.Header) error { return nil }, Row: func(canalquery.Row) error { return nil }})
		_ = h.Header(canalquery.Header{Fields: []string{"a"}})
		_ = h.Row(canalquery.Row(`["1"]`))
		c.finish(ctx, nil)
		if c.result.CachedAt == nil || c.result.ExpiresAt == nil || c.result.ExpiresAt.Sub(*c.result.CachedAt) != time.Hour || c.result.Hit {
			t.Fatalf("the answer: %+v", c.result)
		}
		e, ok, err := qc.store.Lookup(ctx, querycache.Key(c.result.Key), time.Now())
		if err != nil || !ok || e.RowCount != 1 || e.SizeBytes != 5 || !strings.Contains(string(e.Columns), `"a"`) || e.SQL != "select 1" {
			t.Errorf("the entry: %+v, %v, %v", e, ok, err)
		}
	})

	t.Run("a write is tagged with its data source, and an AQL query's dataset", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		aql := keyInput{compilerKey: "v1:abc", truncate: 10, dataset: "sales"}
		c := run(qc, CacheOptions{Duration: "1h"}).startCache(ctx, prepared(ctx, t, "id1"), aql, "q", "select 1")
		c.finish(ctx, nil)
		m, _ := qc.Manager(ctx)
		for _, tag := range []querycache.Tag{{Kind: querycache.TagDataSource, Value: "w"}, {Kind: querycache.TagDataset, Value: "sales"}} {
			if ex, err := m.Clean(ctx, []querycache.Tag{tag, {Kind: querycache.TagDataSource, Value: "w"}}); err != nil || ex.Entries != 1 {
				t.Errorf("clean by %v: %+v, %v", tag, ex, err)
			}
			c = run(qc, CacheOptions{Duration: "1h"}).startCache(ctx, prepared(ctx, t, "id1"), aql, "q", "select 1")
			c.finish(ctx, nil)
		}
		if tags := k.tags(prepared(ctx, t, "id1")); len(tags) != 1 {
			t.Errorf("a SQL query's tags: %v, want its data source only", tags)
		}
	})

	t.Run("a failed query: abandoned, never used", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		c := run(qc, CacheOptions{Duration: "1h"}).startCache(ctx, prepared(ctx, t, "id1"), k, "", "select 1")
		c.finish(ctx, errors.New("the warehouse refused"))
		if _, ok, _ := qc.store.Lookup(ctx, querycache.Key(c.result.Key), time.Now()); ok || c.result.CachedAt != nil || len(qc.reported) != 0 {
			t.Errorf("the abandoned write answers: %v, %+v, reported %v", ok, c.result, qc.reported)
		}
	})
}
