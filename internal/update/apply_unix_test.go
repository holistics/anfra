//go:build !windows

package update

import (
	"bytes"
	"compress/gzip"
	"testing"
)

// packAsset is content as a release publishes it for Linux and macOS: gzipped.
func packAsset(t *testing.T, content []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	if _, err := zw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
