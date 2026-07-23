// Package update implements anfra's self-update: discover the latest GitHub
// release, compare it to the compiled-in version, download the binary for this
// platform, and atomically replace the running executable.
//
// Follows the common CLI pattern (gh, deno, bun): a manual `anfra update` command
// plus a cached, non-blocking "update available" notice. Fully-automatic update is
// opt-in (ANFRA_AUTO_UPDATE), never the default.
package update

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/meta"
	"github.com/minio/selfupdate"
	"golang.org/x/mod/semver"
)

const (
	// The GitHub repo whose Releases hold the anfra binaries.
	repoSlug  = "holistics/anfra"
	tagPrefix = "anfra-v"
	apiBase   = "https://api.github.com"
	userAgent = "anfra-cli"

	// How long a cached update check stays fresh (matches gh/deno's 24h).
	checkTTL = 24 * time.Hour

	// Per-request timeouts: the release lookup is a small JSON response; the asset
	// download is a large binary (sidecars are embedded), so it gets much longer.
	lookupTimeout   = 30 * time.Second
	downloadTimeout = 10 * time.Minute
)

// Release is the latest release plus the asset for the running platform.
type Release struct {
	Version  string // e.g. "0.2.0" (tag without the anfra-v prefix)
	Tag      string // e.g. "anfra-v0.2.0"
	AssetURL string // GitHub API asset URL (octet-stream download) for this platform
}

// assetName maps the running platform to its release asset name (anfra-<target>),
// matching build_release.yml's per-target output. Windows is not built yet.
func assetName() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "anfra-linux-x64", nil
	case "linux/arm64":
		return "anfra-linux-arm64", nil
	case "darwin/amd64":
		return "anfra-darwin-x64", nil
	case "darwin/arm64":
		return "anfra-darwin-arm64", nil
	default:
		return "", fmt.Errorf("no anfra release build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// authToken returns a GitHub token from the environment, or "" for anonymous.
// Used only as a fallback when an anonymous request is rejected (the repo/releases
// are private until public distribution).
func authToken() string {
	for _, k := range []string{"ANFRA_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// get does an anonymous GET; if it's rejected (401/403/404) and a token is
// available, it retries authenticated. accept selects the response format;
// timeout bounds the whole exchange (including reading the body).
func get(ctx context.Context, url, accept string, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{Timeout: timeout}
	do := func(token string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", userAgent)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return client.Do(req)
	}

	resp, err := do("")
	if err != nil {
		return nil, err
	}
	// Anonymous rejected + a token on hand → retry authenticated (private repo).
	if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 {
		if token := authToken(); token != "" {
			resp.Body.Close()
			return do(token)
		}
	}
	return resp, nil
}

// Latest fetches the newest release and resolves the asset for this platform.
func Latest(ctx context.Context) (*Release, error) {
	base, err := assetName()
	if err != nil {
		return nil, err
	}
	// Release binaries are published gzip-compressed for transport (see the
	// compression plan); Apply decompresses on the way in.
	want := base + ".gz"
	resp, err := get(ctx, fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repoSlug), "application/vnd.github+json", lookupTimeout)
	if err != nil {
		return nil, fmt.Errorf("check latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("check latest release: GitHub returned %s (set ANFRA_GITHUB_TOKEN if the repo is private)", resp.Status)
	}

	var rel struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	out := &Release{Tag: rel.TagName, Version: strings.TrimPrefix(rel.TagName, tagPrefix)}
	for _, a := range rel.Assets {
		if a.Name == want {
			out.AssetURL = a.URL
			break
		}
	}
	if out.AssetURL == "" {
		return nil, fmt.Errorf("release %s has no asset %q", rel.TagName, want)
	}
	return out, nil
}

// IsNewer reports whether the latest version is newer than the running one. A
// local "dev" build is always considered outdated so the notice/update fires.
func (r *Release) IsNewer() bool {
	cur := meta.Version
	if cur == "dev" || cur == "" {
		return true
	}
	// Normalize to canonical vX.Y.Z so a stray "v"/"anfra-v" prefix on either side
	// can't produce an invalid string (e.g. "vv0.2.0"), which Compare treats as 0.
	return semver.Compare(canonicalVersion(r.Version), canonicalVersion(cur)) > 0
}

// canonicalVersion strips any anfra-v / v prefix and re-adds a single "v", so
// semver.Compare always sees a valid version regardless of the input format.
func canonicalVersion(v string) string {
	v = strings.TrimPrefix(v, tagPrefix)
	v = strings.TrimPrefix(v, "v")
	return "v" + v
}

// Apply downloads the platform asset and atomically replaces the running binary.
// If progress is non-nil, download progress is rendered to it (a single, in-place
// updated line); pass nil for a silent download (agents/CI).
func Apply(ctx context.Context, r *Release, progress io.Writer) error {
	resp, err := get(ctx, r.AssetURL, "application/octet-stream", downloadTimeout)
	if err != nil {
		return fmt.Errorf("download %s: %w", r.Tag, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: GitHub returned %s", r.Tag, resp.Status)
	}
	// Progress tracks the compressed bytes actually transferred; gzip.Reader then
	// decompresses to the real binary for the self-replace (embed stays uncompressed).
	var src io.Reader = resp.Body
	if progress != nil {
		src = &progressReader{r: resp.Body, total: resp.ContentLength, w: progress}
	}
	gz, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("decompress %s: %w", r.Tag, err)
	}
	defer gz.Close()
	if err := selfupdate.Apply(gz, selfupdate.Options{}); err != nil {
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return fmt.Errorf("update failed and rollback also failed: %v (rollback: %v)", err, rerr)
		}
		return fmt.Errorf("apply update: %w", err)
	}
	return nil
}

// progressReader wraps the download stream and renders a single, carriage-return
// updated progress line as bytes flow through. It redraws only when the whole
// percent changes (or every ~1 MB when the total is unknown), so it stays cheap.
type progressReader struct {
	r       io.Reader
	w       io.Writer
	total   int64 // -1 if the server didn't send Content-Length
	read    int64
	lastPct int
	lastN   int64
	done    bool
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.read += int64(n)
		p.maybeRender()
	}
	if err == io.EOF && !p.done {
		p.done = true
		p.render()
		fmt.Fprintln(p.w)
	}
	return n, err
}

func (p *progressReader) maybeRender() {
	if p.total > 0 {
		if pct := int(p.read * 100 / p.total); pct != p.lastPct {
			p.lastPct = pct
			p.render()
		}
		return
	}
	if p.read-p.lastN >= 1<<20 {
		p.lastN = p.read
		p.render()
	}
}

func (p *progressReader) render() {
	if p.total > 0 {
		fmt.Fprintf(p.w, "\r  downloading %3d%% (%s / %s)   ", p.read*100/p.total, humanBytes(p.read), humanBytes(p.total))
		return
	}
	fmt.Fprintf(p.w, "\r  downloading %s   ", humanBytes(p.read))
}

// humanBytes renders a byte count as a human-readable size (e.g. "117.2 MB").
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// --- cached background check (for the "update available" notice) ---

type cache struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
	LatestTag     string    `json:"latest_tag"`
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "anfra", "update-check.json"), nil
}

