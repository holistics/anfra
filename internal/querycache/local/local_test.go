package local

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/querycache"
	sqlitestore "github.com/holistics/anfra/internal/querycache/store/sqlite"
)

var ctx = context.Background()

func newCache(t *testing.T) *Cache {
	t.Helper()
	c := New(filepath.Join(t.TempDir(), "query"), Options{})
	t.Cleanup(func() { c.Close(ctx) })
	return c
}

// exec runs statements on the store's file, as another anfra would.
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

// Each state says what to do, and summarizing changes nothing.
func TestSummarize(t *testing.T) {
	t.Run("no store yet is ready, and summarizing makes none", func(t *testing.T) {
		c := newCache(t)
		if s := c.Summarize(ctx); s.State != querycache.StateReady || s.Store != c.StorePath() || s.Hint != "" {
			t.Errorf("%+v", s)
		}
		if _, err := os.Stat(c.StorePath()); !errors.Is(err, os.ErrNotExist) {
			t.Error("Summarize made a store")
		}
	})
	t.Run("an older or newer compatible store is ready", func(t *testing.T) {
		c := newCache(t)
		if _, _, err := c.Open(ctx); err != nil {
			t.Fatal(err)
		}
		exec(t, c.StorePath(), fmt.Sprintf("PRAGMA user_version = %d", sqlitestore.CurrentVersion+1))
		if s := c.Summarize(ctx); s.State != querycache.StateReady || s.Problem != "" {
			t.Errorf("%+v", s)
		}
	})
	t.Run("a newer incompatible store", func(t *testing.T) {
		c := newCache(t)
		if _, _, err := c.Open(ctx); err != nil {
			t.Fatal(err)
		}
		next := sqlitestore.CurrentVersion + 1
		exec(t, c.StorePath(), fmt.Sprintf("PRAGMA user_version = %d", next), fmt.Sprintf("UPDATE query_cache_meta SET min_store_version = %d", next))
		s := c.Summarize(ctx)
		want := fmt.Sprintf("(the store's schema is at version %d, and requires schema version %d or later, but this anfra's is version %d)", next, next, sqlitestore.CurrentVersion)
		if s.State != querycache.StateIncompatible || !strings.Contains(s.Problem, want) || !strings.Contains(s.Hint, "anfra update") {
			t.Errorf("%+v", s)
		}
	})
	t.Run("a file anfra didn't make", func(t *testing.T) {
		c := newCache(t)
		if err := os.MkdirAll(filepath.Dir(c.StorePath()), 0o700); err != nil {
			t.Fatal(err)
		}
		exec(t, c.StorePath(), "CREATE TABLE something (x)")
		if s := c.Summarize(ctx); s.State != querycache.StateUnavailable || !strings.Contains(s.Hint, c.StorePath()) {
			t.Errorf("%+v", s)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		c := New(t.TempDir(), Options{Disabled: "turned off"})
		if s := c.Summarize(ctx); s.State != querycache.StateDisabled || s.Reason != "turned off" || s.Store != "" {
			t.Errorf("%+v", s)
		}
		if _, _, err := c.Open(ctx); !errors.Is(err, ErrDisabled) {
			t.Errorf("Open of a disabled cache: %v", err)
		}
	})
}

// A new store's lake folder already holding files is reported with their size;
// they are never deleted. An existing store's isn't.
func TestOpenReportsOldFiles(t *testing.T) {
	c := newCache(t)
	if err := os.MkdirAll(filepath.Join(c.LakeDir(), "a"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.LakeDir(), "a", "1.parquet"), make([]byte, 2048), 0o600); err != nil {
		t.Fatal(err)
	}
	_, notices, err := c.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 || notices[0].Code != NoticeOrphanedFiles || !strings.Contains(notices[0].Message, "2.0 KB") || !strings.Contains(notices[0].Message, c.LakeDir()) {
		t.Errorf("notices %+v", notices)
	}
	if _, err := os.Stat(filepath.Join(c.LakeDir(), "a", "1.parquet")); err != nil {
		t.Errorf("the old file is gone: %v", err)
	}

	c.Close(ctx)
	if _, notices, _ := c.Open(ctx); len(notices) != 0 {
		t.Errorf("an existing store's folder reported: %+v", notices)
	}
}

// A store that fails to open is tried again at the next Open.
func TestOpenRetries(t *testing.T) {
	c := newCache(t)
	if err := os.MkdirAll(c.StorePath(), 0o700); err != nil { // a folder where the file goes
		t.Fatal(err)
	}
	if _, _, err := c.Open(ctx); err == nil {
		t.Fatal("opened a folder as the store")
	}
	if err := os.Remove(c.StorePath()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Open(ctx); err != nil {
		t.Errorf("the next Open: %v", err)
	}
}

// A namespace becomes the cache's file prefix, ending in /. One canal-lake
// can't take leaves the cache unavailable, saying how to fix it, rather than
// failing.
func TestNamespace(t *testing.T) {
	for given, want := range map[string]string{"": "", "prod": "prod/", "prod/eu/": "prod/eu/"} {
		if got := New(t.TempDir(), Options{Namespace: given}).Namespace(); got != want {
			t.Errorf("namespace %q: %q, want %q", given, got, want)
		}
	}
	for _, bad := range []string{"my-prod", "../x", "/prod", "a//b"} {
		c := New(t.TempDir(), Options{Namespace: bad})
		if s := c.Summarize(ctx); s.State != querycache.StateUnavailable || !strings.Contains(s.Problem, NamespaceEnv) || !strings.Contains(s.Hint, "letters, digits and _") {
			t.Errorf("%q: %+v", bad, s)
		}
		if _, _, err := c.Open(ctx); !errors.Is(err, errBadNamespace) || !strings.Contains(err.Error(), bad) {
			t.Errorf("%q: Open %v", bad, err)
		}
		if err := c.StartLake(ctx); !errors.Is(err, errBadNamespace) {
			t.Errorf("%q: StartLake %v", bad, err)
		}
	}
}
