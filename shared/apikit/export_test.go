package apikit

import "time"

// SetPingEvery shortens the ping interval for a test, and restores it after.
func SetPingEvery(d time.Duration) (restore func()) {
	old := pingEvery
	pingEvery = d
	return func() { pingEvery = old }
}
