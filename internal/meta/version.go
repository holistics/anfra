// Package meta holds build-time metadata for the anfra binary.
package meta

import (
	"runtime/debug"
	"strings"
)

// Version is the anfra version this binary is.
//
// A release injects it at build time, as the `version` from manifest.yml (the
// single source of truth, which also names the `anfra-v<version>` tag):
//
//	go build -ldflags "-X github.com/holistics/anfra/internal/meta.Version=0.4.3"
//
// Any other build is one from source, and says which: the release it comes after
// (base), then, as semver build metadata, the commit it was built from and
// whether the tree had uncommitted changes, as Go records them in the binary:
// 0.4.3+f4fb358, or 0.4.3+f4fb358.dirty. Build metadata leaves the order alone,
// so such a build sorts with the release it comes after; and a release never
// carries any, so its presence alone tells a build from source (FromSource).
var Version = ""

// base is the release a build from source comes after, injected like Version by
// make build and make dev:
//
//	go build -ldflags "-X github.com/holistics/anfra/internal/meta.base=0.4.3"
//
// Without it, 0.0.0.
var base = ""

func init() {
	if Version == "" {
		var settings []debug.BuildSetting
		if info, ok := debug.ReadBuildInfo(); ok {
			settings = info.Settings
		}
		Version = sourceVersion(base, settings)
	}
}

// FromSource reports whether this binary was built from source rather than
// released: its version carries build metadata.
func FromSource() bool { return strings.Contains(Version, "+") }

// sourceVersion is the version of a build from source: base, and the commit and
// state of the tree it was built from, or "source" when Go recorded no commit (a
// build outside a git checkout, or with -buildvcs=false).
func sourceVersion(base string, settings []debug.BuildSetting) string {
	if base == "" {
		base = "0.0.0"
	}
	var revision string
	var dirty bool
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if revision == "" {
		return base + "+source"
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if dirty {
		return base + "+" + revision + ".dirty"
	}
	return base + "+" + revision
}
