// Package meta holds build-time metadata for the anfra binary.
package meta

// Version is the anfra release version. It defaults to "dev" for local builds and
// is overridden at release-build time via ldflags:
//
//	go build -ldflags "-X github.com/holistics/anfra/internal/meta.Version=0.1.0"
//
// The value is the `version` from manifest.yml (the single source of truth). The
// same value drives the `anfra-v<version>` git tag, so manifest, tag, and the
// compiled-in version always agree. Injected by the assembler workflow
// (.github/workflows/build_release.yml).
var Version = "dev"
