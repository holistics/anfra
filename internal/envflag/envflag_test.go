package envflag

import (
	"errors"
	"testing"

	"github.com/holistics/anfra/shared/telemetry"
)

func TestOn(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "1": true, "true": false} {
		t.Setenv(LogStderr, v)
		if got := On(LogStderr); got != want {
			t.Errorf("%s=%q: On() = %v, want %v", LogStderr, v, got, want)
		}
	}
}

func TestOnOr(t *testing.T) {
	for v, want := range map[string]bool{"": true, "0": false, "1": true} {
		t.Setenv(LogStderr, v)
		if got := OnOr(LogStderr, true); got != want {
			t.Errorf("%s=%q: OnOr(true) = %v, want %v", LogStderr, v, got, want)
		}
	}
	t.Setenv(LogStderr, "")
	if OnOr(LogStderr, false) {
		t.Errorf("%s unset: OnOr(false) = true", LogStderr)
	}
}

func TestValidate(t *testing.T) {
	for _, name := range all {
		t.Setenv(name, "")
	}
	if err := Validate(); err != nil {
		t.Fatalf("all unset: %v", err)
	}

	t.Setenv(AutoUpdate, "1")
	t.Setenv(LogStderr, "0")
	if err := Validate(); err != nil {
		t.Fatalf("1 and 0: %v", err)
	}

	for _, bad := range []string{"true", "yes", "on", " 1", "2"} {
		t.Setenv(HideErrorCauses, bad)
		err := Validate()
		if want := HideErrorCauses + ` must be 1 or 0, not "` + bad + `"`; err == nil || err.Error() != want {
			t.Errorf("%s=%q: %v, want %q", HideErrorCauses, bad, err, want)
		}
	}
}

// The telemetry data settings are checked too, and the switch they replaced
// is refused, naming them.
func TestValidateChecksTheTelemetrySettings(t *testing.T) {
	for _, name := range all {
		t.Setenv(name, "")
	}
	t.Setenv("ANFRA_TRACE_SQL", "raw")
	t.Setenv(TracePayloads, "")
	if err := Validate(); err != nil {
		t.Fatalf("ANFRA_TRACE_SQL=raw: %v", err)
	}
	t.Setenv("ANFRA_TRACE_INPUTS", "masked")
	if _, ok := errors.AsType[*telemetry.InvalidSettingError](Validate()); !ok {
		t.Errorf("ANFRA_TRACE_INPUTS=masked: %v, want it refused", Validate())
	}
	t.Setenv("ANFRA_TRACE_INPUTS", "")
	t.Setenv(TracePayloads, "1")
	if _, ok := errors.AsType[*RetiredError](Validate()); !ok {
		t.Errorf("%s=1: %v, want it refused as retired", TracePayloads, Validate())
	}
}
