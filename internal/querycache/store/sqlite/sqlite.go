// Package sqlite is anfra's query cache store (querycache.Store), on SQLite:
// one file per repo, under anfra's home. modernc.org/sqlite is pure Go, as
// anfra builds without cgo. Its queries are queries.sql, generated into gen/ by
// sqlc (sqlc.yaml).
//
// Import it as sqlitestore, so a call site says it is the store it opens, not
// the query cache:
//
//	import sqlitestore "github.com/holistics/anfra/internal/querycache/store/sqlite"
//
//	store, err := sqlitestore.Open(ctx, path, sqlitestore.Options{})
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"time"

	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/internal/querycache/store/sqlite/gen"
	"github.com/holistics/anfra/shared/jsonkit"
	_ "modernc.org/sqlite" // the "sqlite" driver
)

// Store is a querycache.Store on one SQLite file.
type Store struct {
	db *sql.DB
	q  *gen.Queries
}

var _ querycache.Store = (*Store)(nil)

// dsn is the store at path, opened with params. Several processes may open one
// store: a write waits for another's (WAL, a busy timeout), and each
// transaction takes its write lock as it starts, so two commits never deadlock
// upgrading a read.
func dsn(path string, params url.Values) string {
	params.Add("_pragma", "busy_timeout(10000)")
	return "file:" + path + "?" + params.Encode()
}

// Options are how Open opens a store.
type Options struct {
	// OnCreate, if set, runs when Open creates the store, inside the
	// transaction that creates it. Another process's Open waits for it, so
	// nothing has used the store yet: what OnCreate finds is no entry's.
	OnCreate func()
}

// Open opens the store at path, creating it when there is none, and migrates it
// to this anfra's schema version (CurrentVersion). It refuses a newer store this anfra can't
// use (ErrOtherVersion, a *VersionError) and a file that isn't a store
// (ErrNotStore), and never changes either. A failed migration leaves the store
// as it was, to be tried again at the next Open.
func Open(ctx context.Context, path string, o Options) (*Store, error) {
	params := url.Values{}
	params.Add("_pragma", "journal_mode(WAL)")
	params.Add("_pragma", "synchronous(NORMAL)")
	params.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", dsn(path, params))
	if err != nil {
		return nil, fmt.Errorf("open the query cache's store: %w", err)
	}
	if _, err := migrate(ctx, db, o.OnCreate); err != nil {
		db.Close()
		return nil, notStore(err)
	}
	return &Store{db: db, q: gen.New(db)}, nil
}

// Peek reads the versions of the store at path without changing it, nor
// creating it: what a status reports. os.ErrNotExist when there is none.
func Peek(ctx context.Context, path string) (Versions, error) {
	if _, err := os.Stat(path); err != nil {
		return Versions{}, err
	}
	params := url.Values{}
	params.Set("mode", "ro")
	db, err := sql.Open("sqlite", dsn(path, params))
	if err != nil {
		return Versions{}, fmt.Errorf("open the query cache's store: %w", err)
	}
	defer db.Close()
	return readVersions(ctx, db)
}

