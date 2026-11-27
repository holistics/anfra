//go:build windows

package update

import (
	"archive/zip"
	"errors"
	"io"
)

// unpack is the anfra binary in the release asset at path: anfra.exe, at the
// top of a .zip.
func unpack(path string) (io.ReadCloser, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	for _, f := range z.File {
		if f.Name == "anfra.exe" {
			exe, err := f.Open()
			if err != nil {
				_ = z.Close()
				return nil, err
			}
			return readCloser{exe, func() error { _ = exe.Close(); return z.Close() }}, nil
		}
	}
	_ = z.Close()
	return nil, errors.New("the release's .zip has no anfra.exe at its top")
}
