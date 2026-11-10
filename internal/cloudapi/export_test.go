package cloudapi

import (
	"context"
	"time"
)

// NoWait makes retries happen at once, recording each wait asked for, until
// the test ends.
func NoWait(cleanup func(func())) *[]time.Duration {
	var waits []time.Duration
	old := sleep
	sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	cleanup(func() { sleep = old })
	return &waits
}
