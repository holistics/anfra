//go:build !windows

package cliauth

import (
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/clierr"
)

// makePrivate makes the new authentication file readable by this user only.
func makePrivate(f *os.File) error { return f.Chmod(0o600) }

// checkPrivate refuses the authentication file unless only this user can read it.
func checkPrivate(path string, info os.FileInfo) error {
	if info.Mode().Perm() != 0o600 {
		return clierr.Precondition("can't use the authentication file (%s), because its permissions aren't 600 (read and write for you only). To fix them, run `chmod 600 %s`", path, path)
	}
	return nil
}

func syncCredentialDirectory(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
