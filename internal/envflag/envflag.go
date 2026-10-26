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

// The switches. Each is off by default, but LogStderr, which is on for
// `anfra serve` (OnOr).
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

// OnOr reports whether the switch is on: 1, or def when it is unset or empty.
// For a switch whose default depends on the command: 0 turns it off where it
// defaults to on.
func OnOr(name string, def bool) bool {
	if os.Getenv(name) == "" {
		return def
	}
	return On(name)
}

func parse(name string) (bool, error) {
	switch v := os.Getenv(name); v {
	case "", "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, &InvalidError{Name: name, Value: v}
	}
}

// InvalidError is a switch set to something other than 1 or 0.
type InvalidError struct {
	Name  string // the variable, ANFRA_LOG_STDERR
	Value string // what it is set to
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf("%s must be 1 or 0, not %q", e.Name, e.Value)
}
