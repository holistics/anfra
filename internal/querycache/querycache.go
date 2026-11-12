// Package querycache is anfra's query cache: its data structures, its algorithm
// (Manager), and the Store that keeps its metadata, which anfra implements on
// SQLite (store/sqlite) and a platform embedding the engine may implement on its
// own database. A platform reaches the types through engine's aliases, and
// checks its store with engine/querycachetest.
//
// A key's results are physical entries: one for each write of the result, each
// with its own file in canal-lake. A logical entry, one per key, points at the
// physical entry that answers the key now. Entry is a physical entry; the
// logical entry is how a store keeps which one is current, and only Lookup and
// Get return a current one. See docs/designs/query-cache.md.
package querycache

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"
)

// Key is a query's key in the cache: anfra-node's cache key, hashed with what
// the compiler doesn't see (the data source's identity, the result's bounds).
type Key string

// EntryID identifies a physical entry, and orders writes: the greater is the
// newer, and wins a commit. A store never reuses one.
type EntryID int64

// Entry is a physical entry: one write of a key's result, with its own file in
// canal-lake. Whether it answers the key now is the store's to say: Lookup and
// Get return only an entry that does; Sweepable returns ones that don't.
type Entry struct {
	ID EntryID
	// Namespace prefixes its file's name in canal-lake (LakeKey): empty, or
	// segments each ending in "/", such as "3f9a1c0e/". A platform sharing
	// one canal-lake between tenants groups each one's files by it.
	Namespace string
	Key       Key
	// CachedAt is when its rows were read from the warehouse: when its write
	// began, as the warehouse reads a query's rows as it starts. Zero while its
	// write is pending: a pending entry is one with no CachedAt.
	CachedAt time.Time
	// ExpiresAt is when it stops being used. While pending, its lease: the
	// latest its write can commit and be used, set from the run's deadline so a
	// write can't outlive it, and swept like any expiry once it runs out.
	// Abandoning a write, or a clean, ends it at once, so its result is never
	// used (docs/designs/query-cache.md#a-writes-lease).
	ExpiresAt time.Time
	RowCount  int64
	SizeBytes int64
	// Columns is the result's header, as the executor gave it, kept as given.
	Columns []byte
	// AQL and SQL are what ran, for inspecting an entry.
	AQL, SQL string
}

// LakeKey is the entry's file in canal-lake: its namespace and its id, such as
// 1234, or 3f9a1c0e/1234 in a namespace. From a file to its entry is its id;
// from an entry to its file, this.
func (e Entry) LakeKey() string {
	return e.Namespace + strconv.FormatInt(int64(e.ID), 10)
}

var namespacePattern = regexp.MustCompile(`^(\w+/)*$`)

// ValidNamespace reports whether ns can prefix a file's name in canal-lake:
// empty, or word segments each ending in "/".
func ValidNamespace(ns string) bool { return namespacePattern.MatchString(ns) }

// TagKind is what a tag selects entries by.
type TagKind string

const (
	TagDataSource TagKind = "data_source"
	TagDataset    TagKind = "dataset"
	TagModel      TagKind = "model"
)

// Tag is what a key's entries are selected by for cleaning: the data source,
// dataset and models a query read. A tag is never read by a lookup.
type Tag struct {
	Kind  TagKind
	Value string
}

// Expired is what Expire expired: how many committed entries, and their size.
type Expired struct {
	Entries int
	Bytes   int64
}

// ErrNotPending is Commit's or Abandon's refusal of an entry that is not a
// pending write of the key given: committed already, or swept.
var ErrNotPending = errors.New("not a pending write")

// Store keeps the cache's metadata. Each method is one step of the cache's
// algorithm, and its transaction is the store's to keep. Times are given, never
// read from a clock, so the rules can be tested. engine/querycachetest says what each method
// must do; a store passes it, whatever its tables.
//
// A store is one cache: anfra's, one per repo. A platform serving several
// tenants binds a store to one for each invocation, and keeps the tenant in its
// own rows; nothing here names it.
type Store interface {
	// Lookup is the key's current entry, if it is unexpired at now. The age
	// rule, against the query's own duration, is the caller's.
	Lookup(ctx context.Context, key Key, now time.Time) (Entry, bool, error)
	// Get is the entry id, as Lookup would return it: current and unexpired at
	// now. For a Planner that found it by other means.
	Get(ctx context.Context, id EntryID, now time.Time) (Entry, bool, error)

	// Begin records a pending write (Namespace, Key, AQL and SQL set, and
	// ExpiresAt as its lease) and its key's tags, and gives it its id, which
	// with its namespace names its file (LakeKey). No Lookup returns it.
	Begin(ctx context.Context, e Entry, tags []Tag) (EntryID, error)
	// Commit finishes a pending write at now, in one transaction: fills it
	// (CachedAt, ExpiresAt, RowCount, SizeBytes, Columns), makes it the key's
	// current entry unless a greater id is already current, and expires the
	// key's other committed entries at now. A write that lost to a greater id,
	// or whose lease ended by now (abandoned, cleaned, or past its lease),
	// commits expired at now. ErrNotPending when e is not a pending write of
	// its key.
	Commit(ctx context.Context, e Entry, now time.Time) error
	// Abandon ends a pending write's lease at now: one that failed or was
	// cancelled. ErrNotPending when it is not pending.
	Abandon(ctx context.Context, id EntryID, now time.Time) error

	// Sweepable is up to limit entries a sweep may remove, oldest first: those
	// expired before cutoff, pending or committed.
	Sweepable(ctx context.Context, cutoff time.Time, limit int) ([]Entry, error)
	// Delete removes entries; and a key's logical entry and tags with its last
	// entry. The caller drops their files first.
	Delete(ctx context.Context, ids []EntryID) error
	// Expire expires at now every unexpired entry of the keys that have all of
	// tags, or of every key when tags is empty: what a clean does.
	// A pending write among them has its lease ended, so it commits expired:
	// after a clean, no result read before it is used, which is what someone
	// cleaning after fixing data relies on. It answers the committed entries
	// it expired, and their size.
	Expire(ctx context.Context, tags []Tag, now time.Time) (Expired, error)
}
