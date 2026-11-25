//go:build !windows

package sidecar

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/holistics/anfra/internal/home"
)

// extractRuntime writes an embedded sidecar binary to anfra's folder, keyed by
// content hash (<home>/sidecars/<name>-<hash>), writing only when missing so
// concurrent runs converge on the same file.
func extractRuntime(name string, data []byte) (string, error) {
	home.RemoveLegacy()
	dir := filepath.Join(home.Dir(), "sidecars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", unpackErr(name, err)
	}

	sum := sha256.Sum256(data)
	fname := name + "-" + hex.EncodeToString(sum[:8])
	path := filepath.Join(dir, fname)

	if info, err := os.Stat(path); err != nil || info.Size() != int64(len(data)) {
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o755); err != nil { //nolint:gosec // G306: an extracted sidecar binary must be executable
			return "", unpackErr(name, err)
		}
		if err := os.Rename(tmp, path); err != nil {
			return "", unpackErr(name, err)
		}
	}

	// Prune stale versions of *this* binary; leave other sidecars' binaries alone.
	pruneRuntimes(dir, name, fname)
	return path, nil
}

// pruneRuntimes removes "<prefix>-*" entries in dir except keep (best-effort).
func pruneRuntimes(dir, prefix, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == keep || !strings.HasPrefix(e.Name(), prefix+"-") {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}
