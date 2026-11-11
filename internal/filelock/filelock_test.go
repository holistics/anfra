package filelock_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/filelock"
)

func open(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// A lock held by one open of a file refuses another, and Lock waits for it
// until its context ends, or takes it once it's released.
func TestLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	a, b := open(t, path), open(t, path)
	if err := filelock.TryLock(a); err != nil {
		t.Fatal(err)
	}
	if err := filelock.TryLock(b); !errors.Is(err, filelock.ErrHeld) {
		t.Fatalf("second TryLock: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if err := filelock.Lock(ctx, b); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Lock while held: %v", err)
	}
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(30 * time.Millisecond)
		filelock.Unlock(a)
	}()
	err := filelock.Lock(context.Background(), b)
	// Done with a before the test's cleanup closes it: the lock orders the
	// two, but the race detector can't see a lock between processes.
	<-released
	if err != nil {
		t.Fatalf("Lock after release: %v", err)
	}
	filelock.Unlock(b)
}
