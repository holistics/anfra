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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/home"
	"github.com/holistics/anfra/shared/jsonkit"

	"github.com/holistics/anfra/internal/meta"
	"github.com/minio/selfupdate"
	"golang.org/x/mod/semver"
)

const (
	// The GitHub repo whose Releases hold the anfra binaries.
	repoSlug = "holistics/anfra"
	// Release tags are v<version>; those up to 0.8.0 were anfra-v<version>.
	tagPrefix       = "v"
	legacyTagPrefix = "anfra-v"
	userAgent       = "anfra-cli"

	// The anfra skills' latest version is their plugin manifest's on the skills
	// repo's main: the skills repo bumps every manifest in lockstep on each
	// release (docs/designs/agent-setup.md).
	skillsRepo     = "holistics/anfra-skills"
	skillsManifest = "plugins/anfra/.claude-plugin/plugin.json"
	skillsRef      = "main"

	// How long a cached update check stays fresh (matches gh/deno's 24h).
	checkTTL = 24 * time.Hour

	// Per-request timeouts: the release lookup is a small JSON response; the asset
	// download is a large binary (sidecars are embedded), so it gets much longer.
	lookupTimeout   = 30 * time.Second
	downloadTimeout = 10 * time.Minute
)

// apiBase is GitHub's API, and rawBase where it serves a repo's files, which a
// test points at its own server.
var (
	apiBase = "https://api.github.com"
	rawBase = "https://raw.githubusercontent.com"
)

