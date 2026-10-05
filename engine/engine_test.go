package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/holistics/anfra/engine"
)

// The tests in this file deliberately import only the public package, exactly
// as a consumer in another module would. If any of them needs something from
// internal/, that is a signal the facade is missing something — not a reason to
// reach past it.

func TestCommandsReportsTheRegistryByNameOnly(t *testing.T) {
	names := engine.Commands()
	if len(names) == 0 {
		t.Fatal("Commands() returned nothing")
	}
	// version is the one command certain to exist and to need no sidecars.
	var found bool
	for _, n := range names {
		if n == "version" {
			found = true
		}
		if n == "" {
			t.Error("Commands() returned an empty name")
		}
	}
	if !found {
		t.Errorf("Commands() = %v, expected it to include \"version\"", names)
	}
}

// A caller must not be able to mutate the registry through the slice it is
// handed.
func TestCommandsReturnsACopy(t *testing.T) {
	first := engine.Commands()
	if len(first) == 0 {
		t.Fatal("Commands() returned nothing")
	}
	first[0] = "clobbered"
	if engine.Commands()[0] == "clobbered" {
		t.Error("mutating the returned slice changed the registry")
	}
}

// The headline guarantee of this package: a host that has not said what the
// caller may see gets nothing, for every command.
func TestDispatchRefusesUndecidedPermissions(t *testing.T) {
	for _, name := range engine.Commands() {
		_, err := engine.Dispatch(context.Background(), engine.Invocation{}, engine.Request{Command: name})
		if err == nil {
			t.Errorf("command %q ran with undecided DataPerms", name)
			continue
		}
		if !strings.Contains(err.Error(), "no data permissions") {
			t.Errorf("command %q: want a data-permissions refusal, got: %v", name, err)
		}
	}
}

func TestUnrestrictedLetsACommandRun(t *testing.T) {
	res, err := engine.Dispatch(context.Background(),
		engine.Invocation{DataPerms: engine.Unrestricted()},
		engine.Request{Command: "version"})
	if err != nil {
		t.Fatalf("version under Unrestricted: %v", err)
	}
	if res.Status != engine.StatusOK {
		t.Errorf("status = %q, want %q", res.Status, engine.StatusOK)
	}
}

func TestRestrictedLetsACommandRun(t *testing.T) {
	inv := engine.Invocation{
		DataPerms:   engine.Restricted(engine.Attributes{"region": "APAC"}),
		Attribution: engine.Attribution{"tenant": "acme", "user": "u-1"},
	}
	if _, err := engine.Dispatch(context.Background(), inv, engine.Request{Command: "version"}); err != nil {
		t.Fatalf("version under Restricted: %v", err)
	}
}

// Naming the caller is not the same as saying what they may see, and the API
// must not let the two be confused.
func TestAttributionAloneIsNotEnough(t *testing.T) {
	inv := engine.Invocation{Attribution: engine.Attribution{"tenant": "acme", "user": "u-1"}}
	_, err := engine.Dispatch(context.Background(), inv, engine.Request{Command: "version"})
	if err == nil {
		t.Fatal("an Attribution was accepted in place of a permissions decision")
	}
}

func TestConnectRequiresBothURLs(t *testing.T) {
	for _, tc := range []struct{ node, canal string }{
		{"", ""},
		{"http://node:8080", ""},
		{"", "http://canal:11320"},
	} {
		_, _, err := engine.Connect(context.Background(), tc.node, tc.canal)
		if err == nil {
			t.Errorf("Connect(%q, %q) succeeded; both URLs are required", tc.node, tc.canal)
		}
	}
}

func TestOpenRepoResolvesIdentity(t *testing.T) {
	r := engine.OpenRepo(t.TempDir())
	if r.ID == "" {
		t.Error("OpenRepo returned a repo with no ID")
	}
	if r.Dir == "" {
		t.Error("OpenRepo returned a repo with no Dir")
	}
}

// Describe covers what Commands lists, through the public API alone.
func TestDescribeMatchesCommands(t *testing.T) {
	specs := engine.Describe()
	names := engine.Commands()
	if len(specs) != len(names) {
		t.Fatalf("%d specs for %d commands", len(specs), len(names))
	}
	for i, s := range specs {
		if s.Name != names[i] || s.Short == "" || len(s.ErrorCodes) == 0 {
			t.Errorf("spec %d: %+v", i, s)
		}
	}
}