func readCache() (*cache, bool) {
	p, err := cachePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(p) //nolint:gosec // G304: our own cache path, not user input
	if err != nil {
		return nil, false
	}
	var c cache
	if json.Unmarshal(data, &c) != nil {
		return nil, false
	}
	return &c, true
}

func writeCache(c *cache) {
	p, err := cachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { //nolint:gosec // G301: cache dir
		return
	}
	if data, err := json.Marshal(c); err == nil {
		_ = os.WriteFile(p, data, 0o644) //nolint:gosec // G306: non-secret cache file
	}
}

// Stale reports whether the cached check is missing or older than the TTL.
func Stale() bool {
	c, ok := readCache()
	return !ok || time.Since(c.CheckedAt) > checkTTL
}

// Refresh runs a live check and updates the cache. Called from the detached
// background refresh (see cmd/anfra), never in a command's hot path.
func Refresh(ctx context.Context) error {
	rel, err := Latest(ctx)
	if err != nil {
		return err
	}
	RecordCheck(rel)
	return nil
}

// RecordCheck stores an already-fetched release in the cache (no network), so a
// foreground check refreshes the notice cache without a second request.
func RecordCheck(r *Release) {
	writeCache(&cache{CheckedAt: time.Now(), LatestVersion: r.Version, LatestTag: r.Tag})
}

// CachedNotice returns a one-line "update available" message from the cached
// check, or "" if none is available / the cache says we're current.
func CachedNotice() string {
	c, ok := readCache()
	if !ok || c.LatestVersion == "" {
		return ""
	}
	r := &Release{Version: c.LatestVersion, Tag: c.LatestTag}
	if !r.IsNewer() {
		return ""
	}
	return fmt.Sprintf("anfra %s is available (you have %s). Run `anfra update`.", c.LatestTag, meta.Version)
}
