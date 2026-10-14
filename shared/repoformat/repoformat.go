// Package repoformat is what an anfra repo is, as anything handling one outside
// the engine needs it: which files make up a repo (Publishable), and how a file's
// content is named (BlobID). anfra core owns it, as it owns the repo format;
// anfra-cloud imports it, so a server and the CLI agree on both.
package repoformat

import (
	"crypto/sha1" //nolint:gosec // G505: git names blobs by SHA-1; BlobID computes such names, nothing secret
	"encoding/hex"
	"strconv"
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
// The CLI selects with it, and a server refuses what it does not allow, so
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

// BlobID is content's git blob id: SHA-1 over "blob <size>\0" and the content,
// as git names it with no filters applied. It is what a publish names a file's
// content by, and what a server, storing it in git, answers for it.
func BlobID(content []byte) string {
	h := sha1.New() //nolint:gosec // G401: git's object naming
	h.Write([]byte("blob " + strconv.Itoa(len(content)) + "\x00"))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}
