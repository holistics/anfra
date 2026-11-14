package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// The answer's warnings, when a query that asked for the cache ran without it
// (docs/designs/query-cache.md#a-querys-answer).
const (
	cacheUnavailable = "cache_unavailable"
	cacheDisabled    = "cache_disabled"
)

// unavailableMessage is the cache_unavailable warning's, and failure's, word
// on where to look.
const unavailableMessage = "anfra's log says why, and anfra status shows the cache's state."

// cacheLookup is a query looked up in the query cache, before it runs: whether
// the query uses the cache, its key, the cache it was looked up in, and the
// entry it's answered from, if any; or why it runs without the cache. A
// value: each step that changes it returns a copy.
type cacheLookup struct {
	used       bool // the query uses the cache
	cachedOnly bool
	readOnly   bool
	duration   time.Duration
	key        querycache.Key    // "" when none could be made
	cache      *openCache        // the cache it was looked up in; nil when it can't be used
	hit        *querycache.Entry // the entry the query is answered from
	warning    CacheWarning      // why the query runs without the cache, if it does
}

// openCache is the repo's query cache, opened for a query: what the query
// reports the cache's failures to and starts canal-lake through, and the
// manager its lookup and write go through.
type openCache struct {
	command.QueryCache
	m *querycache.Manager
}

// cacheWrite is how a miss's write of its result ended: committed, or a
// warning when the cache failed it. Zero when nothing was written.
type cacheWrite struct {
	committed *querycache.Entry
	warning   CacheWarning
}

// runCached streams st's rows to h, within bounds, through the query cache,
// and answers what the cache did: zero when the query doesn't use it. A cache
// problem never fails a query, but a query asking for the cache only
// (cache.cached_only) never runs on the warehouse, so it fails instead:
//
//	When the query                         It                                                  With cache.cached_only
//	sets no cache.duration, nor the repo   runs on the warehouse, with no cache in its answer  fails: invalid_request
//	asks for the cache, and there is none  the same                                            fails: cache_disabled
//	asks for it, and it's turned off       runs on the warehouse, warning cache_disabled       fails: cache_disabled
//	asks for it, and it isn't working      runs on the warehouse, warning cache_unavailable    fails: cache_unavailable
//	asks for it, and an entry fits         is answered from the entry: a hit                   the same
//	asks for it, and none fits             runs on the warehouse, and its result is cached     fails: cache_miss
//
// An entry whose file is gone (swept, or deleted by hand) is a miss, found
// before any of its rows is answered. t is told when the request goes to
// canal-query: not for a query refused before it.
func (st statement) runCached(ctx context.Context, r queryRun, t *tracker, canal *canalquery.Client, bounds query.ResultBounds, h canalquery.Handler) (CacheResult, error) {
	l, err := r.lookUpInCache(ctx, st, bounds.Truncate)
	if err != nil {
		return CacheResult{}, err
	}
	t.requested()
	if l.hit != nil {
		answered, err := readCacheEntry(ctx, canal, *l.hit, bounds, h)
		if !cacheFileGone(err, answered) {
			return newCacheResult(l, cacheWrite{}), err
		}
		if l.cachedOnly {
			return CacheResult{}, cacheMissError()
		}
		l.hit = nil // its file is gone: a miss
	}
	w, err := runAndCache(ctx, l, st, canal, bounds, h)
	return newCacheResult(l, w), err
}

// newCacheResult is what the cache did for a query, as its answer says it:
// from its lookup, and its miss's write, if any. Zero when the query doesn't use
// the cache.
func newCacheResult(l cacheLookup, w cacheWrite) CacheResult {
	if !l.used {
		return CacheResult{}
	}
	r := CacheResult{Key: string(l.key), Hit: l.hit != nil, Warning: l.warning}
	switch {
	case l.hit != nil:
		r.CachedAt, r.ExpiresAt = l.hit.CachedAt, l.hit.ExpiresAt
	case w.committed != nil:
		r.CachedAt, r.ExpiresAt = w.committed.CachedAt, w.committed.ExpiresAt
	case w.warning.Code != "":
		r.Warning = w.warning
	}
	return r
}

