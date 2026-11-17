package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/envflag"
	localquerycache "github.com/holistics/anfra/internal/querycache/local"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// The repo's query.cache.enabled turns the query cache off, and
// ANFRA_QUERY_CACHE_ENABLED overrides it either way.
func TestQueryCacheDisabled(t *testing.T) {
	repoOff := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoOff, ".anfra"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoOff, ".anfra", "config.yml"), []byte("query:\n  cache:\n    enabled: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repoOn := t.TempDir()

	for _, tc := range []struct {
		name, repo, env string
		off             string // what the reason names; empty: on
	}{
		{"default", repoOn, "", ""},
		{"the repo turns it off", repoOff, "", "query.cache.enabled: false"},
		{"the environment turns it off", repoOn, "0", envflag.QueryCacheEnabled + "=0"},
		{"the environment turns it on over the repo", repoOff, "1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envflag.QueryCacheEnabled, tc.env)
			got := queryCacheDisabled(repoconfig.Read(tc.repo))
			if (got == "") != (tc.off == "") || !strings.Contains(got, tc.off) {
				t.Errorf("reason %q, want one naming %q", got, tc.off)
			}
		})
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
			t.Setenv(envflag.QueryCacheEnabled, tc.enabled)
			c := newQueryCache(r, sidecar.Config{Logger: logger, CanalQueryURL: tc.url}, repoconfig.Config{}, nil)
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

// A repo's defaults and locks reach every command; a settings file anfra
// can't use refuses every query, rather than leaving the defaults.
func TestQueryDefaults(t *testing.T) {
	if got := queryDefaults(repoconfig.Config{}, nil); got.Err != nil || got.Timezone != "" || got.CacheDuration != 0 || got.Locked != nil {
		t.Errorf("no file: %+v, want anfra's defaults", got)
	}
	var c repoconfig.Config
	c.Query.Default.Input.WeekStartDay, c.Query.Default.Cache.Duration = "sunday", "90m"
	c.Query.LockedDefaults = []string{"input.weekStartDay"}
	if got := queryDefaults(c, nil); got.WeekStartDay != "sunday" || got.CacheDuration != 90*time.Minute || !slices.Equal(got.Locked, c.Query.LockedDefaults) {
		t.Errorf("settings: %+v", got)
	}
	broken := errors.New(".anfra/config.yml line 1: x isn't a setting.")
	if got := queryDefaults(c, broken); !errors.Is(got.Err, broken) || got.WeekStartDay != "" {
		t.Errorf("a broken file: %+v, want its error and nothing else", got)
	}
}