// Release is the latest release plus the asset for the running platform.
type Release struct {
	Version  string // e.g. "0.2.0" (the tag without its prefix)
	Tag      string // e.g. "v0.2.0"
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
	if err := jsonkit.UnmarshalRead(resp.Body, &rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	out := &Release{Tag: rel.TagName, Version: tagVersion(rel.TagName)}
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

// LatestSkills fetches the latest anfra skills version: the version in the
// skills repo's plugin manifest on main. It is a file GitHub serves from its
// CDN, not its API, so it doesn't count against the API's rate limit, which
// everyone behind one address shares (60 an hour, anonymous). Anonymous, then
// with a token if that is refused, as the release lookup.
func LatestSkills(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/%s/%s/%s", rawBase, skillsRepo, skillsRef, skillsManifest)
	resp, err := get(ctx, url, "application/json", lookupTimeout)
	if err != nil {
		return "", fmt.Errorf("check the latest anfra skills: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check the latest anfra skills: GitHub returned %s", resp.Status)
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := jsonkit.UnmarshalRead(io.LimitReader(resp.Body, 1<<20), &m); err != nil {
		return "", fmt.Errorf("check the latest anfra skills: decode %s: %w", skillsManifest, err)
	}
	if semver.Canonical("v"+m.Version) != "v"+m.Version {
		return "", fmt.Errorf("check the latest anfra skills: %q in %s is not a version", m.Version, skillsManifest)
	}
	return m.Version, nil
}

// IsNewer reports whether the latest version is newer than the running one, so
// worth announcing and installing. Never for a build from source: it was built
// on purpose, and a release would replace it with something else.
func (r *Release) IsNewer() bool {
	cur := meta.Version
	if meta.FromSource() {
		return false
	}
	// Normalize to canonical vX.Y.Z so a stray "v"/"anfra-v" prefix on either side
	// can't produce an invalid string (e.g. "vv0.2.0"), which Compare treats as 0.
	return semver.Compare(canonicalVersion(r.Version), canonicalVersion(cur)) > 0
}

// tagVersion strips a release tag's prefix, v or anfra-v, if it has one.
func tagVersion(tag string) string {
	return strings.TrimPrefix(strings.TrimPrefix(tag, legacyTagPrefix), tagPrefix)
}

// canonicalVersion strips any anfra-v / v prefix and re-adds a single "v", so
// semver.Compare always sees a valid version regardless of the input format.
func canonicalVersion(v string) string {
	return "v" + tagVersion(v)
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

// cache is the last background check: anfra's latest release, and the anfra
// skills' latest version, each with when it was checked. They are checked
// together, but `anfra update` records anfra's alone.
type cache struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
	LatestTag     string    `json:"latest_tag"`
	// SkillsLatest is "" when the last fetch failed: a failure is cached as
	// unknown, never as a version, so it can't pass for current.
	SkillsLatest    string    `json:"skills_latest,omitempty"`
	SkillsCheckedAt time.Time `json:"skills_checked_at,omitzero"`
}

func cachePath() (string, error) {
	return filepath.Join(home.Dir(), "update-check.json"), nil
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
	if jsonkit.Unmarshal(data, &c) != nil {
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
	if data, err := jsonkit.Marshal(c); err == nil {
		_ = os.WriteFile(p, data, 0o644) //nolint:gosec // G306: non-secret cache file
	}
}

// Stale reports whether the cached check, of anfra or of the skills, is missing
// or older than the TTL.
func Stale() bool {
	c, ok := readCache()
	return !ok || time.Since(c.CheckedAt) > checkTTL || time.Since(c.SkillsCheckedAt) > checkTTL
}

// Refresh runs a live check, of anfra and of the skills, and updates the cache.
// One failing doesn't stop the other. Called from the detached background
// refresh (see cmd/anfra), never in a command's hot path.
func Refresh(ctx context.Context) error {
	rel, err := Latest(ctx)
	if err == nil {
		RecordCheck(rel)
	}
	skills, serr := LatestSkills(ctx)
	c, _ := readCache()
	if c == nil {
		c = &cache{}
	}
	c.SkillsLatest, c.SkillsCheckedAt = skills, time.Now()
	writeCache(c)
	return errors.Join(err, serr)
}

// RecordCheck stores an already-fetched release in the cache (no network), so a
// foreground check refreshes the notice cache without a second request. It
// keeps the skills' check as it was.
func RecordCheck(r *Release) {
	c, _ := readCache()
	if c == nil {
		c = &cache{}
	}
	c.CheckedAt, c.LatestVersion, c.LatestTag = time.Now(), r.Version, r.Tag
	writeCache(c)
}

// CachedSkills is the latest anfra skills version from the cached check, and
// when it was checked; false when it wasn't, or the fetch failed. No network.
func CachedSkills() (latest string, checkedAt time.Time, ok bool) {
	c, found := readCache()
	if !found || c.SkillsLatest == "" {
		return "", time.Time{}, false
	}
	return c.SkillsLatest, c.SkillsCheckedAt, true
}

// Cached returns the latest release from the cached check (no network), or nil
// when there is none. Its AssetURL is empty: it can be reported, not applied.
func Cached() *Release {
	c, ok := readCache()
	if !ok || c.LatestVersion == "" {
		return nil
	}
	return &Release{Version: c.LatestVersion, Tag: c.LatestTag}
}

// Check returns the latest release: the cached one while it is fresh, else a
// live lookup, which it caches. For a long-running process (serve), which can
// afford the request off its hot path; a short command uses Cached and Refresh.
func Check(ctx context.Context) (*Release, error) {
	// anfra's check alone: serve doesn't check the skills.
	if c, ok := readCache(); ok && c.LatestVersion != "" && time.Since(c.CheckedAt) <= checkTTL {
		return &Release{Version: c.LatestVersion, Tag: c.LatestTag}, nil
	}
	r, err := Latest(ctx)
	if err != nil {
		return nil, err
	}
	RecordCheck(r)
	return r, nil
}

// Notice is the one-line "update available" message for r, or "" when r is not
// newer than the running version.
func Notice(r *Release) string {
	if r == nil || !r.IsNewer() {
		return ""
	}
	return fmt.Sprintf("anfra %s is available (you have %s). Run `anfra update`.", r.Version, meta.Version)
}

// CachedNotice returns a one-line "update available" message from the cached
// check, or "" if none is available / the cache says we're current.
func CachedNotice() string {
	return Notice(Cached())
}
