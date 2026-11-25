//go:build windows

package sidecar

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/holistics/anfra/internal/home"
)

// extractRuntime writes an embedded sidecar binary to anfra's folder, as
// <home>\sidecars\<name>-<hash>\<name>.exe, and libs beside it: Windows runs
// only a file named .exe, and can't replace or delete one while it runs, so
// each version gets a folder of its own, and an anfra still running an older
// one keeps it. The hash covers libs too, so a new library is a new version.
func extractRuntime(name string, data []byte, libs []Library) (string, error) {
	home.RemoveLegacy()
	h := sha256.New()
	h.Write(data)
	for _, lib := range libs {
		h.Write([]byte(lib.Name))
		h.Write(lib.Data)
	}
	version := name + "-" + hex.EncodeToString(h.Sum(nil)[:8])
	sidecars := filepath.Join(home.Dir(), "sidecars")
	dir := filepath.Join(sidecars, version)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G703: anfra's own folder
		return "", unpackErr(name, err)
	}

	for _, lib := range libs {
		if err := writeOnce(filepath.Join(dir, lib.Name), lib.Data); err != nil {
			return "", unpackErr(name, err)
		}
	}
	path := filepath.Join(dir, name+".exe")
	if err := writeOnce(path, data); err != nil {
		return "", unpackErr(name, err)
	}

	pruneRuntimes(sidecars, name, version)
	return path, nil
}

// writeOnce writes data to path unless it is there, through a temporary file
// of its own, so several anfras unpacking at once don't write into each
// other's. When the rename fails because another anfra's copy is in place, and
// running, theirs is kept.
func writeOnce(path string, data []byte) error {
	if hasSize(path, len(data)) {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path) //nolint:gosec // G703: anfra's own folder
	}
	if err != nil {
		_ = os.Remove(tmp.Name()) //nolint:gosec // G703: anfra's own folder
		if hasSize(path, len(data)) {
			return nil
		}
	}
	return err
}

func hasSize(path string, size int) bool {
	info, err := os.Stat(path) //nolint:gosec // G703: anfra's own folder
	return err == nil && info.Size() == int64(size)
}

// pruneRuntimes removes the folders of name's other versions (best-effort). A
// folder whose binary Windows won't delete, because an anfra still runs it, is
// left whole, for a later prune.
func pruneRuntimes(sidecars, name, keep string) {
	entries, err := os.ReadDir(sidecars)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.Name() == keep || !e.IsDir() || !strings.HasPrefix(e.Name(), name+"-") {
			continue
		}
		dir := filepath.Join(sidecars, e.Name())
		if err := os.Remove(filepath.Join(dir, name+".exe")); err != nil && !os.IsNotExist(err) {
			continue
		}
		_ = os.RemoveAll(dir)
	}
}
