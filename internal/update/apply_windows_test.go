//go:build windows

package update

import (
	"archive/zip"
	"bytes"
	"context"
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
