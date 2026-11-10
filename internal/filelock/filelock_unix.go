//go:build !windows

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// TryLock takes f's lock without waiting: ErrHeld when it's held.
func TryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrHeld
	}
	return err
}

// Unlock releases f's lock.
func Unlock(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
