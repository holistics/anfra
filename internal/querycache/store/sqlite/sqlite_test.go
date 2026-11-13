package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holistics/anfra/engine/querycachetest"
	"github.com/holistics/anfra/internal/querycache"
)

var ctx = context.Background()

func open(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(ctx, path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore(t *testing.T) {
	querycachetest.Run(t, func(t *testing.T) querycache.Store {
		return open(t, filepath.Join(t.TempDir(), "store.sqlite"))
	})
}

// exec runs statements on the file at path, as another process would.
func exec(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// A store at this anfra's schema version opens as it was left; a newer one opens when
// compatible, and is refused, unchanged, when not.
func TestOpenVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.sqlite")
	open(t, path).Close()
	open(t, path).Close() // again, as it was left

	exec(t, path, fmt.Sprintf("PRAGMA user_version = %d", CurrentVersion+1))
	s := open(t, path) // newer, and its min_store_version is one this anfra meets
	if v, _ := readVersions(ctx, s.db); v.Installed != CurrentVersion+1 {
		t.Errorf("a newer store was migrated down, to %d", v.Installed)
	}
	s.Close()

	exec(t, path, fmt.Sprintf("UPDATE query_cache_meta SET min_store_version = %d", CurrentVersion+1))
	_, err := Open(ctx, path, Options{})
	var ve *VersionError
	if !errors.Is(err, ErrOtherVersion) || !errors.As(err, &ve) || ve.Installed != CurrentVersion+1 || ve.RequiredByInstalled != CurrentVersion+1 {
		t.Errorf("an incompatible store: %v, want a *VersionError naming %d, %d", err, CurrentVersion+1, CurrentVersion+1)
	}
	if v, err := Peek(ctx, path); err != nil || v.Installed != CurrentVersion+1 {
		t.Errorf("the refused store changed: %+v (%v)", v, err)
	}
}

// A file that isn't a store is refused, and left as it is.
func TestOpenNotStore(t *testing.T) {
	dir := t.TempDir()
	tables := filepath.Join(dir, "tables.sqlite")
	exec(t, tables, "CREATE TABLE something (x)")
	garbage := filepath.Join(dir, "garbage.sqlite")
	if err := os.WriteFile(garbage, []byte("not a database, at all, not even close to one; sixteen bytes and more"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"a database with other tables": tables, "not a database": garbage} {
		if _, err := Open(ctx, path, Options{}); !errors.Is(err, ErrNotStore) {
			t.Errorf("%s: %v, want ErrNotStore", name, err)
		}
	}
	if b, _ := os.ReadFile(garbage); string(b[:3]) != "not" {
		t.Error("the file that isn't a store was changed")
	}
}

// A process meeting a store a newer anfra made incompatible since it opened it
// stops using it at once, reading or writing; a compatible change, it uses.
func TestCheckEachCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.sqlite")
	s := open(t, path)
	now := time.Now()

	exec(t, path, fmt.Sprintf("PRAGMA user_version = %d", CurrentVersion+1))
	if _, _, err := s.Lookup(ctx, "k", now); err != nil {
		t.Errorf("a newer, compatible store: %v", err)
	}

	exec(t, path, fmt.Sprintf("UPDATE query_cache_meta SET min_store_version = %d", CurrentVersion+1))
	if _, _, err := s.Lookup(ctx, "k", now); !errors.Is(err, ErrOtherVersion) {
		t.Errorf("read: %v, want ErrOtherVersion", err)
	}
	if _, err := s.Begin(ctx, querycache.Entry{Key: "k", ExpiresAt: now}, nil); !errors.Is(err, ErrOtherVersion) {
		t.Errorf("write: %v, want ErrOtherVersion", err)
	}
	if _, err := s.Expire(ctx, nil, now); !errors.Is(err, ErrOtherVersion) {
		t.Errorf("write: %v, want ErrOtherVersion", err)
	}
}

// A store of every earlier version migrates to this one, and passes querycachetest.
func TestMigrateFromEveryVersion(t *testing.T) {
	for from := 0; from < CurrentVersion; from++ {
		t.Run(fmt.Sprintf("from %d", from), func(t *testing.T) {
			querycachetest.Run(t, func(t *testing.T) querycache.Store {
				path := filepath.Join(t.TempDir(), "store.sqlite")
				stmts := []string{}
				for _, m := range migrations[:from] {
					stmts = append(stmts, m.sql)
				}
				if from > 0 {
					stmts = append(stmts, fmt.Sprintf("PRAGMA user_version = %d", from))
					exec(t, path, stmts...)
				}
				return open(t, path)
			})
		})
	}
}

// A breaking migration sets the store's min_store_version to its own number, so an
// older anfra stops using it; an additive one leaves it.
func TestBreakingMigration(t *testing.T) {
	saved, savedVersion := migrations, CurrentVersion
	t.Cleanup(func() { migrations, CurrentVersion = saved, savedVersion })

	more := fstest.MapFS{}
	for _, m := range saved {
		more[fmt.Sprintf("migrations/%04d_m.sql", m.version)] = &fstest.MapFile{Data: []byte(m.sql)}
	}
	n := len(saved)
	more[fmt.Sprintf("migrations/%04d_add.sql", n+1)] = &fstest.MapFile{Data: []byte("CREATE TABLE added (x);")}
	more[fmt.Sprintf("migrations/%04d_drop.sql", n+2)] = &fstest.MapFile{Data: []byte("-- breaking\nDROP TABLE added;")}

	path := filepath.Join(t.TempDir(), "store.sqlite")
	open(t, path).Close() // at this anfra's version

	var err error
	if migrations, err = loadMigrations(more); err != nil {
		t.Fatal(err)
	}
	CurrentVersion = len(migrations)
	if !migrations[n+1].breaking || migrations[n].breaking {
		t.Fatalf("breaking: %v, %v; want only the second", migrations[n].breaking, migrations[n+1].breaking)
	}
	open(t, path).Close()
	v, err := Peek(ctx, path)
	if err != nil || v.Installed != n+2 || v.RequiredByInstalled != n+2 {
		t.Errorf("after a breaking migration: %+v (%v), want version and minimum %d", v, err, n+2)
	}

	// The anfra before it can't use the store.
	migrations, CurrentVersion = saved, savedVersion
	if _, err := Open(ctx, path, Options{}); !errors.Is(err, ErrOtherVersion) {
		t.Errorf("an older anfra opened it: %v", err)
	}
}

// Migrations are numbered from 1 without a gap.
func TestLoadMigrations(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"a gap":       {"migrations/0001_a.sql": {}, "migrations/0003_c.sql": {}},
		"not from 1":  {"migrations/0002_b.sql": {}},
		"no number":   {"migrations/init.sql": {}},
		"a duplicate": {"migrations/0001_a.sql": {}, "migrations/0001_b.sql": {}},
	} {
		if _, err := loadMigrations(fsys); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

// OnCreate runs once, when the store is created, and not when it is opened again.
func TestOnCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.sqlite")
	created := 0
	o := Options{OnCreate: func() { created++ }}
	for range 2 {
		s, err := Open(ctx, path, o)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
	if created != 1 {
		t.Errorf("OnCreate ran %d times, want once", created)
	}
}

// Peek reads a store's versions without making or changing one.
func TestPeek(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.sqlite")
	if _, err := Peek(ctx, path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("no store: %v, want os.ErrNotExist", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("Peek made a store")
	}
	open(t, path).Close()
	if v, err := Peek(ctx, path); err != nil || v != (Versions{Installed: CurrentVersion, RequiredByInstalled: 1}) || !v.Compatible() {
		t.Errorf("Peek = %+v (%v)", v, err)
	}
}
