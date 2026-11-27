//go:build !windows

package update

import (
	"compress/gzip"
	"io"
	"os"
)

// unpack is the anfra binary in the release asset at path: gzipped.
func unpack(path string) (io.ReadCloser, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the download Apply made
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return readCloser{gz, func() error { _ = gz.Close(); return f.Close() }}, nil
}
