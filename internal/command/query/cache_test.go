package query

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
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

// dataSourceQuery is a SQL query on a data source of identity.
func dataSourceQuery(ctx context.Context, t *testing.T, identity string) query.DataSourceQuery {
	t.Helper()
	sources := datasource.Set{"w": {Name: "w", DBType: "duckdb", Connection: map[string]any{}, Identity: identity}}
	sql, err := query.InDataSourceUngoverned(ctx, dataperm.Ungoverned(), sources, "w", "select 1")
	if err != nil {
		t.Fatal(err)
	}
	return sql.DataSourceQuery
}

// errCode is err's code, "" without one.
func errCode(err error) string {
	if e, ok := errors.AsType[*apperr.Error](err); ok {
		return e.Code.String()
	}
	return ""
}

// sqlStatement is a SQL statement on a data source of identity.
func sqlStatement(ctx context.Context, t *testing.T, identity string) statement {
	t.Helper()
	return statement{query: dataSourceQuery(ctx, t, identity)}
}

// aqlStatement is a compiled AQL statement against the sales dataset.
func aqlStatement(ctx context.Context, t *testing.T, identity string) statement {
	t.Helper()
	return statement{query: dataSourceQuery(ctx, t, identity), aql: "sales | select(x)", compilerKey: "v1:abc", dataset: "sales"}
}

