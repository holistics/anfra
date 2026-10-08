// Package envflag reads anfra's on/off environment variables. Each is 1 (on) or
// 0 (off); unset or empty means its default, and any other value is refused,
// at startup, so a typo surfaces at once rather than silently meaning the
// opposite.
//
// Only anfra's own switches follow this rule. Standard variables keep their own
// conventions: CI (set by CI systems, to anything), OTEL_SDK_DISABLED
// (OpenTelemetry's, "true").
package envflag

import (
	"fmt"
	"os"
)

// The switches, each off by default.
const (
	LogStderr        = "ANFRA_LOG_STDERR"
	SidecarStdout    = "ANFRA_SIDECAR_STDOUT"
	NoUpdateNotifier = "ANFRA_NO_UPDATE_NOTIFIER"
	AutoUpdate       = "ANFRA_AUTO_UPDATE"
	HideErrorCauses  = "ANFRA_HIDE_ERROR_CAUSES"
)

var all = []string{LogStderr, SidecarStdout, NoUpdateNotifier, AutoUpdate, HideErrorCauses}

// Validate refuses a switch set to anything but 1 or 0, naming it and the value
// it has. Call it once, before anything reads a switch.
func Validate() error {
	for _, name := range all {
		if _, err := parse(name); err != nil {
			return err
		}
	}
	return nil
}

// On reports whether the switch is on: 1. Unset, empty, 0, or (after Validate
// has refused it) any other value is off.
func On(name string) bool {
	on, err := parse(name)
	return err == nil && on
}

func parse(name string) (bool, error) {
	switch v := os.Getenv(name); v {
	case "", "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("%s must be 1 or 0, not %q", name, v)
	}
}
