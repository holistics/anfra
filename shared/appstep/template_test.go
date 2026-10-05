package appstep

import "testing"

func TestCheckTemplate(t *testing.T) {
	for _, ok := range []string{"import dashboard {name}", "retry attempt {n} of {max_n}", "sign in"} {
		if err := checkTemplate(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "import {name", "import {Name}", "import }"} {
		if checkTemplate(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