// How the cache answers a query, by its duration and the repo's settings: not
// at all without one; with a warning, running it uncached, when it can't; from
// an entry the age rule accepts; and otherwise with a write of its result,
// committed when the query succeeds, abandoned when it fails.
func TestLookUpAndWrite(t *testing.T) {
	ctx := context.Background()
	run := func(qc command.QueryCache, opts CacheOptions) queryRun {
		var cc command.CommandContext
		if qc != nil {
			cc.QueryCache = qc
		}
		cc.TenantID = "acme"
		return queryRun{cc: cc, in: QueryRequestParams{Cache: opts}}
	}
	canal, _ := canalLakeStub(t, false)
	noop := canalquery.Handler{Header: func(canalquery.Header) error { return nil }, Row: func(canalquery.Row) error { return nil }}
	lookUp := func(t *testing.T, qc command.QueryCache, opts CacheOptions, st statement) cacheLookup {
		t.Helper()
		l, err := run(qc, opts).lookUpInCache(ctx, st, 10)
		if err != nil {
			t.Fatalf("lookUpInCache: %v", err)
		}
		return l
	}
	// write runs st on the warehouse, writing its result as l says.
	write := func(t *testing.T, l cacheLookup, st statement) cacheWrite {
		t.Helper()
		w, err := runAndCache(ctx, l, st, canal, query.Unbounded, noop)
		if err != nil {
			t.Fatalf("runAndCache: %v", err)
		}
		return w
	}
	// cached commits a miss of st, so the next query of st hits.
	cached := func(t *testing.T, qc *standInCache, st statement) {
		t.Helper()
		write(t, lookUp(t, qc, CacheOptions{Duration: "1h"}, st), st)
	}
	st := sqlStatement(ctx, t, "id1")

	t.Run("no cache, or no duration, or 0s: no cache in the answer", func(t *testing.T) {
		if l := lookUp(t, nil, CacheOptions{Duration: "1h"}, st); newCacheResult(l, cacheWrite{}) != (CacheResult{}) {
			t.Errorf("without a cache: %+v", l)
		}
		qc := newStandIn(ctx, t)
		for _, opts := range []CacheOptions{{}, {Duration: "0s"}} {
			if l := lookUp(t, qc, opts, st); newCacheResult(l, cacheWrite{}) != (CacheResult{}) || l.cache != nil {
				t.Errorf("%+v: %+v", opts, l)
			}
		}
		qc.settings.Default.Duration = time.Hour
		if l := lookUp(t, qc, CacheOptions{Duration: "0s"}, st); newCacheResult(l, cacheWrite{}) != (CacheResult{}) {
			t.Errorf("0s over the repo's default: %+v", l)
		}
	})

	t.Run("turned off: a warning saying by what", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		qc.settings.Disabled = "ANFRA_CACHE_QUERY_ENABLED=0 turns the query cache off for this process."
		l := lookUp(t, qc, CacheOptions{Duration: "1h"}, st)
		if a := newCacheResult(l, write(t, l, st)); a.Warning.Code != cacheDisabled || !strings.Contains(a.Warning.Message, "ANFRA_CACHE_QUERY_ENABLED=0") || a.Key != "" {
			t.Errorf("%+v", a)
		}
	})

	t.Run("no key, or no store: unavailable, the host told why", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		l := lookUp(t, qc, CacheOptions{Duration: "1h"}, sqlStatement(ctx, t, ""))
		if a := newCacheResult(l, cacheWrite{}); a.Warning.Code != cacheUnavailable || len(qc.reported) != 1 || !errors.Is(qc.reported[0], querycache.ErrNoDataSourceIdentity) {
			t.Errorf("no identity: %+v, reported %v", a, qc.reported)
		}
		noKey := aqlStatement(ctx, t, "id1")
		noKey.compilerKey = "" // an anfra-node without one: no key, rather than one of its SQL
		qc = newStandIn(ctx, t)
		if l := lookUp(t, qc, CacheOptions{Duration: "1h"}, noKey); l.key != "" || len(qc.reported) != 1 || !errors.Is(qc.reported[0], querycache.ErrNoQuery) {
			t.Errorf("no compiler key: %+v, reported %v", l, qc.reported)
		}
		qc = newStandIn(ctx, t)
		qc.managerErr = errors.New("the store is locked")
		l = lookUp(t, qc, CacheOptions{Duration: "1h"}, st)
		if a := newCacheResult(l, write(t, l, st)); a.Warning.Code != cacheUnavailable || a.Key == "" || len(qc.reported) != 1 {
			t.Errorf("no store: %+v, reported %v", a, qc.reported)
		}
	})

	t.Run("a miss: written under the namespace, committed when it succeeds", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		qc.settings.Default.Duration = time.Hour // the query sets none
		l := lookUp(t, qc, CacheOptions{}, st)
		w := write(t, l, st)
		a := newCacheResult(l, w)
		if w.committed == nil || !strings.HasPrefix(w.committed.LakeKey(), "prod/") || a.Hit || a.CachedAt.IsZero() || a.ExpiresAt.Sub(a.CachedAt) != time.Hour {
			t.Fatalf("the answer: %+v, the write %+v", a, w)
		}
		e, ok, err := qc.store.Lookup(ctx, querycache.Key(a.Key), time.Now())
		if err != nil || !ok || e.RowCount != 1 || e.SizeBytes != 3 || !strings.Contains(string(e.Columns), `"a"`) || e.SQL != "select 1" {
			t.Errorf("the entry: %+v, %v, %v", e, ok, err)
		}
		if !e.CachedAt.Equal(a.CachedAt) || !e.ExpiresAt.Equal(a.ExpiresAt) {
			t.Errorf("a miss answers %v to %v, and a lookup %v to %v: want the same times", a.CachedAt, a.ExpiresAt, e.CachedAt, e.ExpiresAt)
		}
	})

	t.Run("a hit: the entry, unless refreshed, or older than the query accepts", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		cached(t, qc, st)
		if l := lookUp(t, qc, CacheOptions{Duration: "1h"}, st); l.hit == nil || !newCacheResult(l, cacheWrite{}).Hit {
			t.Fatalf("a hit: %+v", l)
		}
		if l := lookUp(t, qc, CacheOptions{Duration: "1h", ReadOnly: true}, st); l.hit == nil {
			t.Errorf("read only reads: %+v", l)
		}
		if l := lookUp(t, qc, CacheOptions{Duration: "1h", Refresh: true}, st); l.hit != nil {
			t.Errorf("refresh: %+v", l)
		}
		time.Sleep(2 * time.Millisecond) // times are whole milliseconds: the entry's age, 2ms or more
		if l := lookUp(t, qc, CacheOptions{Duration: "1ms"}, st); l.hit != nil {
			t.Errorf("an entry older than the query accepts: %+v", l)
		}
		if l := lookUp(t, qc, CacheOptions{Duration: "1h"}, sqlStatement(ctx, t, "id2")); l.hit != nil {
			t.Errorf("another identity read the entry: %+v", l)
		}
	})

	t.Run("read only: the key, nothing written", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		l := lookUp(t, qc, CacheOptions{Duration: "1h", ReadOnly: true}, st)
		if w := write(t, l, st); w.committed != nil || newCacheResult(l, w).Key == "" {
			t.Errorf("%+v", w)
		}
	})

	t.Run("a hit's header: the miss's types and names, the entry read", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		cached(t, qc, st)
		l := lookUp(t, qc, CacheOptions{Duration: "1h"}, st)
		var got canalquery.Header
		reader, sent := canalLakeStub(t, false)
		answered, err := readCacheEntry(ctx, reader, *l.hit, query.Unbounded, canalquery.Handler{Header: func(h canalquery.Header) error { got = h; return nil }, Row: func(canalquery.Row) error { return nil }})
		if err != nil || !answered || len(got.Types) != 1 || got.Types[0] != "NUMERIC" || len(got.Fields) != 1 || got.Fields[0] != "a" {
			t.Fatalf("header %+v, %v", got, err)
		}
		entries, _ := (*sent)["dbconfig"].(map[string]any)["entries"].(map[string]any)
		if (*sent)["dbtype"] != "canal_lake" || (*sent)["sql"] != "SELECT * FROM entry" || entries["entry"] != l.hit.LakeKey() || (*sent)["skip_cache"] != true {
			t.Errorf("sent %v", *sent)
		}
	})

	t.Run("cached only: the entry, or a failure saying why, never the warehouse", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		only := CacheOptions{Duration: "1h", CachedOnly: true}
		if _, err := run(qc, only).lookUpInCache(ctx, st, 10); errCode(err) != "cache_miss" {
			t.Errorf("nothing cached: %v", err)
		}
		cached(t, qc, st)
		if l, err := run(qc, only).lookUpInCache(ctx, st, 10); err != nil || l.hit == nil {
			t.Errorf("cached: %+v, %v", l, err)
		}
		if _, err := run(qc, CacheOptions{CachedOnly: true}).lookUpInCache(ctx, st, 10); field(err) != "cache.cached_only" {
			t.Errorf("no duration, here or the repo's: %v", err)
		}
		if _, err := run(nil, only).lookUpInCache(ctx, st, 10); errCode(err) != "cache_disabled" {
			t.Errorf("no cache: %v", err)
		}
		qc.managerErr = errors.New("the store is locked")
		if _, err := run(qc, only).lookUpInCache(ctx, st, 10); errCode(err) != "cache_unavailable" {
			t.Errorf("not working: %v", err)
		}
		qc.managerErr, qc.settings.Disabled = nil, "turned off"
		if _, err := run(qc, only).lookUpInCache(ctx, st, 10); errCode(err) != "cache_disabled" {
			t.Errorf("turned off: %v", err)
		}
	})

	t.Run("a write is tagged with its data source, and an AQL query's dataset", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		aql := aqlStatement(ctx, t, "id1")
		refresh := CacheOptions{Duration: "1h", Refresh: true}
		write(t, lookUp(t, qc, refresh, aql), aql)
		m, _ := qc.Manager(ctx)
		for _, tag := range []querycache.Tag{{Kind: querycache.TagDataSource, Value: "w"}, {Kind: querycache.TagDataset, Value: "sales"}} {
			if ex, err := m.Clean(ctx, []querycache.Tag{tag, {Kind: querycache.TagDataSource, Value: "w"}}); err != nil || ex.Entries != 1 {
				t.Errorf("clean by %v: %+v, %v", tag, ex, err)
			}
			write(t, lookUp(t, qc, refresh, aql), aql)
		}
		if tags := st.tags(); len(tags) != 1 {
			t.Errorf("a SQL query's tags: %v, want its data source only", tags)
		}
	})

	t.Run("a failed query: abandoned, never used", func(t *testing.T) {
		qc := newStandIn(ctx, t)
		l := lookUp(t, qc, CacheOptions{Duration: "1h"}, st)
		failing, _ := canalFailing(t)
		w, err := runAndCache(ctx, l, st, failing, query.Unbounded, noop)
		if _, ok, _ := qc.store.Lookup(ctx, l.key, time.Now()); err == nil || ok || w.committed != nil || len(qc.reported) != 0 {
			t.Errorf("the abandoned write answers: %v, %+v, %v, reported %v", ok, w, err, qc.reported)
		}
	})
}

