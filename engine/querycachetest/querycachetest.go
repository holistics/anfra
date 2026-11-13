// Package querycachetest is the conformance suite of the query cache's store
// (engine/querycache's Store): what each of its methods must do, whatever a
// store's tables. Every store runs it, anfra's and a platform's alike:
//
//	func TestStore(t *testing.T) {
//		querycachetest.Run(t, func(t *testing.T) querycache.Store { return openEmptyStore(t) })
//	}
package querycachetest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/querycache"
)

// Run runs the suite, each test on a store open gives it empty.
func Run(t *testing.T, open func(t *testing.T) querycache.Store) {
	for _, tc := range []struct {
		name string
		test func(t *testing.T, s querycache.Store)
	}{
		{"a pending write is not looked up", pendingIsInvisible},
		{"a committed write is looked up, as it was written", committedIsLookedUp},
		{"an entry expires", expires},
		{"the newer write wins, whichever commits first", newerWins},
		{"concurrent commits leave the newest current", concurrentCommits},
		{"only a pending write commits", onlyPendingCommits},
		{"an abandoned write never becomes current", abandonedNeverCurrent},
		{"a write past its lease never becomes current", leaseRunsOut},
		{"sweepable: expired before the cutoff, a lease among them", sweepable},
		{"delete removes a key with its last entry", deleteRemovesKey},
		{"expire by tags", expireByTags},
		{"expire reaches a write in progress", expireReachesPending},
	} {
		t.Run(tc.name, func(t *testing.T) { tc.test(t, open(t)) })
	}
}

var (
	ctx = context.Background()
	t0  = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
)

func at(minutes int) time.Time { return t0.Add(time.Duration(minutes) * time.Minute) }

// lease is how long a write may take, in the suite: from its start to the
// latest it can commit.
const lease = 30

// begin starts a write of key at minute started, with tags, its lease lease
// minutes.
func begin(t *testing.T, s querycache.Store, key querycache.Key, started int, tags ...querycache.Tag) querycache.Entry {
	t.Helper()
	return beginIn(t, s, "", key, started, tags...)
}

// beginIn is begin in a namespace.
func beginIn(t *testing.T, s querycache.Store, namespace string, key querycache.Key, started int, tags ...querycache.Tag) querycache.Entry {
	t.Helper()
	e := querycache.Entry{Namespace: namespace, Key: key, ExpiresAt: at(started + lease), AQL: "explore { … }", SQL: "select 1"}
	id, err := s.Begin(ctx, e, tags)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	e.ID = id
	return e
}

// commit finishes e at minute cached, kept for minutes.
func commit(t *testing.T, s querycache.Store, e querycache.Entry, cached, minutes int) querycache.Entry {
	t.Helper()
	e.CachedAt, e.ExpiresAt = at(cached), at(cached+minutes)
	e.RowCount, e.SizeBytes, e.Columns = 2, 128, []byte(`[{"name":"region"}]`)
	if err := s.Commit(ctx, e, at(cached)); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return e
}

func lookup(t *testing.T, s querycache.Store, key querycache.Key, now int) (querycache.Entry, bool) {
	t.Helper()
	e, ok, err := s.Lookup(ctx, key, at(now))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	return e, ok
}

func sameEntry(t *testing.T, got, want querycache.Entry) {
	t.Helper()
	if got.ID != want.ID || got.Namespace != want.Namespace || got.Key != want.Key ||
		!got.CachedAt.Equal(want.CachedAt) ||
		!got.ExpiresAt.Equal(want.ExpiresAt) || got.RowCount != want.RowCount || got.SizeBytes != want.SizeBytes ||
		string(got.Columns) != string(want.Columns) || got.AQL != want.AQL || got.SQL != want.SQL {
		t.Errorf("entry =\n  %+v\nwant\n  %+v", got, want)
	}
}

func pendingIsInvisible(t *testing.T, s querycache.Store) {
	e := begin(t, s, "k", 0)
	if _, ok := lookup(t, s, "k", 1); ok {
		t.Error("Lookup returned a pending write")
	}
	if _, ok, err := s.Get(ctx, e.ID, at(1)); ok || err != nil {
		t.Errorf("Get returned a pending write (%v)", err)
	}
}

func committedIsLookedUp(t *testing.T, s querycache.Store) {
	want := commit(t, s, begin(t, s, "k", 0), 1, 60)
	got, ok := lookup(t, s, "k", 2)
	if !ok {
		t.Fatal("Lookup missed a committed write")
	}
	sameEntry(t, got, want)
	got, ok, err := s.Get(ctx, want.ID, at(2))
	if !ok || err != nil {
		t.Fatalf("Get missed a committed write (%v)", err)
	}
	sameEntry(t, got, want)

	// Its namespace is kept, as it names its file.
	spaced := commit(t, s, beginIn(t, s, "3f9a/", "k2", 0), 1, 60)
	got, ok = lookup(t, s, "k2", 2)
	if !ok {
		t.Fatal("Lookup missed a committed write in a namespace")
	}
	sameEntry(t, got, spaced)
}

