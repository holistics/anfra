//go:build !windows

package fileperm

import "os"

// OwnerOnly makes f readable and writable by this user only: mode 600.
func OwnerOnly(f *os.File) error { return f.Chmod(0o600) }

// IsOwnerOnly reports whether only this user can read the file at path: mode 600.
func IsOwnerOnly(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm() == 0o600, nil
}