// A hit whose entry's file is gone runs on the warehouse, and caches its
// result again; with cache.cached_only, it fails with cache_miss.
func TestRunCachedFileGone(t *testing.T) {
	ctx := context.Background()
	qc := newStandIn(ctx, t)
	st := sqlStatement(ctx, t, "id1")
	q := func(opts CacheOptions) queryRun {
		return queryRun{cc: command.CommandContext{QueryCache: qc, TenantID: "acme"}, in: QueryRequestParams{Cache: opts}}
	}
	canal, _ := canalLakeStub(t, true)
	noop := canalquery.Handler{Header: func(canalquery.Header) error { return nil }, Row: func(canalquery.Row) error { return nil }}
	run := func(opts CacheOptions) (CacheResult, error) {
		return st.runCached(ctx, q(opts), newTracker(sink{status: func(string) error { return nil }}), canal, query.Unbounded, noop)
	}

	first, err := run(CacheOptions{Duration: "1h"})
	if err != nil || first.Hit || first.CachedAt.IsZero() {
		t.Fatalf("a miss, cached: %+v, %v", first, err)
	}
	again, err := run(CacheOptions{Duration: "1h"}) // a hit, its file gone: canal-lake says so
	if err != nil || again.Hit || again.CachedAt.IsZero() || !again.CachedAt.After(first.CachedAt) {
		t.Errorf("the gone file read as a miss, cached again: %+v, %v", again, err)
	}
	if _, err := run(CacheOptions{Duration: "1h", CachedOnly: true}); errCode(err) != "cache_miss" {
		t.Errorf("cached only, its file gone: %v", err)
	}
}

