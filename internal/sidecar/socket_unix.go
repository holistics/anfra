//go:build !windows

package sidecar

import "os"

// privateTempDir makes a new folder in the temp dir that only this user can open.
func privateTempDir() (string, error) { return os.MkdirTemp("", "anfra-") }
