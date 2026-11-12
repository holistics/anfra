package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strconv"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// The store's schema is its migrations, applied in order; a store is made by
// applying them all (docs/designs/query-cache.md#its-schema-migrations, and
// migrations/README.md).
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// migration is one version of the store's schema.
type migration struct {
	version  int
	breaking bool // applying it sets the store's min_store_version to its own number
	sql      string
}

var migrations = mustLoadMigrations(migrationFiles)

// CurrentVersion is the schema version this anfra creates and migrates stores
// to: its last migration's.
var CurrentVersion = len(migrations)

// loadMigrations reads NNNN_what.sql files, numbered 1 up without a gap. One
// whose first line is "-- breaking" is breaking.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	out := make([]migration, 0, len(names))
	for i, name := range names { // Glob sorts them
		num, _, ok := strings.Cut(path.Base(name), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || v != i+1 {
			return nil, fmt.Errorf("migration %s: want it numbered %04d", name, i+1)
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		text := string(b)
		first, _, _ := strings.Cut(text, "\n")
		out = append(out, migration{version: v, breaking: strings.TrimSpace(first) == "-- breaking", sql: text})
	}
	return out, nil
}

func mustLoadMigrations(fsys fs.FS) []migration {
	m, err := loadMigrations(fsys)
	if err != nil {
		panic(err)
	}
	return m
}

// ErrOtherVersion is a store this anfra can't use: a newer anfra changed it in a
// way this one doesn't know. A *VersionError says which versions.
var ErrOtherVersion = errors.New("the query cache's store is of a newer, incompatible version")

// ErrNotStore is a file that isn't a query cache store: not a database, or one
// with tables but no store version.
var ErrNotStore = errors.New("not a query cache store")

// Versions are a store's schema versions. A store not made yet is at 0.
type Versions struct {
	// Installed is the store's schema version, which is the number of the last
	// migration applied to it. SQLite keeps it in the file's header, as
	// user_version.
	Installed int

	// RequiredByInstalled is the lowest CurrentVersion an anfra must have to
	// use the store's installed schema. The store keeps it in query_cache_meta,
	// as min_store_version.
	//
	// Only a breaking migration changes it, when that migration runs on the
	// store. For example:
	//
	//	Migrations run on the store      Installed  RequiredByInstalled  Anfras that can use it
	//	1 and 2, neither breaking        2          1                    at version 1 or later
	//	then 3, which adds a column      3          1                    at version 1 or later
	//	then 4, which drops a column     4          4                    at version 4 or later
	//
	// The store keeps this number because an older anfra can't work it out.
	// It has never seen the migrations after its own, so it can't tell whether
	// they break it. The anfra that ran them can, and records the answer.
	RequiredByInstalled int
}

// Compatible reports whether this anfra can use the store: as it is, or once
// migrated.
func (v Versions) Compatible() bool { return CurrentVersion >= v.RequiredByInstalled }

// VersionError is a store this anfra can't use (ErrOtherVersion), with its versions.
type VersionError struct{ Versions }

func (e *VersionError) Error() string {
	return fmt.Sprintf("the query cache's store's schema is at version %d, and requires schema version %d or later, but this anfra's is version %d",
		e.Installed, e.RequiredByInstalled, CurrentVersion)
}

func (e *VersionError) Is(target error) bool { return target == ErrOtherVersion }

// querier is what reading the versions needs: a database or a transaction.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readVersions reads a store's versions: its header's user_version, and its
// min_store_version from query_cache_meta. A database with tables but no
// version is ErrNotStore.
func readVersions(ctx context.Context, q querier) (Versions, error) {
	var v Versions
	if err := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v.Installed); err != nil {
		return v, notStore(fmt.Errorf("read the query cache store's version: %w", err))
	}
	if v.Installed == 0 {
		var tables int
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type = 'table'").Scan(&tables); err != nil {
			return v, notStore(fmt.Errorf("read the query cache store's tables: %w", err))
		}
		if tables > 0 {
			return v, ErrNotStore
		}
		return v, nil
	}
	if err := q.QueryRowContext(ctx, "SELECT min_store_version FROM query_cache_meta").Scan(&v.RequiredByInstalled); err != nil {
		return v, fmt.Errorf("read the query cache store's min_store_version: %w", err)
	}
	return v, nil
}

// notStore is err, as ErrNotStore when SQLite says the file isn't a database.
func notStore(err error) error {
	var se *sqlite.Error
	if errors.As(err, &se) && se.Code()&0xff == sqlite3.SQLITE_NOTADB {
		return fmt.Errorf("%w: %w", ErrNotStore, err)
	}
	return err
}

// migrate brings the store to this anfra's version, in one write transaction
// that reads its version first, so processes opening it at once migrate it once.
// A newer store is used as it is when compatible, never migrated down; an
// incompatible one is a *VersionError. A failure rolls back: the store is as it
// was.
//
// onCreate, if any, runs when the transaction creates the store, while it holds
// the write lock: no other process can have made this store, so none has
// written an entry yet.
func migrate(ctx context.Context, db *sql.DB, onCreate func()) (Versions, error) {
	var out Versions
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		v, err := readVersions(ctx, tx)
		if err != nil {
			return err
		}
		out = v
		if v.Installed >= CurrentVersion {
			if !v.Compatible() {
				return &VersionError{v}
			}
			return nil
		}
		if v.Installed == 0 && onCreate != nil {
			onCreate()
		}
		for _, m := range migrations[v.Installed:] {
			if _, err := tx.ExecContext(ctx, m.sql); err != nil {
				return fmt.Errorf("migrate the query cache's store to version %d: %w", m.version, err)
			}
			if m.breaking {
				if _, err := tx.ExecContext(ctx, "UPDATE query_cache_meta SET min_store_version = ?", m.version); err != nil {
					return fmt.Errorf("migrate the query cache's store to version %d: %w", m.version, err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", CurrentVersion)); err != nil {
			return fmt.Errorf("migrate the query cache's store to version %d: %w", CurrentVersion, err)
		}
		out, err = readVersions(ctx, tx)
		return err
	})
	return out, err
}