func expires(t *testing.T, s querycache.Store) {
	e := commit(t, s, begin(t, s, "k", 0), 1, 60)
	if _, ok := lookup(t, s, "k", 60); !ok {
		t.Error("missed before it expired")
	}
	if _, ok := lookup(t, s, "k", 61); ok {
		t.Error("Lookup returned an entry at its expiry")
	}
	if _, ok, _ := s.Get(ctx, e.ID, at(61)); ok {
		t.Error("Get returned an entry at its expiry")
	}
}

func newerWins(t *testing.T, s querycache.Store) {
	older, newer := begin(t, s, "k", 0), begin(t, s, "k", 1)
	if newer.ID <= older.ID {
		t.Fatalf("ids %d, %d: a later write has a greater id", older.ID, newer.ID)
	}
	newer = commit(t, s, newer, 2, 60)
	commit(t, s, older, 3, 60) // finishes after the newer one, and loses
	if got, ok := lookup(t, s, "k", 4); !ok || got.ID != newer.ID {
		t.Errorf("current = %d (%v), want the newer write, %d", got.ID, ok, newer.ID)
	}

	// The other way round: the newer commits second, and the older one expires.
	a, b := begin(t, s, "k2", 0), begin(t, s, "k2", 1)
	a = commit(t, s, a, 2, 60)
	b = commit(t, s, b, 3, 60)
	if got, ok := lookup(t, s, "k2", 4); !ok || got.ID != b.ID {
		t.Errorf("current = %d (%v), want %d", got.ID, ok, b.ID)
	}
	if _, ok, _ := s.Get(ctx, a.ID, at(4)); ok {
		t.Error("the replaced entry is still current")
	}
}

