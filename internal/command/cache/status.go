package cache

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/querycache"
)

// Status reports what the repo's caches hold, without changing them.
var Status = command.Define(command.Def[CacheStatusRequestParams, CacheStatusResult]{
	Name:  "cache.status",
	Short: "Show what the repo's caches hold, and where",
	Long: `Show what the repo's caches hold, and where: for the query cache, its state, its folder, its settings, and how many results it holds and their size.

It reads the caches without changing them. anfra status shows the same state, without the counts.`,
	ReadOnly:   true,
	Idempotent: true,
	Timeout:    time.Minute,
	Run:        status,
})

// CacheStatusRequestParams is what cache.status takes: nothing yet. Every
// cache is reported, which today is the query cache alone.
type CacheStatusRequestParams struct{}

func (CacheStatusRequestParams) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[CacheStatusRequestParams](s)
}

// CacheStatusResult is what each of the repo's caches holds.
type CacheStatusResult struct {
	Query *QueryCacheStatus `json:"query,omitempty" doc:"the query cache. Absent when the server keeps none."`
	Hints []string          `json:"hints,omitempty" doc:"what to do about a problem the caches have, if any"`
}

// QueryCacheStatus is the query cache's state, and what it holds when it can
// be read.
type QueryCacheStatus struct {
	State    querycache.State    `json:"state" enum:"ready,incompatible,unavailable,disabled" doc:"\"ready\": the cache can be used. \"incompatible\": a newer anfra changed the cache's store in a way this anfra can't read. \"unavailable\": the cache's store can't be used, as \"problem\" says. \"disabled\": the query cache is turned off, as \"reason\" says."`
	Folder   string              `json:"folder,omitempty" doc:"the cache's folder on this machine: its store, and the results of the canal-lake anfra starts. Absent when the cache is disabled, or the server keeps it elsewhere."`
	Problem  string              `json:"problem,omitempty" doc:"what's wrong with the cache, when it is incompatible or unavailable"`
	Reason   string              `json:"reason,omitempty" doc:"what turns the cache off, when it is disabled"`
	Settings *QueryCacheSettings `json:"settings,omitempty" doc:"the settings the cache runs with, those that are set"`
	Contents *QueryCacheContents `json:"contents,omitempty" doc:"what the cache holds, when it is ready"`
}

// QueryCacheSettings are the query cache's settings that are set.
type QueryCacheSettings struct {
	DefaultDuration string `json:"default_duration,omitempty" doc:"the cache.duration of a query that sets none (cache.query.default.duration in .anfra/config.yml)"`
	Namespace       string `json:"namespace,omitempty" doc:"the subfolder of canal-lake's storage the cache's files go in (ANFRA_CACHE_QUERY_NAMESPACE)"`
}

// QueryCacheContents are what the query cache holds.
type QueryCacheContents struct {
	Entries        int        `json:"entries" doc:"how many cached results queries can be answered from: committed, and not expired"`
	Bytes          int64      `json:"bytes" doc:"their size, as the rows were streamed"`
	OldestCachedAt *time.Time `json:"oldest_cached_at,omitempty" doc:"when the oldest of those results was read from the warehouse. Absent without any."`
	NewestCachedAt *time.Time `json:"newest_cached_at,omitempty" doc:"when the newest of those results was read from the warehouse. Absent without any."`
	Expired        Expired    `json:"expired" doc:"results no query uses any more, expired, cleaned or abandoned, whose files are deleted by the next sweep"`
	Writing        int        `json:"writing" doc:"results being written now, by queries still running"`
}

// Expired are results no query uses any more.
type Expired struct {
	Entries int   `json:"entries" doc:"how many"`
	Bytes   int64 `json:"bytes" doc:"their size"`
}

func status(ctx context.Context, cc command.CommandContext, _ CacheStatusRequestParams) (CacheStatusResult, error) {
	qc := cc.QueryCache
	if qc == nil {
		return CacheStatusResult{}, nil
	}
	sum := qc.Summarize(ctx)
	out := &QueryCacheStatus{State: sum.State, Problem: sum.Problem, Reason: sum.Reason}
	var hints []string
	if sum.Hint != "" {
		hints = append(hints, sum.Hint)
	}
	if sum.State == querycache.StateDisabled {
		return CacheStatusResult{Query: out, Hints: hints}, nil
	}
	settings := qc.Settings()
	if d, ns := settings.Default.Duration, settings.Namespace; d > 0 || ns != "" {
		out.Settings = &QueryCacheSettings{Namespace: ns}
		if d > 0 {
			out.Settings.DefaultDuration = d.String()
		}
	}
	contents, err := qc.Contents(ctx)
	out.Folder = contents.Folder
	if sum.State != querycache.StateReady {
		return CacheStatusResult{Query: out, Hints: hints}, nil
	}
	if err != nil {
		// Ready a moment ago, and unreadable now: say so as status would.
		out.State, out.Problem = querycache.StateUnavailable, "anfra can't read the query cache's store: "+err.Error()+"."
		return CacheStatusResult{Query: out, Hints: hints}, nil
	}
	c := contents.Contents
	out.Contents = &QueryCacheContents{Entries: c.Entries, Bytes: c.Bytes, Writing: c.Writing,
		Expired: Expired{Entries: c.Expired.Entries, Bytes: c.Expired.Bytes}}
	if !c.OldestCachedAt.IsZero() {
		out.Contents.OldestCachedAt, out.Contents.NewestCachedAt = &c.OldestCachedAt, &c.NewestCachedAt
	}
	return CacheStatusResult{Query: out, Hints: hints}, nil
}
