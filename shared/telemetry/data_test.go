package telemetry_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/holistics/anfra/shared/telemetry"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// Each category is its variable's value, or its default, and says which.
func TestResolveReadsEachCategory(t *testing.T) {
	r, err := telemetry.Resolve(env(map[string]string{"ANFRA_TRACE_INPUTS": "raw", "ANFRA_TRACE_PAYLOAD_LIMIT": "100"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Data[telemetry.Inputs]; got.Value != telemetry.Raw || got.From != "ANFRA_TRACE_INPUTS" {
		t.Errorf("inputs = %+v", got)
	}
	if got := r.Data[telemetry.SQL]; got.Value != telemetry.Masked || got.From != "" {
		t.Errorf("sql = %+v, want masked by default", got)
	}
	if got := r.Data[telemetry.Outputs]; got.Value != telemetry.Off {
		t.Errorf("outputs = %+v, want off by default", got)
	}
	if s := r.Settings(); s.PayloadLimit() != 100 || s.Level(telemetry.Inputs) != telemetry.Raw {
		t.Errorf("settings: limit %d, inputs %s", s.PayloadLimit(), s.Level(telemetry.Inputs))
	}
}

// ANFRA_TRACE_DATA=off holds every category at its safest, whatever each says.
func TestDataOffHoldsEveryCategoryOff(t *testing.T) {
	r, err := telemetry.Resolve(env(map[string]string{"ANFRA_TRACE_DATA": "off", "ANFRA_TRACE_INPUTS": "raw", "ANFRA_TRACE_SQL": "raw"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range telemetry.Categories {
		if got := r.Data[d.Name]; got.Value != telemetry.Off || got.From != "ANFRA_TRACE_DATA" {
			t.Errorf("%s = %+v, want off, set by ANFRA_TRACE_DATA", d.Name, got)
		}
	}
	if got := telemetry.SidecarEnv(r.Settings()); !slices.Equal(got, []string{"H_OTEL_DATA_INPUTS=off", "H_OTEL_DATA_OUTPUTS=off", "H_OTEL_DATA_ERRORS=off"}) {
		t.Errorf("SidecarEnv = %v", got)
	}
}

// A value a setting doesn't take is refused, naming it: masked for inputs,
// on for ANFRA_TRACE_DATA, a payload limit that isn't a positive number.
func TestResolveRefusesWhatASettingDoesntTake(t *testing.T) {
	for name, value := range map[string]string{
		"ANFRA_TRACE_INPUTS": "masked", "ANFRA_TRACE_SQL": "1", "ANFRA_TRACE_DATA": "raw", "ANFRA_TRACE_PAYLOAD_LIMIT": "0",
	} {
		_, err := telemetry.Resolve(env(map[string]string{name: value}))
		e, ok := errors.AsType[*telemetry.InvalidSettingError](err)
		if !ok || e.Name != name || e.Value != value {
			t.Errorf("%s=%s: err = %v, want it refused", name, value, err)
		}
	}
}

// The zero Settings, a host's that sets none, is the defaults.
func TestZeroSettingsAreTheDefaults(t *testing.T) {
	var s telemetry.Settings
	for _, d := range telemetry.Categories {
		if s.Level(d.Name) != d.Default {
			t.Errorf("%s = %s, want %s", d.Name, s.Level(d.Name), d.Default)
		}
	}
	if s.PayloadLimit() != telemetry.DefaultPayloadLimit {
		t.Errorf("limit = %d", s.PayloadLimit())
	}
}
