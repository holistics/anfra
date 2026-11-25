//go:build !windows

package robustio

import "os"

// Rename is os.Rename: Unix renames over a file others have open.
func Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
