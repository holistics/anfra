package appstep_test

import (
	"reflect"
	"testing"

	"github.com/holistics/anfra/shared/appstep"
)

var ns = appstep.DefineNamespace("test")

var (
	importDashboard   = appstep.Define(ns, "import_dashboard", appstep.Public("import dashboard {name}"))
	retry             = appstep.Define(ns, "retry", appstep.Public("retry attempt {n}"))
	connectDatasource = appstep.Define(ns, "connect_datasource", appstep.Public("connect to data source {name}"))
	invite            = appstep.Define(ns, "invite", appstep.Public("invite {email}"), appstep.Sensitive("email"))
	parseFile         = appstep.Define(ns, "parse_file")
)

// The log form names the step and every parameter, redacting the sensitive.
func TestStepLogForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		step appstep.Step
		want string
	}{
		{"no parameters", appstep.NewStep(parseFile), "test.parse_file"},
		{"parameters, in order", appstep.NewStep(importDashboard, "name", "Sales", "by", 7),
			"test.import_dashboard(name=Sales, by=7)"},
		{"a parameter the step declares sensitive is redacted, whatever the caller passes", appstep.NewStep(invite, "email", "carol@example.com"),
			"test.invite(email=[redacted])"},
		{"malformed pairs are kept, visibly, under a bad key", appstep.NewStep(parseFile, "name", "a", 42, "dangling"),
			"test.parse_file(name=a, !BADKEY=42, !BADKEY=dangling)"},
	} {
		if got := tc.step.String(); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A public step describes itself to the user from its template; a private one,
// or one missing a parameter its template names, does not.
func TestDescribe(t *testing.T) {
	for _, tc := range []struct {
		name string
		step appstep.Step
		want *appstep.Description
	}{
		{"strings are quoted", appstep.NewStep(importDashboard, "name", "Sales"),
			&appstep.Description{Name: "test.import_dashboard", Params: map[string]any{"name": "Sales"}, Text: "import dashboard 'Sales'"}},
		{"other values render plainly", appstep.NewStep(retry, "n", 3),
			&appstep.Description{Name: "test.retry", Params: map[string]any{"n": 3}, Text: "retry attempt 3"}},
		{"only the parameters the template names are kept",
			appstep.NewStep(connectDatasource, "name", "warehouse", "host", "db.internal"),
			&appstep.Description{Name: "test.connect_datasource", Params: map[string]any{"name": "warehouse"}, Text: "connect to data source 'warehouse'"}},
		{"a sensitive value is shown: it came from the user",
			appstep.NewStep(invite, "email", "carol@example.com"),
			&appstep.Description{Name: "test.invite", Params: map[string]any{"email": "carol@example.com"}, Text: "invite 'carol@example.com'"}},
		{"a missing parameter is never shown half-filled", appstep.NewStep(importDashboard), nil},
		{"a private step has no description", appstep.NewStep(parseFile, "name", "a"), nil},
	} {
		got, ok := tc.step.Describe()
		switch {
		case tc.want == nil && ok:
			t.Errorf("%s: described as %+v, want none", tc.name, got)
		case tc.want != nil && (!ok || !reflect.DeepEqual(got, *tc.want)):
			t.Errorf("%s: %+v (ok=%v), want %+v", tc.name, got, ok, *tc.want)
		}
	}
}

func TestDefiningStepsRefusesMistakes(t *testing.T) {
	for name, define := range map[string]func(){
		"a duplicate name":       func() { appstep.Define(ns, "parse_file") },
		"a malformed template":   func() { appstep.Define(ns, "bad_template", appstep.Public("import {name")) },
		"an empty name":          func() { appstep.Define(ns, "") },
		"an empty sensitive key": func() { appstep.Define(ns, "empty_sensitive", appstep.Sensitive("")) },
		"no namespace":           func() { appstep.Define(appstep.Namespace{}, "orphan") },
		"a repeated sensitive key": func() {
			appstep.Define(ns, "repeated_sensitive", appstep.Sensitive("email"), appstep.Sensitive("email"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("defining a step with %s did not panic", name)
				}
			}()
			define()
		})
	}
}

// A name is unique within its namespace, not across them: two modules name
// their steps without knowing each other, and the qualified names stay apart.
func TestNamespaces(t *testing.T) {
	other := appstep.DefineNamespace("test_other")
	d := appstep.Define(other, "parse_file")
	if d.String() != "test_other.parse_file" || d == parseFile {
		t.Errorf("a same-named step of another namespace is %v", d)
	}
	for name, define := range map[string]func(){
		"a duplicate namespace":   func() { appstep.DefineNamespace("test") },
		"an empty namespace":      func() { appstep.DefineNamespace("") },
		"a dotted namespace":      func() { appstep.DefineNamespace("a.b") },
		"a hyphenated namespace":  func() { appstep.DefineNamespace("anfra-cloud") },
		"a capitalised namespace": func() { appstep.DefineNamespace("Anfra") },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("defining %s did not panic", name)
				}
			}()
			define()
		})
	}
}
