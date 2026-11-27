package update

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packAsset is content as a release publishes this platform's asset.
func packAsset(t *testing.T, content []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	if _, err := zw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fakeRelease serves asset under its name, and sums as the release's
// SHA256SUMS ("" for none), and answers the Release that points at them.
func fakeRelease(t *testing.T, asset []byte, sums string) *Release {
	t.Helper()
	name := assetFile(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dl/v9.0.0/" + name:
			_, _ = w.Write(asset)
		case "/dl/v9.0.0/SHA256SUMS":
			if sums == "" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &Release{Version: "9.0.0", Tag: "v9.0.0", AssetURL: srv.URL + "/dl/v9.0.0/" + name}
}

// assetFile is this platform's asset file name in a release.
func assetFile(t *testing.T) string {
	t.Helper()
	name, err := assetName()
	if err != nil {
		t.Skip(err)
	}
	if !strings.Contains(name, ".") {
		name += ".gz"
	}
	return name
}

func sumOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// target is a stand-in for the running anfra, holding "old", with the temp
// folder of its own so a leftover download shows.
func target(t *testing.T) (path, tmp string) {
	t.Helper()
	tmp = t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	path = filepath.Join(t.TempDir(), "anfra")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil { //nolint:gosec // G306: a stand-in for an executable
		t.Fatal(err)
	}
	return path, tmp
}

func checkNoDownloadLeft(t *testing.T, tmp string) {
	t.Helper()
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("left in the temp folder: %v", entries)
	}
}

// An asset whose sha256 is the release's applies.
func TestApplyVerifiesTheChecksum(t *testing.T) {
	asset := packAsset(t, []byte("new"))
	r := fakeRelease(t, asset, sumOf(asset)+"  "+assetFile(t)+"\n"+sumOf([]byte("x"))+"  other.gz\n")
	path, tmp := target(t)
	if err := applyTo(context.Background(), r, nil, path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" { //nolint:gosec // G304: the test's own file
		t.Errorf("target holds %q, want the new anfra", b)
	}
	checkNoDownloadLeft(t, tmp)
}

// A wrong checksum, no SHA256SUMS, or no line for the asset refuses: nothing
// is replaced, and the refusal says how to install by hand.
func TestApplyRefusesWhatItCantCheck(t *testing.T) {
	asset := packAsset(t, []byte("new"))
	name := assetFile(t)
	for _, c := range []struct{ name, sums, want string }{
		{"wrong sum", sumOf([]byte("tampered")) + "  " + name + "\n", "doesn't match"},
		{"no SHA256SUMS", "", "no checksum"},
		{"no line", sumOf(asset) + "  other.gz\n", "no checksum"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := fakeRelease(t, asset, c.sums)
			path, tmp := target(t)
			err := applyTo(context.Background(), r, nil, path)
			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "install") {
				t.Fatalf("got %v, want a refusal saying %q and how to install", err, c.want)
			}
			if b, _ := os.ReadFile(path); string(b) != "old" { //nolint:gosec // G304: the test's own file
				t.Errorf("target holds %q, want it untouched", b)
			}
			checkNoDownloadLeft(t, tmp)
		})
	}
}