func concurrentCommits(t *testing.T, s querycache.Store) {
	const n = 8
	writes := make([]querycache.Entry, n)
	for i := range writes {
		writes[i] = begin(t, s, "k", i)
	}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := n - 1; i >= 0; i-- { // newest first, racing
		wg.Add(1)
		go func(e querycache.Entry) {
			defer wg.Done()
			e.CachedAt, e.ExpiresAt = at(10), at(70)
			errs <- s.Commit(ctx, e, at(10))
		}(writes[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}
	newest := writes[0].ID
	for _, e := range writes {
		newest = max(newest, e.ID)
	}
	if got, ok := lookup(t, s, "k", 11); !ok || got.ID != newest {
		t.Errorf("current = %d (%v), want the newest, %d", got.ID, ok, newest)
	}
}

func onlyPendingCommits(t *testing.T, s querycache.Store) {
	e := commit(t, s, begin(t, s, "k", 0), 1, 60)
	if err := s.Commit(ctx, e, at(2)); !errors.Is(err, querycache.ErrNotPending) {
		t.Errorf("committing twice: %v, want ErrNotPending", err)
	}
	other := begin(t, s, "k", 2)
	other.Key = "another"
	if err := s.Commit(ctx, other, at(3)); !errors.Is(err, querycache.ErrNotPending) {
		t.Errorf("committing a write as another key's: %v, want ErrNotPending", err)
	}
	if err := s.Commit(ctx, querycache.Entry{ID: 1 << 40, Key: "k"}, at(3)); !errors.Is(err, querycache.ErrNotPending) {
		t.Errorf("committing a write that isn't there: %v, want ErrNotPending", err)
	}
}

func abandonedNeverCurrent(t *testing.T, s querycache.Store) {
	e := begin(t, s, "k", 0)
	if err := s.Abandon(ctx, e.ID, at(1)); err != nil {
		t.Fatalf("Abandon: %v", err)
	}
	// A write abandoned while canal-query finished it anyway commits expired.
	if err := s.Commit(ctx, querycache.Entry{ID: e.ID, Key: "k", CachedAt: at(0), ExpiresAt: at(60)}, at(2)); err != nil &&
		!errors.Is(err, querycache.ErrNotPending) {
		t.Fatalf("Commit after Abandon: %v", err)
	}
	if _, ok := lookup(t, s, "k", 3); ok {
		t.Error("an abandoned write is current")
	}
}

func leaseRunsOut(t *testing.T, s querycache.Store) {
	e := begin(t, s, "k", 0) // its lease ends at 30
	if err := s.Commit(ctx, querycache.Entry{ID: e.ID, Key: "k", CachedAt: at(0), ExpiresAt: at(lease + 60)}, at(lease+1)); err != nil &&
		!errors.Is(err, querycache.ErrNotPending) {
		t.Fatalf("Commit past the lease: %v", err)
	}
	if _, ok := lookup(t, s, "k", lease+2); ok {
		t.Error("a write that committed past its lease is current")
	}
}

func sweepable(t *testing.T, s querycache.Store) {
	expired := commit(t, s, begin(t, s, "a", 0), 1, 10) // expires at 11
	fresh := commit(t, s, begin(t, s, "b", 0), 1, 600)  // expires at 601
	stale := begin(t, s, "c", -40)                      // pending, its lease ended at -10
	running := begin(t, s, "d", 10)                     // pending, its lease ends at 40
	abandoned := begin(t, s, "e", 0)
	if err := s.Abandon(ctx, abandoned.ID, at(5)); err != nil {
		t.Fatal(err)
	}

	got, err := s.Sweepable(ctx, at(20), 100)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[querycache.EntryID]bool{}
	for _, e := range got {
		ids[e.ID] = true
	}
	for name, tc := range map[string]struct {
		e    querycache.Entry
		want bool
	}{
		"expired before the cutoff":        {expired, true},
		"unexpired":                        {fresh, false},
		"pending, its lease run out":       {stale, true},
		"pending, its lease still running": {running, false},
		"abandoned before the cutoff":      {abandoned, true},
	} {
		if ids[tc.e.ID] != tc.want {
			t.Errorf("%s: sweepable = %v, want %v", name, ids[tc.e.ID], tc.want)
		}
	}
	if got, _ := s.Sweepable(ctx, at(20), 1); len(got) != 1 {
		t.Errorf("limit 1: %d entries", len(got))
	}
}

func deleteRemovesKey(t *testing.T, s querycache.Store) {
	old := commit(t, s, begin(t, s, "k", 0), 1, 60)
	current := commit(t, s, begin(t, s, "k", 2), 3, 60)
	if err := s.Delete(ctx, []querycache.EntryID{old.ID}); err != nil {
		t.Fatal(err)
	}
	if got, ok := lookup(t, s, "k", 4); !ok || got.ID != current.ID {
		t.Errorf("deleting a replaced entry changed the current one: %d (%v)", got.ID, ok)
	}
	if err := s.Delete(ctx, []querycache.EntryID{current.ID}); err != nil {
		t.Fatal(err)
	}
	if _, ok := lookup(t, s, "k", 4); ok {
		t.Error("a deleted entry is current")
	}
	// The key starts again: a new write commits and is current.
	again := commit(t, s, begin(t, s, "k", 5), 6, 60)
	if got, ok := lookup(t, s, "k", 7); !ok || got.ID != again.ID {
		t.Errorf("after the key was removed: %d (%v), want %d", got.ID, ok, again.ID)
	}
}

func expireByTags(t *testing.T, s querycache.Store) {
	orders := querycache.Tag{Kind: querycache.TagModel, Value: "orders"}
	users := querycache.Tag{Kind: querycache.TagModel, Value: "users"}
	warehouse := querycache.Tag{Kind: querycache.TagDataSource, Value: "warehouse"}
	both := commit(t, s, begin(t, s, "both", 0, warehouse, orders, users), 1, 60)
	onlyOrders := commit(t, s, begin(t, s, "orders", 0, warehouse, orders), 1, 60)

	got, err := s.Expire(ctx, []querycache.Tag{orders, users}, at(2))
	if err != nil {
		t.Fatal(err)
	}
	if got != (querycache.Expired{Entries: 1, Bytes: both.SizeBytes}) {
		t.Errorf("expired %+v, want the one entry with both tags", got)
	}
	if _, ok := lookup(t, s, "both", 3); ok {
		t.Error("an entry with both tags is still current")
	}
	if _, ok := lookup(t, s, "orders", 3); !ok {
		t.Error("an entry with one of the tags was expired")
	}

	// No tags: every key.
	got, err = s.Expire(ctx, nil, at(4))
	if err != nil || got != (querycache.Expired{Entries: 1, Bytes: onlyOrders.SizeBytes}) {
		t.Errorf("expire all: %+v (%v), want the one left", got, err)
	}
}

func expireReachesPending(t *testing.T, s querycache.Store) {
	orders := querycache.Tag{Kind: querycache.TagModel, Value: "orders"}
	e := begin(t, s, "k", 0, orders)
	if _, err := s.Expire(ctx, []querycache.Tag{orders}, at(1)); err != nil {
		t.Fatal(err)
	}
	// Its query started before the clean: its rows aren't trusted, so it commits
	// expired, though its CachedAt, when it began, is before the clean too.
	if err := s.Commit(ctx, querycache.Entry{ID: e.ID, Key: "k", CachedAt: at(0), ExpiresAt: at(60)}, at(2)); err != nil &&
		!errors.Is(err, querycache.ErrNotPending) {
		t.Fatalf("Commit: %v", err)
	}
	if _, ok := lookup(t, s, "k", 3); ok {
		t.Error("a write cleaned while it ran is current")
	}
}