// canalLakeStub is a canal-query: a read of canal-lake answers one column,
// named and typed as canal-lake gives it back, and one row, or, gone, fails
// as a file canal-lake doesn't have; a warehouse query answers one column and
// one row. What it was last sent is kept.
func canalLakeStub(t *testing.T, gone bool) (*canalquery.Client, *map[string]any) {
	t.Helper()
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = jsonkit.Unmarshal(body, &sent)
		switch {
		case sent["dbtype"] == "canal_lake" && gone:
			_, _ = w.Write([]byte(`{"__holistics_trailer__":true,"metadata":{},"http_code":422,"error":{"type":"QueryLocationNotFoundError","message":"query location not found"}}` + "\n"))
		case sent["dbtype"] == "canal_lake":
			_, _ = w.Write([]byte(`{"__holistics_header__":true,"fields":["hcol0"],"types":["DECIMAL(18,2)"],"arrow_types":[{"type":"decimal","precision":18,"scale":2}]}` + "\n" +
				`["1.00"]` + "\n" + `{"__holistics_trailer__":true,"metadata":{},"http_code":200}` + "\n"))
		default:
			_, _ = w.Write([]byte(`{"__holistics_header__":true,"fields":["a"],"types":["NUMERIC"],"arrow_types":[{"type":"decimal","precision":18,"scale":2}]}` + "\n" +
				`[1]` + "\n" + `{"__holistics_trailer__":true,"metadata":{},"http_code":200}` + "\n"))
		}
	}))
	t.Cleanup(srv.Close)
	return canalquery.NewClient(srv.URL, false).ForTenant("acme"), &sent
}

// canalFailing is a canal-query that fails every query, as a warehouse
// refusing it.
func canalFailing(t *testing.T) (*canalquery.Client, *map[string]any) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"__holistics_trailer__":true,"metadata":{},"http_code":422,"error":{"type":"User","message":"relation does not exist"}}` + "\n"))
	}))
	t.Cleanup(srv.Close)
	return canalquery.NewClient(srv.URL, false).ForTenant("acme"), nil
}
