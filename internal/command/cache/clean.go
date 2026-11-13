// Package cache is the commands on the repo's caches: cache.status reports
// what they hold, and cache.clean removes their entries. The query cache is
// the only one yet. See docs/designs/query-cache.md#cleaning.
package cache

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/shared/apperr"
)

// Clean removes entries from the repo's query cache.
var Clean = command.Define(command.Def[CacheCleanRequestParams, CacheCleanResult]{
	Name:  "cache.clean",
	Short: "Remove results from the repo's query cache, so that queries run on the warehouse again",
	Long: `Remove results from the repo's query cache, so that queries run on the warehouse again.

Clean after fixing data in the warehouse: from then on, no query is answered from a result read before the clean, including one still being written when it ran. Select what to remove by what the queries read, such as --query.data-source or --query.dataset. Without any, every cache of the repo is cleaned: today, every result of its query cache.`,
	Idempotent: true,
	Timeout:    time.Minute,
	Errors:     []apperr.AnyCode{errcode.CacheDisabled, errcode.CacheUnavailable},
	Run:        clean,
})

// CacheCleanRequestParams is what cache.clean takes: which of each cache's
// entries to remove.
type CacheCleanRequestParams struct {
	Query *QueryCacheSelector `json:"query,omitempty" doc:"the query cache's entries to remove, by what their queries read. Every field given must match, and with none, every entry of the query cache is removed. Without query, every cache of the repo is cleaned, which today is the query cache alone."`
}

func (CacheCleanRequestParams) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[CacheCleanRequestParams](s)
}

// QueryCacheSelector selects the query cache's entries by their tags
// (docs/designs/query-cache.md#tags).
type QueryCacheSelector struct {
	DataSource string `json:"data_source,omitempty" doc:"entries of queries run on this data source"`
	Dataset    string `json:"dataset,omitempty" doc:"entries of queries run against this dataset"`
}

// tags are the selector's tags, all of which an entry must have: none for a
// nil selector, every entry.
func (s *QueryCacheSelector) tags() []querycache.Tag {
	var tags []querycache.Tag
	if s == nil {
		return nil
	}
	if s.DataSource != "" {
		tags = append(tags, querycache.Tag{Kind: querycache.TagDataSource, Value: s.DataSource})
	}
	if s.Dataset != "" {
		tags = append(tags, querycache.Tag{Kind: querycache.TagDataset, Value: s.Dataset})
	}
	return tags
}

// CacheCleanResult is what cache.clean removed, from each cache it cleaned.
type CacheCleanResult struct {
	Query *QueryCacheCleaned `json:"query,omitempty" doc:"what was removed from the query cache"`
}

// QueryCacheCleaned is what a clean removed from the query cache.
type QueryCacheCleaned struct {
	Entries int   `json:"entries" doc:"how many cached results were removed. From now on, no query is answered from a result read before the clean, including one still being written when it ran."`
	Bytes   int64 `json:"bytes" doc:"their size, in bytes. Their files are deleted by the next sweep."`
}

func clean(ctx context.Context, cc command.CommandContext, in CacheCleanRequestParams) (CacheCleanResult, error) {
	qc := cc.QueryCache
	if qc == nil {
		return CacheCleanResult{}, apperr.New(errcode.CacheDisabled, "This server keeps no query cache, so there's nothing to clean.")
	}
	if off := qc.Settings().Disabled; off != "" {
		return CacheCleanResult{}, apperr.New(errcode.CacheDisabled, "The query cache is turned off, so there's nothing to clean. "+off)
	}
	m, err := qc.Manager(ctx)
	if err != nil {
		qc.Report(ctx, err)
		return CacheCleanResult{}, apperr.Encapsulate(err, errcode.CacheUnavailable,
			"The query cache isn't working, so it couldn't be cleaned. anfra's log says why, and anfra status shows the cache's state.")
	}
	expired, err := m.Clean(ctx, in.Query.tags())
	if err != nil {
		qc.Report(ctx, err)
		if errors.Is(err, context.Canceled) {
			return CacheCleanResult{}, err
		}
		return CacheCleanResult{}, apperr.Encapsulate(err, errcode.CacheUnavailable,
			"The query cache isn't working, so it couldn't be cleaned. anfra's log says why, and anfra status shows the cache's state.")
	}
	return CacheCleanResult{Query: &QueryCacheCleaned{Entries: expired.Entries, Bytes: expired.Bytes}}, nil
}