// lookUpInCache looks st up in the query cache, before anything runs: whether the
// query uses the cache, its key, the cache's store opened (created or migrated
// on first use), and the entry it's answered from, if any, with canal-lake
// started to read it. It fails only for a query asking for the cache only
// (cache.cached_only) that can't be answered from it (runCached's table).
// truncate is the rows the query asks for, which its key holds.
func (r queryRun) lookUpInCache(ctx context.Context, st statement, truncate int) (cacheLookup, error) {
	opts, qc := r.in.Cache, r.cc.QueryCache
	l := cacheLookup{cachedOnly: opts.CachedOnly, readOnly: opts.ReadOnly}

	// Does the query use the cache?
	if qc == nil {
		return l.uncached(apperr.New(errcode.CacheDisabled, "This server keeps no query cache, so a query with cache.cached_only can't be answered."))
	}
	settings := qc.Settings()
	l.duration = settings.Default.Duration
	if d, set := opts.duration(); set {
		l.duration = d
	}
	if l.duration <= 0 {
		return l.uncached(command.InvalidArg("cache.cached_only", "invalid", "cache.cached_only needs a cache.duration, because the repo sets no default (cache.query.default.duration)."))
	}
	l.used = true

	// Can it?
	if settings.Disabled != "" {
		return l.disabled(settings.Disabled)
	}
	key, err := cacheKeyOf(r.cc.TenantID, st, truncate)
	if err != nil {
		return l.unavailable(ctx, qc, err)
	}
	l.key = key
	m, err := qc.Manager(ctx)
	if err != nil {
		return l.unavailable(ctx, qc, err)
	}
	l.cache = &openCache{QueryCache: qc, m: m}

	// Is its result cached?
	if opts.Refresh {
		return l, nil
	}
	e, ok, err := l.cache.m.Lookup(ctx, l.key, l.duration)
	if err != nil {
		return l.unavailable(ctx, qc, fmt.Errorf("look the query up in the cache: %w", err))
	}
	if !ok {
		if l.cachedOnly {
			return cacheLookup{}, cacheMissError()
		}
		return l, nil
	}
	if err := qc.StartLake(ctx); err != nil {
		return l.unavailable(ctx, qc, fmt.Errorf("start canal-lake: %w", err))
	}
	l.hit = &e
	return l, nil
}

// cacheKeyOf is st's key in the query cache, for tenant. truncate is the rows the
// query asks for.
func cacheKeyOf(tenant string, st statement, truncate int) (querycache.Key, error) {
	in := querycache.KeyInput{
		Tenant: tenant, DataSource: st.query.Identity(), DBType: st.query.DBType(), TruncateRows: truncate,
	}
	if st.aql != "" {
		in.CompilerKey = st.compilerKey // none from an anfra-node without one: no key
	} else {
		in.SQL = st.query.SQL()
	}
	key, err := querycache.NewKey(in)
	if err != nil {
		return "", fmt.Errorf("make the query's cache key: %w", err)
	}
	return key, nil
}

// uncached is l for a query that doesn't use the cache: without a cache in
// its answer, or failing with refusal when it asked for the cache only.
func (l cacheLookup) uncached(refusal error) (cacheLookup, error) {
	if l.cachedOnly {
		return cacheLookup{}, refusal
	}
	return l, nil
}

// disabled is l for a query that asked for the cache, which reason turns off:
// run with the cache_disabled warning, or failing for cache.cached_only.
func (l cacheLookup) disabled(reason string) (cacheLookup, error) {
	if l.cachedOnly {
		return cacheLookup{}, apperr.New(errcode.CacheDisabled, "The query cache is turned off, so a query with cache.cached_only can't be answered. "+reason)
	}
	l.warning = CacheWarning{Code: cacheDisabled, Message: "The query ran without the cache, because it's turned off. " + reason}
	return l, nil
}

// cacheMissError is cache.cached_only's failure when the cache holds no result the
// query accepts.
func cacheMissError() error {
	return apperr.New(errcode.CacheMiss, "The query's result isn't cached, or is older than its cache.duration accepts. Run it without cache.cached_only to get its rows from the warehouse.")
}

// unavailable is l for a query whose cache, qc, isn't working, as err says:
// run with the cache_unavailable warning, or failing for cache.cached_only.
// qc is told err either way.
func (l cacheLookup) unavailable(ctx context.Context, qc command.QueryCache, err error) (cacheLookup, error) {
	qc.Report(ctx, err)
	if l.cachedOnly {
		return cacheLookup{}, apperr.Encapsulate(err, errcode.CacheUnavailable, "The query cache isn't working, so a query with cache.cached_only can't be answered. "+unavailableMessage)
	}
	l.cache, l.hit = nil, nil
	l.warning = unavailableWarning
	return l, nil
}

