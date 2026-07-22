package update

import (
	"bytes"
	"io"
	"strings"
	"testing"

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
		{"dev", "0.1.0", true}, // local build is always outdated
		{"", "0.1.0", true},    // unset behaves like dev
		{"1.0.0", "0.9.9", false},
	}
	for _, c := range cases {
		meta.Version = c.current
		got := (&Release{Version: c.latest}).IsNewer()
		if got != c.want {
			t.Errorf("current=%q latest=%q: IsNewer()=%v, want %v", c.current, c.latest, got, c.want)
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
