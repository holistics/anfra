package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/envflag"
	localquerycache "github.com/holistics/anfra/internal/querycache/local"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// The repo's cache.query.enabled turns the query cache off, and
// ANFRA_CACHE_QUERY_ENABLED overrides it either way.
func TestQueryCacheDisabled(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	repoOff := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoOff, ".anfra"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoOff, ".anfra", "config.yml"), []byte("cache:\n  query:\n    enabled: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repoOn := t.TempDir()

	for _, tc := range []struct {
		name, repo, env string
		off             string // what the reason names; empty: on
	}{
		{"default", repoOn, "", ""},
		{"the repo turns it off", repoOff, "", "cache.query.enabled: false"},
		{"the environment turns it off", repoOn, "0", envflag.CacheQueryEnabled + "=0"},
		{"the environment turns it on over the repo", repoOff, "1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envflag.CacheQueryEnabled, tc.env)
			got, _ := queryCacheSettings(tc.repo, logger)
			if (got == "") != (tc.off == "") || !strings.Contains(got, tc.off) {
				t.Errorf("reason %q, want one naming %q", got, tc.off)
			}
		})
	}
}

// cache.query.default.duration is read whatever turns the cache off; one anfra
// doesn't take leaves none, and doesn't hide cache.query.enabled.
func TestQueryCacheDefaultDuration(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	write := func(yml string) string {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".anfra"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".anfra", "config.yml"), []byte(yml), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	t.Setenv(envflag.CacheQueryEnabled, "")
	if _, d := queryCacheSettings(write("cache:\n  query:\n    default:\n      duration: 90m\n"), logger); d != 90*time.Minute {
		t.Errorf("default duration %v, want 90m", d)
	}
	for _, bad := range []string{"1d", "200h", "-1m"} {
		off, d := queryCacheSettings(write("cache:\n  query:\n    enabled: false\n    default:\n      duration: "+bad+"\n"), logger)
		if d != 0 || off == "" {
			t.Errorf("%s: default %v, disabled %q; want none, and the cache still off", bad, d, off)
		}
	}
}

// canal-query is given a canal-lake socket only when anfra runs canal-lake:
// with the cache on, and canal-query its own. A deployment's canal-query
// reaches the deployment's canal-lake, so starting it is left to them.
func TestNewQueryCache(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	logger := slog.New(slog.DiscardHandler)
	r := repo.Resolve(t.TempDir())
	for _, tc := range []struct {
		name, enabled, url string
		socket             bool
	}{
		{"on", "1", "", true},
		{"off", "0", "", false},
		{"a deployment's canal-query", "1", "http://canal-query:1323", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envflag.CacheQueryEnabled, tc.enabled)
			c := newQueryCache(r, sidecar.Config{Logger: logger, CanalQueryURL: tc.url})
			defer c.Close(t.Context())
			if (c.lakeSocket != "") != tc.socket {
				t.Errorf("canal-lake's socket %q, want one: %v", c.lakeSocket, tc.socket)
			}
			if tc.url != "" {
				if err := c.StartLake(t.Context()); err != nil {
					t.Errorf("StartLake with a deployment's canal-lake: %v", err)
				}
			}
		})
	}
}

// anfra serve's sweep removes the query cache's results expired past the read
// grace, their files through canal-query, and logs what it freed; a failed
// drop is logged, not returned, so the next run tries again.
func TestSweepQueryCache(t *testing.T) {
	ctx := context.Background()
	fail := false
	canal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer canal.Close()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	qc := localquerycache.New(filepath.Join(t.TempDir(), "query"), localquerycache.Options{Lake: deployedLake{}})
	t.Cleanup(func() { qc.Close(ctx) })
	h := hostContext{queryCache: qc, cfg: sidecar.Config{Logger: logger}}
	m, err := qc.Manager(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expiredLongAgo := func() {
		w, err := m.Begin(ctx, "k", nil, "", "select 1")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Commit(ctx, 1, 7, nil, -time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	sweep := sweepQueryCache(h, canalquery.NewClient(canal.URL, false))

	expiredLongAgo()
	if err := sweep(ctx); err != nil || !strings.Contains(logs.String(), `"event":"query_cache.swept"`) || !strings.Contains(logs.String(), `"bytes":7`) {
		t.Errorf("a sweep: %v, logged %s", err, logs.String())
	}
	logs.Reset()
	if err := sweep(ctx); err != nil || logs.Len() != 0 {
		t.Errorf("nothing to sweep: %v, logged %s", err, logs.String())
	}
	fail = true
	expiredLongAgo()
	if err := sweep(ctx); err != nil || !strings.Contains(logs.String(), `"event":"query_cache.sweep_failed"`) {
		t.Errorf("a failed drop: %v, logged %s", err, logs.String())
	}
}
