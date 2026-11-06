//go:build !windows

package cliauth

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/holistics/anfra/internal/clierr"

	"golang.org/x/sys/unix"
)

func acquireCredentialLock(ctx context.Context, f *os.File) error {
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return clierr.Precondition("can't lock %s: %v. anfra's folder may be on a network drive that doesn't support file locks. If so, set ANFRA_HOME to a folder on a local disk", f.Name(), err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
func releaseCredentialLock(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
