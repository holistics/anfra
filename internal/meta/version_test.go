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

func TestBuildVersion(t *testing.T) {
	commit := debug.BuildSetting{Key: "vcs.revision", Value: "f4fb35812ab34c0d9e1f2a3b4c5d6e7f8a9b0c1d"}
	embedded := func(dep debug.Module) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main:     debug.Module{Path: "github.com/holistics/anfra-cloud", Version: "(devel)"},
			Deps:     []*debug.Module{{Path: "golang.org/x/text", Version: "v0.20.0"}, &dep},
			Settings: []debug.BuildSetting{commit}, // the program's commit, not anfra's
		}
	}
	cases := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"no build info", nil, "0.4.3+source"},
		{"anfra itself", &debug.BuildInfo{Main: debug.Module{Path: modulePath}, Settings: []debug.BuildSetting{commit}}, "0.4.3+f4fb358"},
		{"embedded, a release", embedded(debug.Module{Path: modulePath, Version: "v0.13.0"}), "0.13.0"},
		{"embedded, a commit", embedded(debug.Module{Path: modulePath, Version: "v0.13.1-0.20261020101500-2998976df0c0"}), "0.13.1-0.20261020101500-2998976df0c0"},
		{"embedded, replaced by a version", embedded(debug.Module{Path: modulePath, Version: "v0.13.0", Replace: &debug.Module{Path: "github.com/fork/anfra", Version: "v0.13.2"}}), "0.13.2"},
		{"embedded, replaced by a directory", embedded(debug.Module{Path: modulePath, Version: "v0.13.0", Replace: &debug.Module{Path: "../anfra"}}), "0.4.3+source"},
		{"embedded, in a workspace", embedded(debug.Module{Path: modulePath, Version: "(devel)"}), "0.4.3+source"},
		{"not anfra at all", embedded(debug.Module{Path: "github.com/other/thing", Version: "v1.0.0"}), "0.4.3+source"},
	}
	for _, c := range cases {
		if got := buildVersion(c.info, "0.4.3"); got != c.want {
			t.Errorf("%s: buildVersion = %q, want %q", c.name, got, c.want)
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