// ReadContents reads what the store at path holds at now, without creating,
// changing or migrating it, as Peek reads its versions: nothing when it isn't
// there. A store this anfra can't use is a *VersionError, since its tables may
// not be the ones read here.
func ReadContents(ctx context.Context, path string, now time.Time) (querycache.Contents, error) {
	v, err := Peek(ctx, path)
	if errors.Is(err, os.ErrNotExist) {
		return querycache.Contents{}, nil
	}
	if err != nil {
		return querycache.Contents{}, err
	}
	if !v.Compatible() {
		return querycache.Contents{}, &VersionError{v}
	}
	params := url.Values{}
	params.Set("mode", "ro")
	db, err := sql.Open("sqlite", dsn(path, params))
	if err != nil {
		return querycache.Contents{}, fmt.Errorf("open the query cache's store: %w", err)
	}
	defer db.Close()
	var (
		c              querycache.Contents
		oldest, newest sql.NullInt64
	)
	err = db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(CASE WHEN cached_at IS NOT NULL AND expires_at > ?1 THEN 1 END), 0),
		  COALESCE(SUM(CASE WHEN cached_at IS NOT NULL AND expires_at > ?1 THEN size_bytes END), 0),
		  MIN(CASE WHEN cached_at IS NOT NULL AND expires_at > ?1 THEN cached_at END),
		  MAX(CASE WHEN cached_at IS NOT NULL AND expires_at > ?1 THEN cached_at END),
		  COALESCE(SUM(CASE WHEN expires_at <= ?1 THEN 1 END), 0),
		  COALESCE(SUM(CASE WHEN expires_at <= ?1 THEN size_bytes END), 0),
		  COALESCE(SUM(CASE WHEN cached_at IS NULL AND expires_at > ?1 THEN 1 END), 0)
		FROM query_cache_physical_entries`, ms(now)).Scan(
		&c.Entries, &c.Bytes, &oldest, &newest, &c.Expired.Entries, &c.Expired.Bytes, &c.Writing)
	if err != nil {
		return querycache.Contents{}, fmt.Errorf("read the query cache store's contents: %w", err)
	}
	if oldest.Valid {
		c.OldestCachedAt, c.NewestCachedAt = fromMs(oldest.Int64), fromMs(newest.Int64)
	}
	return c, nil
}

// Close closes the store.
func (s *Store) Close() error { return s.db.Close() }

// errReplaced is a store found older than when this process opened it, which
// migrated it: another file has replaced it.
var errReplaced = errors.New("the query cache's store was replaced by an older one; open it again")

// check reads the store's version before a call (the header, microseconds), so
// a process meeting a store a newer anfra changed incompatibly since it opened
// it stops using it at once. A newer, compatible store is used as it is.
func check(ctx context.Context, q querier) error {
	var v int
	if err := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("read the query cache store's version: %w", err)
	}
	if v == CurrentVersion {
		return nil
	}
	vs, err := readVersions(ctx, q)
	switch {
	case err != nil:
		return err
	case !vs.Compatible():
		return &VersionError{vs}
	case vs.Installed < CurrentVersion:
		return errReplaced
	}
	return nil
}

// read checks the store's version before a read. A read racing a migration in
// the instant between the check and its query fails like any store error.
func (s *Store) read(ctx context.Context) error { return check(ctx, s.db) }

// write runs fn in one transaction, after checking the store's version under
// its lock, so it can't race a migration.
func (s *Store) write(ctx context.Context, fn func(q *gen.Queries) error) error {
	return inTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := check(ctx, tx); err != nil {
			return err
		}
		return fn(gen.New(tx))
	})
}

// inTx runs fn in one transaction.
func inTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func ms(t time.Time) int64 { return t.UnixMilli() }

func fromMs(v int64) time.Time { return time.UnixMilli(v).UTC() }

// entry is a physical entry's row as querycache has it.
func entry(r gen.QueryCachePhysicalEntry) querycache.Entry {
	e := querycache.Entry{
		ID: querycache.EntryID(r.ID), Namespace: r.Namespace, Key: querycache.Key(r.Key),
		ExpiresAt: fromMs(r.ExpiresAt), RowCount: r.RowCount, SizeBytes: r.SizeBytes, Columns: r.Columns,
		AQL: r.Aql, SQL: r.Sql,
	}
	if r.CachedAt != nil {
		e.CachedAt = fromMs(*r.CachedAt)
	}
	return e
}

// found is a row read for one entry: the entry, or false when there is none.
func found(r gen.QueryCachePhysicalEntry, err error) (querycache.Entry, bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return querycache.Entry{}, false, nil
	}
	if err != nil {
		return querycache.Entry{}, false, err
	}
	return entry(r), true, nil
}

func (s *Store) Lookup(ctx context.Context, key querycache.Key, now time.Time) (querycache.Entry, bool, error) {
	if err := s.read(ctx); err != nil {
		return querycache.Entry{}, false, err
	}
	return found(s.q.Lookup(ctx, gen.LookupParams{Key: string(key), Now: ms(now)}))
}

func (s *Store) Get(ctx context.Context, id querycache.EntryID, now time.Time) (querycache.Entry, bool, error) {
	if err := s.read(ctx); err != nil {
		return querycache.Entry{}, false, err
	}
	return found(s.q.Get(ctx, gen.GetParams{ID: int64(id), Now: ms(now)}))
}

func (s *Store) Begin(ctx context.Context, e querycache.Entry, tags []querycache.Tag) (querycache.EntryID, error) {
	var id int64
	if !querycache.ValidNamespace(e.Namespace) {
		return 0, fmt.Errorf("query cache namespace %q: want word segments, each ending in /", e.Namespace)
	}
	err := s.write(ctx, func(q *gen.Queries) error {
		var err error
		id, err = q.BeginWrite(ctx, gen.BeginWriteParams{
			Namespace: e.Namespace, Key: string(e.Key), ExpiresAt: ms(e.ExpiresAt), Aql: e.AQL, Sql: e.SQL,
		})
		if err != nil {
			return fmt.Errorf("begin a query cache write: %w", err)
		}
		for _, t := range tags {
			if err := q.TagKey(ctx, gen.TagKeyParams{Key: string(e.Key), Kind: string(t.Kind), Value: t.Value}); err != nil {
				return fmt.Errorf("tag a query cache key: %w", err)
			}
		}
		return nil
	})
	return querycache.EntryID(id), err
}

func (s *Store) Commit(ctx context.Context, e querycache.Entry, now time.Time) error {
	return s.write(ctx, func(q *gen.Queries) error {
		lease, err := q.PendingLease(ctx, gen.PendingLeaseParams{ID: int64(e.ID), Key: string(e.Key)})
		if errors.Is(err, sql.ErrNoRows) {
			return querycache.ErrNotPending
		}
		if err != nil {
			return fmt.Errorf("read a query cache write: %w", err)
		}

		current := lease > ms(now) // its lease hasn't ended: not abandoned, cleaned or late
		if current {
			winner, err := q.CurrentOf(ctx, string(e.Key))
			switch {
			case errors.Is(err, sql.ErrNoRows):
			case err != nil:
				return fmt.Errorf("read a query cache key: %w", err)
			default:
				current = winner < int64(e.ID)
			}
		}

		cached, expires := ms(e.CachedAt), ms(e.ExpiresAt)
		if !current {
			expires = ms(now) // lost: committed, and never used
		}
		if err := q.FillWrite(ctx, gen.FillWriteParams{
			CachedAt: &cached, ExpiresAt: expires, RowCount: e.RowCount, SizeBytes: e.SizeBytes, Columns: e.Columns, ID: int64(e.ID),
		}); err != nil {
			return fmt.Errorf("commit a query cache write: %w", err)
		}
		if !current {
			return nil
		}
		if err := q.MakeCurrent(ctx, gen.MakeCurrentParams{Key: string(e.Key), PhysicalID: int64(e.ID)}); err != nil {
			return fmt.Errorf("make a query cache entry current: %w", err)
		}
		if err := q.ExpireReplaced(ctx, gen.ExpireReplacedParams{At: ms(now), Key: string(e.Key), ID: int64(e.ID)}); err != nil {
			return fmt.Errorf("expire a query cache key's replaced entries: %w", err)
		}
		return nil
	})
}

func (s *Store) Abandon(ctx context.Context, id querycache.EntryID, now time.Time) error {
	return s.write(ctx, func(q *gen.Queries) error {
		n, err := q.Abandon(ctx, gen.AbandonParams{Now: ms(now), ID: int64(id)})
		if err != nil {
			return fmt.Errorf("abandon a query cache write: %w", err)
		}
		if n == 0 {
			return querycache.ErrNotPending
		}
		return nil
	})
}

func (s *Store) Sweepable(ctx context.Context, cutoff time.Time, limit int) ([]querycache.Entry, error) {
	if err := s.read(ctx); err != nil {
		return nil, err
	}
	rows, err := s.q.Sweepable(ctx, gen.SweepableParams{Cutoff: ms(cutoff), MaxEntries: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("list sweepable query cache entries: %w", err)
	}
	out := make([]querycache.Entry, len(rows))
	for i, r := range rows {
		out[i] = entry(r)
	}
	return out, nil
}

func (s *Store) Delete(ctx context.Context, ids []querycache.EntryID) error {
	if len(ids) == 0 {
		return nil
	}
	list := make([]int64, len(ids))
	for i, id := range ids {
		list[i] = int64(id)
	}
	return s.write(ctx, func(q *gen.Queries) error {
		// The keys losing an entry: those left with none lose their tags too.
		keys, err := q.KeysOf(ctx, list)
		if err != nil {
			return fmt.Errorf("read deleted query cache entries' keys: %w", err)
		}
		if err := q.DeleteEntries(ctx, list); err != nil {
			return fmt.Errorf("delete query cache entries: %w", err)
		}
		if err := q.DeleteLogical(ctx, list); err != nil {
			return fmt.Errorf("delete query cache keys: %w", err)
		}
		for _, k := range keys {
			if err := q.DeleteTagsOfEmptyKey(ctx, k); err != nil {
				return fmt.Errorf("delete a query cache key's tags: %w", err)
			}
		}
		return nil
	})
}

// tagList is tags as a JSON array of [kind, value], for the queries' json_each:
// distinct, so a key matches all of them when it has as many.
func tagList(tags []querycache.Tag) (string, int64, error) {
	pairs := make([][2]string, 0, len(tags))
	for _, t := range tags {
		if p := [2]string{string(t.Kind), t.Value}; !slices.Contains(pairs, p) {
			pairs = append(pairs, p)
		}
	}
	b, err := jsonkit.Marshal(pairs)
	return string(b), int64(len(pairs)), err
}

func (s *Store) Expire(ctx context.Context, tags []querycache.Tag, now time.Time) (querycache.Expired, error) {
	var out querycache.Expired
	err := s.write(ctx, func(q *gen.Queries) error {
		// Committed entries stop being used; pending writes' leases end, so they commit expired.
		if len(tags) == 0 {
			n, err := q.CountAllToExpire(ctx, ms(now))
			if err != nil {
				return fmt.Errorf("count query cache entries to expire: %w", err)
			}
			out = querycache.Expired{Entries: int(n.Entries), Bytes: n.Bytes}
			if err := q.ExpireAll(ctx, ms(now)); err != nil {
				return fmt.Errorf("expire query cache entries: %w", err)
			}
			return nil
		}
		list, count, err := tagList(tags)
		if err != nil {
			return err
		}
		n, err := q.CountTaggedToExpire(ctx, gen.CountTaggedToExpireParams{Now: ms(now), Tags: list, TagCount: count})
		if err != nil {
			return fmt.Errorf("count query cache entries to expire: %w", err)
		}
		out = querycache.Expired{Entries: int(n.Entries), Bytes: n.Bytes}
		if err := q.ExpireTagged(ctx, gen.ExpireTaggedParams{Now: ms(now), Tags: list, TagCount: count}); err != nil {
			return fmt.Errorf("expire query cache entries: %w", err)
		}
		return nil
	})
	return out, err
}
