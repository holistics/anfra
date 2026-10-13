package remoteapi

import (
	"strings"
)

// AppsDir is where a repo's Data Apps are: published whole.
//
// TODO: configurable, in the repo's config, rather than fixed.
const AppsDir = "apps"

// Publishable reports whether a file is part of an anfra repo, so a publish
// carries it: its AML (*.aml), and its Data Apps (everything under AppsDir).
// path is slash-separated and relative to the repo root.
//
// A publish carries the repo, not the folder it is in: what the server uses,
// chosen by the repo's format, the same on every machine, with or without git.
// So a .env, build output, notes or scripts beside the repo are never sent, and
// nothing depends on .gitignore, which says what belongs in version control, a
// different question (PUBLISH.md §What gets published). Within the repo, never:
//
//   - a path with a part starting with ".": git's own files, the repo's config
//     (.anfra/, whose data_sources.yml holds credentials), editors' and tools'
//     state;
//   - a path through node_modules: a Data App's installed dependencies.
//
// The client selects with it, and a server refuses what it does not allow, so
// neither a modified client nor a mistake in one can publish anything else.
func Publishable(path string) bool {
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if p == "" || strings.HasPrefix(p, ".") || p == "node_modules" {
			return false
		}
	}
	return parts[0] == AppsDir && len(parts) > 1 || strings.HasSuffix(strings.ToLower(path), ".aml")
}