// unavailableWarning is the warning of a query that ran without the cache
// because the cache isn't working.
var unavailableWarning = CacheWarning{Code: cacheUnavailable,
	Message: "The query ran without the cache, because the cache isn't working. " + unavailableMessage}

// readCacheEntry streams the hit e to h through canal-query, within bounds, the
// same way a warehouse result is streamed (query.CacheReadQuery). The rows come
// from the cache file. The header is the one the miss returned, stored with the
// entry, because canal-lake's header differs from it in three ways. canal-lake
// names the columns hcol0, hcol1 and so on, gives each column the type DuckDB
// reads from the file, and labels a timestamp with its own time zone (Etc/UTC
// where the miss said UTC). answered says whether the header reached h.
func readCacheEntry(ctx context.Context, canal *canalquery.Client, e querycache.Entry, bounds query.ResultBounds, h canalquery.Handler) (answered bool, err error) {
	var stored canalquery.Header
	_ = jsonkit.Unmarshal(e.Columns, &stored)
	header := h.Header
	h.Header = func(hd canalquery.Header) error {
		answered = true
		if len(stored.Fields) == len(hd.Fields) {
			hd.Fields = stored.Fields
		}
		if len(stored.Types) == len(hd.Fields) {
			hd.Types = stored.Types
		}
		if len(stored.ArrowTypes) == len(hd.Fields) {
			hd.ArrowTypes = stored.ArrowTypes
		}
		return header(hd)
	}
	err = query.CacheReadQuery(e.LakeKey()).Stream(ctx, canal, bounds, h)
	return answered, err
}

// cacheFileGone reports err as a hit whose file canal-lake doesn't have, found before
// anything was answered: swept, deleted by hand, or the store pointed at
// another storage. The query is then a miss.
func cacheFileGone(err error, answered bool) bool {
	e, ok := errors.AsType[*canalquery.Error](err)
	return ok && e.Type == canalquery.TypeLakeFileGone && !answered
}

// runAndCache runs st on the warehouse, streaming its rows to h within bounds,
// and, unless l says not to, writes its result to the cache as it streams:
// committed when the query succeeds, abandoned when it fails, so it is never
// used. A cache failure leaves the query to run, its write's warning saying
// so.
func runAndCache(ctx context.Context, l cacheLookup, st statement, canal *canalquery.Client, bounds query.ResultBounds, h canalquery.Handler) (cacheWrite, error) {
	c := l.cache
	if c == nil || l.readOnly {
		return cacheWrite{}, st.query.Stream(ctx, canal, bounds, h)
	}
	if err := c.StartLake(ctx); err != nil {
		c.Report(ctx, fmt.Errorf("start canal-lake: %w", err))
		return cacheWrite{warning: unavailableWarning}, st.query.Stream(ctx, canal, bounds, h)
	}
	w, err := c.m.Begin(ctx, l.key, st.tags(), st.aql, st.query.SQL())
	if err != nil {
		c.Report(ctx, fmt.Errorf("begin the query's cache write: %w", err))
		return cacheWrite{warning: unavailableWarning}, st.query.Stream(ctx, canal, bounds, h)
	}

	// What the commit needs, recorded as the result streams.
	var (
		header      []byte
		rows, bytes int64
	)
	onHeader, onRow := h.Header, h.Row
	h.Header = func(hd canalquery.Header) error {
		b, err := jsonkit.Marshal(hd)
		if err != nil {
			return err
		}
		header = b
		return onHeader(hd)
	}
	h.Row = func(r canalquery.Row) error {
		rows++
		bytes += int64(len(r))
		return onRow(r)
	}
	if err := st.query.WithLakeKey(w.LakeKey()).Stream(ctx, canal, bounds, h); err != nil {
		if aerr := w.Abandon(ctx); aerr != nil {
			c.Report(ctx, fmt.Errorf("abandon the failed query's cache write: %w", aerr))
		}
		return cacheWrite{}, err
	}
	e, err := w.Commit(ctx, rows, bytes, header, l.duration)
	if err != nil {
		if errors.Is(err, querycache.ErrNotPending) {
			err = fmt.Errorf("the write's entry was swept before it committed: %w", err)
		}
		c.Report(ctx, fmt.Errorf("commit the query's cache write: %w", err))
		return cacheWrite{warning: unavailableWarning}, nil
	}
	return cacheWrite{committed: &e}, nil
}
