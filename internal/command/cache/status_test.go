package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/querycache"
	localquerycache "github.com/holistics/anfra/internal/querycache/local"
)

// A status reads what the cache holds without making a store, counts current,
// expired and pending entries apart, and says no more than the state when the
// cache is off or can't be read.
func TestStatus(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "query")
	qc := localquerycache.New(dir, localquerycache.Options{Namespace: "prod", Default: command.QueryCacheDefaults{Duration: time.Hour}})
	t.Cleanup(func() { qc.Close(ctx) })
	cc := command.CommandContext{QueryCache: qc}

	got, err := status(ctx, cc, CacheStatusRequestParams{})
	if err != nil || got.Query.State != querycache.StateReady || got.Query.Folder != dir || got.Query.Contents == nil || got.Query.Contents.Entries != 0 {
		t.Fatalf("no store yet: %+v, %v", got.Query, err)
	}
	if _, err := os.Stat(qc.StorePath()); !os.IsNotExist(err) {
		t.Error("the status made a store")
	}
	if s := got.Query.Settings; s == nil || s.DefaultDuration != "1h0m0s" || s.Namespace != "prod" {
		t.Errorf("settings: %+v", s)
	}

	m, err := qc.Manager(ctx)
	if err != nil {
		t.Fatal(err)
	}
	write(ctx, t, m, "k1")
	write(ctx, t, m, "k2")
	if _, err := m.Begin(ctx, "k3", nil, "", "select 1"); err != nil { // still writing
		t.Fatal(err)
	}
	if _, err := m.Clean(ctx, nil); err != nil { // expires k1 and k2, and ends k3's lease
		t.Fatal(err)
	}
	write(ctx, t, m, "k4")
	got, _ = status(ctx, cc, CacheStatusRequestParams{})
	c := got.Query.Contents
	if c.Entries != 1 || c.Bytes != 100 || c.Expired.Entries != 3 || c.Expired.Bytes != 200 || c.Writing != 0 || c.OldestCachedAt == nil {
		t.Errorf("contents: %+v", c)
	}
	if _, err := m.Begin(ctx, "k5", nil, "", "select 1"); err != nil {
		t.Fatal(err)
	}
	if got, _ = status(ctx, cc, CacheStatusRequestParams{}); got.Query.Contents.Writing != 1 {
		t.Errorf("writing: %+v", got.Query.Contents)
	}

	off := localquerycache.New(dir, localquerycache.Options{Disabled: "turned off"})
	if got, _ = status(ctx, command.CommandContext{QueryCache: off}, CacheStatusRequestParams{}); got.Query.State != querycache.StateDisabled || got.Query.Folder != "" || got.Query.Contents != nil {
		t.Errorf("disabled: %+v", got.Query)
	}
	bad := localquerycache.New(dir, localquerycache.Options{Namespace: "my-prod"})
	if got, _ = status(ctx, command.CommandContext{QueryCache: bad}, CacheStatusRequestParams{}); got.Query.State != querycache.StateUnavailable || got.Query.Contents != nil || len(got.Hints) != 1 {
		t.Errorf("unavailable: %+v, hints %v", got.Query, got.Hints)
	}
}
