//go:build windows

package appserve

import (
	"testing"
	"time"
	_ "time/tzdata" // LoadLocation, without Go's zone files on the machine
)

// Data Apps get Windows' own zone, as an IANA name: the same offset as the
// machine's local time.
func TestLocalTimezoneIsWindows(t *testing.T) {
	t.Setenv("TZ", "")
	tz := localTimezone()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatalf("localTimezone() = %q: %v", tz, err)
	}
	now := time.Now()
	_, got := now.In(loc).Zone()
	_, want := now.Zone()
	if got != want {
		t.Errorf("localTimezone() = %s, at UTC%+d s; Windows is at UTC%+d s", tz, got, want)
	}
	t.Logf("localTimezone() = %s", tz)
}

// $TZ still wins.
func TestLocalTimezoneTZWins(t *testing.T) {
	t.Setenv("TZ", "Europe/Paris")
	if got := localTimezone(); got != "Europe/Paris" {
		t.Errorf("localTimezone() = %s, want TZ's Europe/Paris", got)
	}
}
