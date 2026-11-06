package cliauth

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/holistics/anfra/internal/clierr"

	"golang.org/x/sys/windows"
)

func acquireCredentialLock(ctx context.Context, f *os.File) error {
	for {
		err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
		if err == nil {
			return nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return clierr.Precondition("can't lock %s: %v. anfra's folder may be on a network drive that doesn't support file locks. If so, set ANFRA_HOME to a folder on a local disk", f.Name(), err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
func releaseCredentialLock(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{})
}
