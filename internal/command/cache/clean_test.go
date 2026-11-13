package cache

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/querycache"
	localquerycache "github.com/holistics/anfra/internal/querycache/local"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
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

// A clean then sweeps: results expired for longer than the read grace have
// their files dropped through canal-query, and those it just cleaned wait. A
// failed drop leaves the clean done, without freed, and is reported.
func TestCleanSweeps(t *testing.T) {
	ctx := context.Background()
	var dropped []string
	fail := false
	canal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var body struct {
			Body struct {
				CacheKeys []string `json:"cache_keys"`
			} `json:"body"`
		}
		_ = jsonkit.UnmarshalRead(r.Body, &body)
		dropped = append(dropped, body.Body.CacheKeys...)
	}))
	defer canal.Close()

	var reported []error
	qc := localquerycache.New(filepath.Join(t.TempDir(), "query"), localquerycache.Options{
		Lake:   runningLake{},
		Report: func(_ context.Context, err error) { reported = append(reported, err) },
	})
	t.Cleanup(func() { qc.Close(ctx) })
	m, err := qc.Manager(ctx)
	if err != nil {
		t.Fatal(err)
	}
	old, err := m.Begin(ctx, "old", nil, "", "select 1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Commit(ctx, 1, 100, nil, -time.Hour); err != nil { // expired an hour ago
		t.Fatal(err)
	}
	write(ctx, t, m, "fresh")

	cc := command.CommandContext{QueryCache: qc, Clients: command.Clients{CanalQuery: canalquery.NewClient(canal.URL, false)}}
	got, err := clean(ctx, cc, CacheCleanRequestParams{})
	if err != nil || got.Query.Entries != 1 || got.Query.Freed == nil || got.Query.Freed.Entries != 1 || got.Query.Freed.Bytes != 100 {
		t.Fatalf("%+v, %v", got.Query, err)
	}
	if len(dropped) != 1 || dropped[0] != old.LakeKey() {
		t.Errorf("dropped %v, want the old result's file only", dropped)
	}

	fail = true
	old, _ = m.Begin(ctx, "old2", nil, "", "select 1")
	_, _ = old.Commit(ctx, 1, 100, nil, -time.Hour)
	if got, err = clean(ctx, cc, CacheCleanRequestParams{}); err != nil || got.Query.Freed != nil || len(reported) != 1 {
		t.Errorf("a failed drop: %+v, %v, reported %v", got.Query, err, reported)
	}
}

// runningLake is a canal-lake that something else runs: starting it does
// nothing.
type runningLake struct{}

func (runningLake) Start(context.Context) error { return nil }
func (runningLake) Close(context.Context)       {}
