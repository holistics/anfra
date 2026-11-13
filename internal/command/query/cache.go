package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
)

// The answer's warnings, when a query that asked for the cache ran without it
// (docs/designs/query-cache.md#a-querys-answer).
const (
	cacheUnavailable = "cache_unavailable"
	cacheDisabled    = "cache_disabled"
)

// cachedRun is a query's run through the query cache: on a miss, its result
// written to canal-lake as it streams, and committed once it has.
// docs/designs/query-cache.md#a-miss.
type cachedRun struct {
	qc       command.QueryCache
	duration time.Duration
	result   *CacheResult // nil: the query doesn't use the cache
	write    *querycache.Write

	header []byte
	rows   int64
	bytes  int64
}

// keyInput is what the query's cache key is made of, besides the tenant and
// what the prepared statement gives: anfra-node's key for AQL, the text for
// SQL, and the rows the run asks canal-query for; and the dataset an AQL
// query ran against, for its tags.
type keyInput struct {
	compilerKey, sql string
	truncate         int
	dataset          string
}

// tags are what a clean selects the query's entries by: its data source, and
// its dataset (docs/designs/query-cache.md#tags). The models it read come
// when anfra-node says which they are.
func (k keyInput) tags(p query.Prepared) []querycache.Tag {
	tags := []querycache.Tag{{Kind: querycache.TagDataSource, Value: p.DataSource()}}
	if k.dataset != "" {
		tags = append(tags, querycache.Tag{Kind: querycache.TagDataset, Value: k.dataset})
	}
	return tags
}

// startCache is how the cache answers q, whose statement is prepared, before
// it runs: nothing when the query doesn't use the cache, a warning when it
// asked and can't, or a write begun. A cache problem never fails the query:
// it runs without the cache, and the host is told why (QueryCache.Report).
func (q queryRun) startCache(ctx context.Context, p query.Prepared, k keyInput, aql, sql string) *cachedRun {
	qc := q.cc.QueryCache
	if qc == nil {
		return &cachedRun{}
	}
	settings := qc.Settings()
	duration, set := q.in.Cache.duration()
	if !set {
		duration = settings.Default.Duration
	}
	c := &cachedRun{qc: qc, duration: duration}
	if duration <= 0 {
		return c
	}
	c.result = &CacheResult{}
	if settings.Disabled != "" {
		c.result.Warning = &CacheWarning{Code: cacheDisabled, Message: "The query ran without the cache, because it's turned off. " + settings.Disabled}
		return c
	}
	key, err := querycache.NewKey(querycache.KeyInput{
		CompilerKey: k.compilerKey, SQL: k.sql,
		Tenant: q.cc.TenantID, DataSource: p.Identity(), DBType: p.DBType(), TruncateRows: k.truncate,
	})
	if err != nil {
		return c.unavailable(ctx, fmt.Errorf("make the query's cache key: %w", err))
	}
	c.result.Key = string(key)
	if q.in.Cache.ReadOnly {
		return c // no entry can be read yet, and none is written
	}
	m, err := qc.Manager(ctx)
	if err != nil {
		return c.unavailable(ctx, err)
	}
	if err := qc.StartLake(ctx); err != nil {
		return c.unavailable(ctx, fmt.Errorf("start canal-lake: %w", err))
	}
	c.write, err = m.Begin(ctx, key, k.tags(p), aql, sql)
	if err != nil {
		return c.unavailable(ctx, fmt.Errorf("begin the query's cache write: %w", err))
	}
	return c
}

// unavailable is c with the cache_unavailable warning, the host told why.
func (c *cachedRun) unavailable(ctx context.Context, err error) *cachedRun {
	c.qc.Report(ctx, err)
	c.write = nil
	c.result.Warning = &CacheWarning{Code: cacheUnavailable,
		Message: "The query ran without the cache, because the cache isn't working. anfra's log says why, and anfra status shows the cache's state."}
	return c
}

// prepared is p writing to canal-lake, when c writes the result.
func (c *cachedRun) prepared(p query.Prepared) query.Prepared {
	if c.write == nil {
		return p
	}
	return p.Caching(c.write.LakeKey())
}

// handler is h, the result's header and size kept for the commit.
func (c *cachedRun) handler(h canalquery.Handler) canalquery.Handler {
	if c.write == nil {
		return h
	}
	header, row := h.Header, h.Row
	h.Header = func(hd canalquery.Header) error {
		b, err := jsonkit.Marshal(hd)
		if err != nil {
			return err
		}
		c.header = b
		return header(hd)
	}
	h.Row = func(r canalquery.Row) error {
		c.rows++
		c.bytes += int64(len(r))
		return row(r)
	}
	return h
}

// finish ends the write: committed when the query succeeded, abandoned when it
// didn't, so it is never used. A failed commit leaves the query's answer as
// it is, without the cache.
func (c *cachedRun) finish(ctx context.Context, runErr error) {
	if c.write == nil {
		return
	}
	if runErr != nil {
		if err := c.write.Abandon(ctx); err != nil {
			c.qc.Report(ctx, fmt.Errorf("abandon the failed query's cache write: %w", err))
		}
		return
	}
	e, err := c.write.Commit(ctx, c.rows, c.bytes, c.header, c.duration)
	if err != nil {
		if errors.Is(err, querycache.ErrNotPending) {
			err = fmt.Errorf("the write's entry was swept before it committed: %w", err)
		}
		c.unavailable(ctx, fmt.Errorf("commit the query's cache write: %w", err))
		return
	}
	c.result.CachedAt, c.result.ExpiresAt = &e.CachedAt, &e.ExpiresAt
}
