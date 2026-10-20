package routine

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// Backoff is a retry delay: exponential from its min to its max, with jitter.
// A Loop restarts by one; a routine retrying its own items, a worker's failed
// write say, uses one of its own. Not safe for concurrent use.
type Backoff struct {
	min, max time.Duration
	attempt  int
}

// NewBackoff returns a backoff from min to max.
func NewBackoff(min, max time.Duration) *Backoff {
	checkBackoff(min, max)
	return &Backoff{min: min, max: max}
}

func checkBackoff(min, max time.Duration) {
	if min <= 0 || max < min {
		panic(fmt.Sprintf("routine: backoff from %v to %v: want 0 < min <= max", min, max))
	}
}

// Next is the next delay, and counts an attempt. Each is between half and all
// of min doubled once per attempt before it, capped at max.
func (b *Backoff) Next() time.Duration {
	d := b.max
	if b.attempt < 62 {
		if grown := b.min << b.attempt; grown > 0 && grown < b.max {
			d = grown
		}
	}
	b.attempt++
	return d/2 + time.Duration(rand.Int64N(int64(d/2)+1)) //nolint:gosec // jitter, not security
}

// Wait sleeps the next delay, or until ctx ends, returning its error.
func (b *Backoff) Wait(ctx context.Context) error {
	t := time.NewTimer(b.Next())
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Reset starts the delays over from min, after a success.
func (b *Backoff) Reset() { b.attempt = 0 }
