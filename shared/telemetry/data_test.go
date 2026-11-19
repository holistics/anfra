package telemetry_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/holistics/anfra/shared/telemetry"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// Data is on unless its switch is 0, and says what set it.
func TestResolveReadsTheSwitch(t *testing.T) {
	r, err := telemetry.Resolve(env(map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": "1", "ANFRA_EXPERIMENTAL_TRACE_PAYLOAD_LIMIT": "100"}))
	if err != nil {
		t.Fatal(err)
	}
	if r.Data.Value != true || r.Data.From != "ANFRA_EXPERIMENTAL_TRACE_DATA" {
		t.Errorf("data = %+v", r.Data)
	}
	if s := r.Settings(); s.PayloadLimit() != 100 || !s.RecordsData() {
		t.Errorf("settings: limit %d, data %v", s.PayloadLimit(), s.RecordsData())
	}
	if got := telemetry.SidecarEnv(r.Settings()); !slices.Equal(got, []string{"H_OTEL_DATA=1"}) {
		t.Errorf("SidecarEnv = %v", got)
	}
	r, _ = telemetry.Resolve(env(nil))
	if !r.Data.Value || r.Data.From != "" || !slices.Equal(telemetry.SidecarEnv(r.Settings()), []string{"H_OTEL_DATA=1"}) {
		t.Errorf("unset: data = %+v, want on by default", r.Data)
	}
	r, _ = telemetry.Resolve(env(map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": "0"}))
	if r.Data.Value || !slices.Equal(telemetry.SidecarEnv(r.Settings()), []string{"H_OTEL_DATA=0"}) {
		t.Errorf("0: data = %+v, want off", r.Data)
	}
}

// A value a setting doesn't take is refused, naming it: anything but 1 or 0
// for the switch, a payload limit that isn't a positive number.
func TestResolveRefusesWhatASettingDoesntTake(t *testing.T) {
	for name, value := range map[string]string{"ANFRA_EXPERIMENTAL_TRACE_DATA": "raw", "ANFRA_EXPERIMENTAL_TRACE_PAYLOAD_LIMIT": "0"} {
		_, err := telemetry.Resolve(env(map[string]string{name: value}))
		e, ok := errors.AsType[*telemetry.InvalidSettingError](err)
		if !ok || e.Name != name || e.Value != value {
			t.Errorf("%s=%s: err = %v, want it refused", name, value, err)
		}
	}
}

// The zero Settings, a host's that sets none, records no data.
func TestZeroSettingsRecordNoData(t *testing.T) {
	var s telemetry.Settings
	if s.RecordsData() || s.PayloadLimit() != telemetry.DefaultPayloadLimit {
		t.Errorf("data %v, limit %d", s.RecordsData(), s.PayloadLimit())
	}
}
