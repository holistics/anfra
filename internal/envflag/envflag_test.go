package envflag

import "testing"

func TestOn(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "1": true, "true": false} {
		t.Setenv(LogStderr, v)
		if got := On(LogStderr); got != want {
			t.Errorf("%s=%q: On() = %v, want %v", LogStderr, v, got, want)
		}
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
