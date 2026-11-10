// Package filelock takes an open file's exclusive lock, across processes. The
// system releases a lock when its process ends, so a crash never leaves one
// held.
package filelock

import (
	"context"
	"errors"
	"os"
	"time"
)

// ErrHeld is a lock another process, or another open of the file, holds.
var ErrHeld = errors.New("the lock is held")

// pollInterval is how often Lock tries again while the lock is held.
const pollInterval = 25 * time.Millisecond

// Lock takes f's lock, waiting while it's held, until ctx is done. An error
// other than the lock being held, such as a file system without locks,
// returns at once.
func Lock(ctx context.Context, f *os.File) error {
	for {
		err := TryLock(f)
		if !errors.Is(err, ErrHeld) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
