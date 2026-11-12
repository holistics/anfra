package querycache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/querycache"
)

var t0 = time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)

// store is a querycache.Store that answers what a test sets, and records what
// the manager asked of it. Its rules are querycachetest's; the manager's are tested here.
type store struct {
	current  querycache.Entry
	hasEntry bool
	lookups  int

	begun     []querycache.Entry
	committed []querycache.Entry
	commitAt  []time.Time
	abandoned []error // the context's error, per Abandon
	expireAt  time.Time
	sweepable []querycache.Entry
	cutoff    time.Time
	deleted   []querycache.EntryID
	calls     []string
}

func (s *store) Lookup(_ context.Context, _ querycache.Key, _ time.Time) (querycache.Entry, bool, error) {
	s.lookups++
	return s.current, s.hasEntry, nil
}

func (s *store) Get(context.Context, querycache.EntryID, time.Time) (querycache.Entry, bool, error) {
	return querycache.Entry{}, false, nil
}

func (s *store) Begin(_ context.Context, e querycache.Entry, _ []querycache.Tag) (querycache.EntryID, error) {
	s.begun = append(s.begun, e)
	return querycache.EntryID(len(s.begun)), nil
}

func (s *store) Commit(_ context.Context, e querycache.Entry, now time.Time) error {
	s.committed, s.commitAt = append(s.committed, e), append(s.commitAt, now)
	return nil
}

func (s *store) Abandon(ctx context.Context, _ querycache.EntryID, _ time.Time) error {
	s.abandoned = append(s.abandoned, ctx.Err())
	return querycache.ErrNotPending
}

func (s *store) Sweepable(_ context.Context, cutoff time.Time, _ int) ([]querycache.Entry, error) {
	s.cutoff = cutoff
	return s.sweepable, nil
}

func (s *store) Delete(_ context.Context, ids []querycache.EntryID) error {
	s.calls, s.deleted = append(s.calls, "delete"), ids
	return nil
}

func (s *store) Expire(_ context.Context, _ []querycache.Tag, now time.Time) (querycache.Expired, error) {
	s.expireAt = now
	return querycache.Expired{Entries: 1}, nil
}

type lake struct {
	s       *store
	dropped []string
	err     error
}

func (l *lake) Drop(_ context.Context, keys []string) error {
	l.s.calls, l.dropped = append(l.s.calls, "drop"), keys
	return l.err
}

