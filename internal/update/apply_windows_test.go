//go:build windows

package update

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packAsset is content as a release publishes it for Windows: anfra.exe, zipped.
func packAsset(t *testing.T, content []byte) []byte {
	t.Helper()
	return zipOf(t, "anfra.exe", content)
}

func zipOf(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A .zip without anfra.exe at its top refuses, saying so; nothing is replaced.
func TestApplyRefusesAZipWithoutAnfra(t *testing.T) {
	asset := zipOf(t, "bin/anfra.exe", []byte("new"))
	r := fakeRelease(t, asset, sumOf(asset)+"  "+assetFile(t)+"\n")
	path, _ := target(t)
	if err := applyTo(context.Background(), r, nil, path); err == nil || !strings.Contains(err.Error(), "anfra.exe") {
		t.Errorf("got %v, want a refusal naming anfra.exe", err)
	}
}

// Leftovers of earlier updates (.anfra.exe.old*) go before an update; one still
// in use (an older anfra serve) stays, and the update goes ahead anyway.
func TestApplyAroundLeftoverOldBinaries(t *testing.T) {
	asset := packAsset(t, []byte("new"))
	r := fakeRelease(t, asset, sumOf(asset)+"  "+assetFile(t)+"\n")
	dir := t.TempDir()
	path := filepath.Join(dir, "anfra.exe")
	for name, content := range map[string]string{"anfra.exe": "old", ".anfra.exe.old": "in use", ".anfra.exe.old-1": "stale"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755); err != nil { //nolint:gosec // G306: stand-ins for executables
			t.Fatal(err)
		}
	}
	held, err := os.Open(filepath.Join(dir, ".anfra.exe.old")) // as a running anfra holds its binary
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := applyTo(context.Background(), r, nil, path); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" { //nolint:gosec // G304: the test's own file
		t.Errorf("anfra.exe holds %q, want the new anfra", b)
	}
	if _, err := os.Stat(filepath.Join(dir, ".anfra.exe.old-1")); !os.IsNotExist(err) {
		t.Error("a stale leftover is still there")
	}
}

// On Windows, a refusal points to install.ps1.
func TestApplyAdviceOnWindows(t *testing.T) {
	asset := packAsset(t, []byte("new"))
	r := fakeRelease(t, asset, "")
	path, _ := target(t)
	if err := applyTo(context.Background(), r, nil, path); err == nil || !strings.Contains(err.Error(), "install.ps1") {
		t.Errorf("got %v, want advice to use install.ps1", err)
	}
}
