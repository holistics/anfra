package cache

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/querycache"
	localquerycache "github.com/holistics/anfra/internal/querycache/local"
	"github.com/holistics/anfra/shared/apperr"
)

// write commits an entry of key with tags, kept for an hour.
func write(ctx context.Context, t *testing.T, m *querycache.Manager, key string, tags ...querycache.Tag) {
	t.Helper()
	w, err := m.Begin(ctx, querycache.Key(key), tags, "", "select 1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(ctx, 1, 100, nil, time.Hour); err != nil {
		t.Fatal(err)
	}
}

// A clean removes the entries whose tags match every field given, or every
// entry with none, and says how many and their size.
func TestClean(t *testing.T) {
	ctx := context.Background()
	qc := localquerycache.New(filepath.Join(t.TempDir(), "query"), localquerycache.Options{})
	t.Cleanup(func() { qc.Close(ctx) })
	m, err := qc.Manager(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := querycache.Tag{Kind: querycache.TagDataSource, Value: "w"}
	write(ctx, t, m, "k1", w, querycache.Tag{Kind: querycache.TagDataset, Value: "sales"})
	write(ctx, t, m, "k2", w, querycache.Tag{Kind: querycache.TagDataset, Value: "hr"})
	write(ctx, t, m, "k3", querycache.Tag{Kind: querycache.TagDataSource, Value: "other"})

	cc := command.CommandContext{QueryCache: qc}
	for _, step := range []struct {
		name string
		in   *QueryCacheSelector
		want int
	}{
		{"both fields must match", &QueryCacheSelector{DataSource: "other", Dataset: "sales"}, 0},
		{"by dataset", &QueryCacheSelector{Dataset: "sales"}, 1},
		{"by data source", &QueryCacheSelector{DataSource: "w"}, 1},
		{"every cache", nil, 1},
		{"all of the query cache, nothing left", &QueryCacheSelector{}, 0},
	} {
		got, err := clean(ctx, cc, CacheCleanRequestParams{Query: step.in})
		if err != nil || got.Query.Entries != step.want || got.Query.Bytes != int64(100*step.want) {
			t.Errorf("%s: %+v, %v; want %d entries", step.name, got, err, step.want)
		}
	}
}

// A clean of a cache that is off, or isn't working, fails with a code
// saying which.
func TestCleanRefusals(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "query")
	for _, tc := range []struct {
		name string
		qc   command.QueryCache
		want apperr.AnyCode
	}{
		{"no cache", nil, errcode.CacheDisabled},
		{"turned off", localquerycache.New(dir, localquerycache.Options{Disabled: "turned off"}), errcode.CacheDisabled},
		{"not working", localquerycache.New(dir, localquerycache.Options{Namespace: "my-prod"}), errcode.CacheUnavailable},
	} {
		cc := command.CommandContext{}
		if tc.qc != nil {
			cc.QueryCache = tc.qc
		}
		_, err := clean(ctx, cc, CacheCleanRequestParams{})
		if e, ok := errors.AsType[*apperr.Error](err); !ok || e.Code != tc.want.Code() {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
}
