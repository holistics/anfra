package update

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/meta"
)

func TestIsNewer(t *testing.T) {
	orig := meta.Version
	t.Cleanup(func() { meta.Version = orig })

	cases := []struct {
		current string
		latest  string
		want    bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.1.0", "0.1.1", true},
		{"0.2.0", "0.1.0", false},
		{"0.1.0", "0.1.0", false},
		// A build from source is never offered a release in its place, older or newer.
		{"0.1.0+f4fb358", "0.2.0", false},
		{"0.0.0+source", "0.1.0", false},
		{"1.0.0", "0.9.9", false},
		// Mixed prefixes must normalize, not degrade to a 0/invalid compare.
		{"v0.1.0", "0.2.0", true},
		{"0.1.0", "v0.2.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"anfra-v0.1.0", "0.2.0", true},
	}
	for _, c := range cases {
		meta.Version = c.current
		got := (&Release{Version: c.latest}).IsNewer()
		if got != c.want {
			t.Errorf("current=%q latest=%q: IsNewer()=%v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestTagVersion(t *testing.T) {
	for tag, want := range map[string]string{
		"v0.9.0":       "0.9.0",
		"anfra-v0.8.0": "0.8.0", // releases up to 0.8.0
		"0.9.0":        "0.9.0",
	} {
		if got := tagVersion(tag); got != want {
			t.Errorf("tagVersion(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestProgressReader(t *testing.T) {
	// Drain 1000 bytes with a known total through the progress reader.
	var buf bytes.Buffer
	pr := &progressReader{r: bytes.NewReader(make([]byte, 1000)), w: &buf, total: 1000}
	n, err := io.Copy(io.Discard, pr)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n != 1000 {
		t.Fatalf("copied %d bytes, want 1000", n)
	}
	out := buf.String()
	if !strings.Contains(out, "downloading") {
		t.Errorf("no progress rendered: %q", out)
	}
	if !strings.Contains(out, "100%") {
		t.Errorf("final progress not 100%%: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("progress did not end with a newline: %q", out)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:               "512 B",
		1024:              "1.0 KB",
		272 * 1024 * 1024: "272.0 MB",
	}
	for n, want := range cases {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d)=%q, want %q", n, got, want)
		}
	}
}

func TestAssetName(t *testing.T) {
	// The mapping must stay in lockstep with build_release.yml's per-target names.
	name, err := assetName()
	if err != nil {
		// Only fails on a platform anfra isn't built for; skip rather than fail.
		t.Skipf("no build for this platform: %v", err)
	}
	if name == "" {
		t.Fatal("assetName returned empty name with no error")
	}
}

// fakeGitHub serves the latest release, v9.0.0, as GitHub's site does, and the
// skills' manifest, as a raw file with the status and body given. It points
// the bases and anfra's folder at the test's.
func fakeGitHub(t *testing.T, manifestStatus int, manifest string) {
	t.Helper()
	asset, err := assetName()
	if err != nil {
		t.Skip(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/holistics/anfra/releases/latest":
			http.Redirect(w, r, srv.URL+"/holistics/anfra/releases/tag/v9.0.0", http.StatusFound)
		case "/holistics/anfra/releases/download/v9.0.0/" + asset + ".gz":
			http.Redirect(w, r, "https://objects.example.com/anfra", http.StatusFound)
		case "/holistics/anfra-skills/main/plugins/anfra/.claude-plugin/plugin.json":
			w.WriteHeader(manifestStatus)
			fmt.Fprint(w, manifest)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	setBases(t, srv.URL)
	t.Setenv("ANFRA_HOME", t.TempDir())
}

func TestLatestSkills(t *testing.T) {
	fakeGitHub(t, http.StatusOK, `{"name": "anfra", "version": "0.4.5"}`)
	if got, err := LatestSkills(context.Background()); err != nil || got != "0.4.5" {
		t.Errorf("got %q, %v", got, err)
	}

	for name, tc := range map[string]struct {
		status   int
		manifest string
	}{
		"not found":     {http.StatusNotFound, `{}`},
		"not a version": {http.StatusOK, `{"version": "0.4"}`},
		"no version":    {http.StatusOK, `{"name": "anfra"}`},
		"not JSON":      {http.StatusOK, `version: 0.4.5`},
	} {
		t.Run(name, func(t *testing.T) {
			fakeGitHub(t, tc.status, tc.manifest)
			if got, err := LatestSkills(context.Background()); err == nil {
				t.Errorf("got %q, want an error", got)
			}
		})
	}
}

// Refresh caches both checks; the skills' failing is cached as unknown, never
// as a version, and doesn't stop anfra's.
func TestRefreshCachesTheSkills(t *testing.T) {
	fakeGitHub(t, http.StatusOK, `{"version": "0.4.5"}`)
	if err := Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if latest, at, ok := CachedSkills(); !ok || latest != "0.4.5" || time.Since(at) > time.Minute {
		t.Errorf("skills: %q, %v, %v", latest, at, ok)
	}
	if r := Cached(); r == nil || r.Version != "9.0.0" {
		t.Errorf("anfra: %+v", r)
	}
	if Stale() {
		t.Error("stale right after a refresh")
	}

	// anfra update records anfra's check alone, and keeps the skills'.
	RecordCheck(&Release{Version: "9.0.1", Tag: "v9.0.1"})
	if latest, _, ok := CachedSkills(); !ok || latest != "0.4.5" {
		t.Errorf("skills after RecordCheck: %q, %v", latest, ok)
	}

	fakeGitHub(t, http.StatusNotFound, `{}`)
	if err := Refresh(context.Background()); err == nil {
		t.Error("a failed skills check: got no error")
	}
	if latest, _, ok := CachedSkills(); ok {
		t.Errorf("a failed check is cached as %q", latest)
	}
	if r := Cached(); r == nil || r.Version != "9.0.0" {
		t.Errorf("anfra's check after the skills' failed: %+v", r)
	}
	if Stale() {
		t.Error("a failed skills check is retried on every command")
	}
}

// A cache from before the skills' check is stale, so the next background
// refresh fills it in.
func TestACacheWithoutTheSkillsIsStale(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	writeCache(&cache{CheckedAt: time.Now(), LatestVersion: "9.0.0", LatestTag: "v9.0.0"})
	if !Stale() {
		t.Error("not stale")
	}
	if _, _, ok := CachedSkills(); ok {
		t.Error("skills known")
	}
}

// serve's Check goes by anfra's own check: skills never checked don't send it
// to GitHub again.
func TestCheckIgnoresTheSkills(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	setBases(t, "http://127.0.0.1:1") // nothing listens: a lookup would fail
	writeCache(&cache{CheckedAt: time.Now(), LatestVersion: "9.0.0", LatestTag: "v9.0.0"})
	if r, err := Check(context.Background()); err != nil || r.Version != "9.0.0" {
		t.Errorf("got %+v, %v; want the cached release", r, err)
	}
}

// setBases points GitHub's site and raw files at base, for the test.
func setBases(t *testing.T, base string) {
	t.Helper()
	oldWeb, oldRaw := webBase, rawBase
	webBase, rawBase = base, base
	t.Cleanup(func() { webBase, rawBase = oldWeb, oldRaw })
}

// Latest reads the tag from the site's redirect, and the asset's download
// from its fixed path, with no API call.
func TestLatestFromTheSite(t *testing.T) {
	fakeGitHub(t, http.StatusOK, `{"version": "0.4.5"}`)
	r, err := Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	asset, _ := assetName()
	if r.Tag != "v9.0.0" || r.Version != "9.0.0" || r.AssetURL != webBase+"/holistics/anfra/releases/download/v9.0.0/"+asset+".gz" {
		t.Errorf("got %+v", r)
	}
}

// A release whose asset for this platform isn't uploaded yet isn't the latest.
func TestLatestWithoutTheAsset(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/holistics/anfra/releases/latest" {
			http.Redirect(w, r, srv.URL+"/holistics/anfra/releases/tag/v9.0.0", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	setBases(t, srv.URL)
	if _, err := Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "anfra 9.0.0 has no build for this platform") {
		t.Errorf("got %v, want no build", err)
	}
}

// A latest-release page that points at no release says so, naming the page.
func TestLatestWithoutARelease(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setBases(t, srv.URL)
	_, err := Latest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "check for a newer anfra: "+srv.URL+"/holistics/anfra/releases/latest answered 404 Not Found, without pointing at a release") {
		t.Errorf("got %v", err)
	}
}
