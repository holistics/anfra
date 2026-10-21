// Package meta holds build-time metadata for the anfra binary.
package meta

import (
	"runtime/debug"
	"strings"
)

// Version is the anfra version this binary is.
//
// A release injects it at build time, as the `version` from manifest.yml (the
// single source of truth, which also names the `v<version>` tag):
//
//	go build -ldflags "-X github.com/holistics/anfra/internal/meta.Version=0.4.3"
//
// Built into another program, which imports anfra as a module (anfra-cloud's
// server, say), it is the version of anfra that program pins, as Go records it in
// the binary: a release, 0.4.3, or an unreleased commit's pseudo-version,
// 0.4.4-0.20260722101500-f4fb35812ab3. Nothing is injected there: the program's
// build sets its own version, not anfra's.
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

// modulePath is anfra's module, as another program's build records it.
const modulePath = "github.com/holistics/anfra"

func init() {
	if Version == "" {
		info, _ := debug.ReadBuildInfo() // nil without one
		Version = buildVersion(info, base)
	}
}

// buildVersion is the version of a binary with none injected: the version of the
// anfra module another program was built with, or else a build from source.
func buildVersion(info *debug.BuildInfo, base string) string {
	if info == nil {
		return sourceVersion(base, nil)
	}
	if info.Main.Path == modulePath {
		return sourceVersion(base, info.Settings)
	}
	for _, dep := range info.Deps {
		if dep.Path != modulePath {
			continue
		}
		if dep.Replace != nil {
			dep = dep.Replace
		}
		// A version, unless anfra was a local directory (a replace directive, or a
		// go.work): then it is a build from source, of a commit Go did not record.
		if v := dep.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
		break
	}
	// The settings are the program's, whose commit is not anfra's.
	return sourceVersion(base, nil)
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
