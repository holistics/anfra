package querycache

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Lake is what the cache needs of canal-lake besides reading and writing an
// entry, which go through the executor: dropping entries' files.
type Lake interface {
	// Drop removes the files of these lake keys. A key with no file is no
	// error: a write that failed has none, and a sweep may be repeated.
	Drop(ctx context.Context, lakeKeys []string) error
}

// Defaults, for a Config that leaves them unset.
const (
	// DefaultReadGrace is how long after an entry expires its file is kept: the
	// longest a read of it can run (an export's, 30 minutes) and a margin, so a
	// read that began before expiry finishes.
	DefaultReadGrace = 35 * time.Minute
	// DefaultLeaseMargin is added to a write's deadline to make its lease.
	DefaultLeaseMargin = 5 * time.Minute
	// DefaultLease is the lease of a write whose context has no deadline.
	DefaultLease = 35 * time.Minute
)

// Config is a Manager's store, namespace and timings. Zero timings take
// the defaults.
type Config struct {
	Store Store
	// Namespace prefixes the names of the files its writes put in canal-lake:
	// empty for anfra, whose canal-lake is its repo's; a platform sharing one
	// canal-lake between tenants sets one per tenant. See ValidNamespace.
	Namespace string
	// ReadGrace is how long after an entry expires its file is kept, for a
	// read that began before.
	ReadGrace time.Duration
	// LeaseMargin is added to a write's deadline to make its lease.
	LeaseMargin time.Duration
	// DefaultLease is the lease of a write whose context has no deadline.
	DefaultLease time.Duration
	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

// Manager is the query cache's algorithm, over a Store, sweeping through a Lake: which entry
// a query accepts, a write's lease and commit, cleaning, and sweeping. It holds
// no state of its own: it is as cheap to make as its store, for each invocation.
type Manager struct {
	store        Store
	namespace    string
	readGrace    time.Duration
	leaseMargin  time.Duration
	defaultLease time.Duration
	now          func() time.Time
}

// NewManager makes a Manager. A namespace canal-lake can't take is an error.
func NewManager(c Config) (*Manager, error) {
	if !ValidNamespace(c.Namespace) {
		return nil, fmt.Errorf("query cache namespace %q: want word segments, each ending in /", c.Namespace)
	}
	m := &Manager{
		store: c.Store, namespace: c.Namespace,
		readGrace: c.ReadGrace, leaseMargin: c.LeaseMargin, defaultLease: c.DefaultLease,
	}
	if m.readGrace <= 0 {
		m.readGrace = DefaultReadGrace
	}
	if m.leaseMargin <= 0 {
		m.leaseMargin = DefaultLeaseMargin
	}
	if m.defaultLease <= 0 {
		m.defaultLease = DefaultLease
	}
	clock := c.Now
	if clock == nil {
		clock = time.Now
	}
	m.now = func() time.Time { return ms(clock()) }
	return m, nil
}

// ms is t in whole milliseconds, as a store keeps times: so an entry the
// manager hands out has the times a lookup of it gives back.
func ms(t time.Time) time.Time { return t.Truncate(time.Millisecond) }

// Lookup is the entry a query accepts: the key's current entry, unexpired, and
// younger than maxAge, the query's cache duration. So a query asking for
// fresher data than an entry was written for never gets older data than it
// accepts (docs/designs/query-cache.md#which-entry-a-query-accepts). None when
// maxAge is not positive: the query doesn't read the cache.
func (m *Manager) Lookup(ctx context.Context, key Key, maxAge time.Duration) (Entry, bool, error) {
	if maxAge <= 0 {
		return Entry{}, false, nil
	}
	now := m.now()
	e, ok, err := m.store.Lookup(ctx, key, now)
	if err != nil || !ok {
		return Entry{}, false, err
	}
	if now.Sub(e.CachedAt) >= maxAge {
		return Entry{}, false, nil
	}
	return e, true, nil
}

// Write is a write of a key's result in progress: its rows go to canal-lake
// under LakeKey, and Commit or Abandon ends it.
type Write struct {
	m       *Manager
	entry   Entry
	started time.Time
}

// LakeKey is the file in canal-lake the write's rows go to.
func (w *Write) LakeKey() string { return w.entry.LakeKey() }

// ID is the write's entry.
func (w *Write) ID() EntryID { return w.entry.ID }

// Begin starts a write of key's result, with the key's tags and what runs. Its
// lease is its context's deadline and a margin, so the write can't outlive it;
// a context without one gets the default lease. Its file in canal-lake is named
// by the namespace and its id (LakeKey).
func (m *Manager) Begin(ctx context.Context, key Key, tags []Tag, aql, sql string) (*Write, error) {
	now := m.now()
	lease := now.Add(m.defaultLease)
	if deadline, ok := ctx.Deadline(); ok {
		lease = ms(deadline.Add(m.leaseMargin))
	}
	e := Entry{Namespace: m.namespace, Key: key, ExpiresAt: lease, AQL: aql, SQL: sql}
	id, err := m.store.Begin(ctx, e, tags)
	if err != nil {
		return nil, err
	}
	e.ID = id
	return &Write{m: m, entry: e, started: now}, nil
}

// Commit ends the write: its rows are in canal-lake, rowCount of them in
// sizeBytes, their header columns, kept for duration. Its rows were read when
// it began, so that is its CachedAt, and it expires duration after. It becomes
// the key's current entry unless a newer one is, or its lease ended (it was
// cleaned, or ran past its deadline): then it is committed expired, and never
// used. ErrNotPending when it was swept or committed already.
func (w *Write) Commit(ctx context.Context, rowCount, sizeBytes int64, columns []byte, duration time.Duration) (Entry, error) {
	e := w.entry
	e.CachedAt, e.ExpiresAt = w.started, ms(w.started.Add(duration))
	e.RowCount, e.SizeBytes, e.Columns = rowCount, sizeBytes, columns
	if err := w.m.store.Commit(ctx, e, w.m.now()); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// Abandon ends the write without a result: its query failed or was cancelled.
// Its lease ends now, so it is swept, and never used. It runs whether or not ctx
// is done, since a cancelled query is a reason to abandon its write. A write
// already committed or swept is no error.
func (w *Write) Abandon(ctx context.Context) error {
	err := w.m.store.Abandon(context.WithoutCancel(ctx), w.entry.ID, w.m.now())
	if errors.Is(err, ErrNotPending) {
		return nil
	}
	return err
}

// Clean expires the entries of the keys with all of tags, or of every key
// without tags, and ends their writes' leases: after it, no result read before
// it is used (docs/designs/query-cache.md#cleaning). Their files go with the
// next sweep.
func (m *Manager) Clean(ctx context.Context, tags []Tag) (Expired, error) {
	return m.store.Expire(ctx, tags, m.now())
}

// Swept is what a sweep removed: how many entries, and their size.
type Swept struct {
	Entries int
	Bytes   int64
}

// Sweep removes up to limit entries expired for longer than the read grace,
// whatever their namespace, as each names its own file: their files from
// canal-lake first, through lake, then their rows. In that order a row may
// name a file already gone, but no file is left that no row names. Pending
// writes are among them once their lease has run out.
func (m *Manager) Sweep(ctx context.Context, lake Lake, limit int) (Swept, error) {
	entries, err := m.store.Sweepable(ctx, m.now().Add(-m.readGrace), limit)
	if err != nil || len(entries) == 0 {
		return Swept{}, err
	}
	keys := make([]string, len(entries))
	ids := make([]EntryID, len(entries))
	var swept Swept
	for i, e := range entries {
		keys[i], ids[i] = e.LakeKey(), e.ID
		swept.Bytes += e.SizeBytes
	}
	if err := lake.Drop(ctx, keys); err != nil {
		return Swept{}, fmt.Errorf("drop swept query cache entries' files: %w", err)
	}
	if err := m.store.Delete(ctx, ids); err != nil {
		return Swept{}, err
	}
	swept.Entries = len(entries)
	return swept, nil
}
