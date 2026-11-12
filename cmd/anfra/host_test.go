package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/envflag"
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
			got := queryCacheDisabled(tc.repo, logger)
			if (got == "") != (tc.off == "") || !strings.Contains(got, tc.off) {
				t.Errorf("reason %q, want one naming %q", got, tc.off)
			}
		})
	}
}
