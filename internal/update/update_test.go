package update

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/holistics/anfra/internal/cliui"
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

func TestBanner(t *testing.T) {
	orig := meta.Version
	t.Cleanup(func() { meta.Version = orig })
	meta.Version = "0.9.1"

	r := &Release{Version: "0.11.0"}
	want := "▲ Update available  0.9.1 → 0.11.0\n  Run `anfra update` to install."
	if got := Banner(r, cliui.TerminalStyles{}); got != want {
		t.Errorf("Banner(plain) = %q, want %q", got, want)
	}
	styled := cliui.TerminalStyles{
		Heading: lipgloss.NewStyle().Bold(true),
		Muted:   lipgloss.NewStyle().Faint(true),
		Success: lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)),
		Warning: lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)),
		Code:    lipgloss.NewStyle().Bold(true),
	}
	got := Banner(r, styled)
	if !strings.Contains(got, "\033[") || ansi.Strip(got) != want {
		t.Errorf("Banner(styled) = %q, want escape codes around %q", got, want)
	}
	// The version comes from GitHub or a cache file on disk: escapes and line
	// breaks in it must not reach the terminal.
	if got := Banner(&Release{Version: "0.11.0\033[2J\nforged"}, cliui.TerminalStyles{}); strings.Contains(got, "\033") || strings.Contains(got, "forged") {
		t.Errorf("Banner(hostile version) = %q, want escapes and line breaks removed", got)
	}
	if got := Banner(&Release{Version: "0.9.1"}, styled); got != "" {
		t.Errorf("Banner(current release) = %q, want empty", got)
	}
	if got := Banner(nil, styled); got != "" {
		t.Errorf("Banner(nil) = %q, want empty", got)
	}
}
