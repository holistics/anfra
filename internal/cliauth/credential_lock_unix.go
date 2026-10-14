//go:build !windows

package cliauth

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func acquireCredentialLock(ctx context.Context, f *os.File) error {
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return errors.New("cliauth: cannot lock credentials")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
func releaseCredentialLock(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
