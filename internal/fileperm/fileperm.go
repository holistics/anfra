// Package fileperm makes files only their owner can read, on every OS: mode
// 600 on Unix, an access list of the owner alone on Windows, which has no file
// modes.
package fileperm

import "os"

// WriteFile writes data to the file name, made or emptied, that only this user
// can read; set before data is written.
func WriteFile(name string, data []byte) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // G304: the caller's own file
	if err != nil {
		return err
	}
	if err := OwnerOnly(f); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
