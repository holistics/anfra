package meta

import (
	"runtime/debug"
	"testing"
)

func TestSourceVersion(t *testing.T) {
	commit := debug.BuildSetting{Key: "vcs.revision", Value: "f4fb35812ab34c0d9e1f2a3b4c5d6e7f8a9b0c1d"}
	clean := debug.BuildSetting{Key: "vcs.modified", Value: "false"}
	dirty := debug.BuildSetting{Key: "vcs.modified", Value: "true"}
	cases := []struct {
		base     string
		settings []debug.BuildSetting
		want     string
	}{
		{"0.4.3", []debug.BuildSetting{commit, clean}, "0.4.3+f4fb358"},
		{"0.4.3", []debug.BuildSetting{commit, dirty}, "0.4.3+f4fb358.dirty"},
		{"", []debug.BuildSetting{commit, clean}, "0.0.0+f4fb358"},
		{"0.4.3", nil, "0.4.3+source"},
	}
	for _, c := range cases {
		if got := sourceVersion(c.base, c.settings); got != c.want {
			t.Errorf("sourceVersion(%q, %v) = %q, want %q", c.base, c.settings, got, c.want)
		}
	}
}

func TestFromSource(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	for v, want := range map[string]bool{"0.4.3": false, "0.4.3+f4fb358": true, "0.0.0+source": true} {
		Version = v
		if FromSource() != want {
			t.Errorf("FromSource() for %q: %v, want %v", v, !want, want)
		}
	}
}
