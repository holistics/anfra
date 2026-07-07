package sidecar

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// resolveAnfraNodeBinary returns a path to an executable anfra-node. Release
// builds embed it (extracted to a content-addressed cache dir); dev builds take
// it from ANFRA_NODE_BIN.
func resolveAnfraNodeBinary() (string, error) {
	if data, ok := embeddedAnfraNode(); ok {
		return extractRuntime("anfra-node", data)
	}
	if env := os.Getenv("ANFRA_NODE_BIN"); env != "" {
		return env, nil
	}
	return "", errors.New("no embedded anfra-node; set ANFRA_NODE_BIN")
}

// resolveCanalQueryBinary returns a path to an executable canal-query. Release
// builds embed it; dev builds take it from ANFRA_CANAL_QUERY_BIN.
func resolveCanalQueryBinary() (string, error) {
	if data, ok := embeddedCanalQuery(); ok {
		return extractRuntime("canal-query", data)
	}
	if env := os.Getenv("ANFRA_CANAL_QUERY_BIN"); env != "" {
		return env, nil
	}
	return "", errors.New("no embedded canal-query; set ANFRA_CANAL_QUERY_BIN")
}

// extractRuntime writes an embedded sidecar binary to the per-user cache dir
// keyed by content hash (<name>-<hash>), writing only when missing so concurrent
// runs converge on the same file.
func extractRuntime(name string, data []byte) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "anfra", "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	fname := name + "-" + hex.EncodeToString(sum[:8])
	path := filepath.Join(dir, fname)

	if info, err := os.Stat(path); err != nil || info.Size() != int64(len(data)) {
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o755); err != nil { //nolint:gosec // G306: an extracted sidecar binary must be executable
			return "", err
		}
		if err := os.Rename(tmp, path); err != nil {
			return "", err
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