func manager(t *testing.T, s *store, l *lake, now *time.Time, namespace string) *querycache.Manager {
	t.Helper()
	m, err := querycache.NewManager(querycache.Config{Store: s, Lake: l, Namespace: namespace, Now: func() time.Time { return *now }})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// A query accepts an entry younger than its own duration; with none, it doesn't
// read the cache at all.
func TestLookupAgeRule(t *testing.T) {
	now := t0
	s := &store{current: querycache.Entry{ID: 1, CachedAt: t0.Add(-5 * time.Minute)}, hasEntry: true}
	m := manager(t, s, &lake{s: s}, &now, "")
	for _, tc := range []struct {
		name   string
		maxAge time.Duration
		want   bool
	}{
		{"older than it accepts", 5 * time.Minute, false},
		{"younger than it accepts", 10 * time.Minute, true},
		{"no duration", 0, false},
	} {
		_, ok, err := m.Lookup(context.Background(), "k", tc.maxAge)
		if err != nil || ok != tc.want {
			t.Errorf("%s: %v (%v), want %v", tc.name, ok, err, tc.want)
		}
	}
	if s.lookups != 2 {
		t.Errorf("the store was asked %d times, want 2: none without a duration", s.lookups)
	}
}

// A write's lease is its run's deadline and a margin, or the default without
// one; its file is named by the namespace and its id.
func TestBeginLeaseAndLakeKey(t *testing.T) {
	now := t0
	s := &store{}
	m := manager(t, s, &lake{s: s}, &now, "")

	ctx, cancel := context.WithDeadline(context.Background(), t0.Add(10*time.Minute))
	defer cancel()
	w1, err := m.Begin(ctx, "v1:abc", nil, "aql", "sql")
	if err != nil {
		t.Fatal(err)
	}
	w2, _ := m.Begin(context.Background(), "v1:abc", nil, "aql", "sql")

	if got, want := s.begun[0].ExpiresAt, t0.Add(10*time.Minute+querycache.DefaultLeaseMargin); !got.Equal(want) {
		t.Errorf("lease with a deadline = %v, want %v", got, want)
	}
	if got, want := s.begun[1].ExpiresAt, t0.Add(querycache.DefaultLease); !got.Equal(want) {
		t.Errorf("lease without a deadline = %v, want %v", got, want)
	}
	if w1.LakeKey() != "1" || w2.LakeKey() != "2" {
		t.Errorf("lake keys %q, %q: want their ids", w1.LakeKey(), w2.LakeKey())
	}
	if s.begun[0].Namespace != "" {
		t.Errorf("namespace %q, want none", s.begun[0].Namespace)
	}

	spaced := manager(t, s, &lake{s: s}, &now, "3f9a/")
	w3, _ := spaced.Begin(context.Background(), "v1:abc", nil, "aql", "sql")
	if w3.LakeKey() != "3f9a/3" || s.begun[2].Namespace != "3f9a/" {
		t.Errorf("in a namespace: lake key %q, recorded %q; want 3f9a/3, 3f9a/", w3.LakeKey(), s.begun[2].Namespace)
	}
	if !s.begun[0].CachedAt.IsZero() {
		t.Error("a write began with a CachedAt: it would be read as committed")
	}
}

// A namespace is what canal-lake takes as the start of a key.
func TestNamespace(t *testing.T) {
	for ns, ok := range map[string]bool{"": true, "a/": true, "3f9a/b_1/": true, "a": false, "/a/": false, "a//": false, "a b/": false, "../": false} {
		_, err := querycache.NewManager(querycache.Config{Namespace: ns})
		if (err == nil) != ok {
			t.Errorf("namespace %q: %v, want valid %v", ns, err, ok)
		}
	}
}

// A write's rows were read when it began: its CachedAt, from which it expires.
// The store decides at commit time whether its lease ended.
func TestCommit(t *testing.T) {
	now := t0
	s := &store{}
	m := manager(t, s, &lake{s: s}, &now, "")
	w, _ := m.Begin(context.Background(), "k", nil, "", "")
	now = t0.Add(3 * time.Minute)
	e, err := w.Commit(context.Background(), 2, 128, []byte(`[]`), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !e.CachedAt.Equal(t0) || !e.ExpiresAt.Equal(t0.Add(time.Hour)) || !s.commitAt[0].Equal(now) {
		t.Errorf("committed cached %v, expiring %v, at %v; want %v, %v, %v", e.CachedAt, e.ExpiresAt, s.commitAt[0], t0, t0.Add(time.Hour), now)
	}
}

// A cancelled query is the reason to abandon its write: the abandon isn't
// cancelled with it, and a write already gone is no error.
func TestAbandonOutlivesItsContext(t *testing.T) {
	now := t0
	s := &store{}
	m := manager(t, s, &lake{s: s}, &now, "")
	w, _ := m.Begin(context.Background(), "k", nil, "", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := w.Abandon(ctx); err != nil {
		t.Errorf("Abandon: %v, want none for a write already gone", err)
	}
	if s.abandoned[0] != nil {
		t.Errorf("the store's Abandon ran cancelled: %v", s.abandoned[0])
	}
}

// A sweep drops files before rows, past the read grace; when canal-lake fails,
// the rows stay, so no file is left that no row names.
func TestSweep(t *testing.T) {
	now := t0
	s := &store{sweepable: []querycache.Entry{{ID: 1, SizeBytes: 10}, {ID: 2, Namespace: "a/", SizeBytes: 5}}}
	l := &lake{s: s}
	m := manager(t, s, l, &now, "")

	swept, err := m.Sweep(context.Background(), 100)
	if err != nil || swept != (querycache.Swept{Entries: 2, Bytes: 15}) {
		t.Fatalf("swept %+v (%v)", swept, err)
	}
	if !s.cutoff.Equal(t0.Add(-querycache.DefaultReadGrace)) {
		t.Errorf("cutoff %v, want the read grace ago", s.cutoff)
	}
	if len(s.calls) != 2 || s.calls[0] != "drop" || s.calls[1] != "delete" {
		t.Errorf("calls %v, want the files dropped, then the rows deleted", s.calls)
	}
	if len(l.dropped) != 2 || l.dropped[0] != "1" || l.dropped[1] != "a/2" {
		t.Errorf("dropped %v, want each entry's own name", l.dropped)
	}

	s.calls, l.err = nil, errors.New("canal-lake down")
	if _, err := m.Sweep(context.Background(), 100); err == nil {
		t.Error("a failed drop: no error")
	}
	if len(s.calls) != 1 {
		t.Errorf("calls %v, want no delete after a failed drop", s.calls)
	}
}

// A clean expires at the time it runs.
func TestClean(t *testing.T) {
	now := t0
	s := &store{}
	m := manager(t, s, &lake{s: s}, &now, "")
	if _, err := m.Clean(context.Background(), nil); err != nil || !s.expireAt.Equal(t0) {
		t.Errorf("clean expired at %v (%v), want %v", s.expireAt, err, t0)
	}
}
