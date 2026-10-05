package app

import (
	"context"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/dataperm"
)

// Nothing dispatches until the host has said what the caller may see — and
// "nothing" means every registered command, with no exempt list.
//
// This iterates the registry rather than naming commands, so a command added
// later is covered without anyone remembering to cover it. That is the property
// the previous per-command DataAccess declaration could not give: an exemption
// list is only ever wrong in one direction.
func TestEveryCommandRefusedWithoutDataPerms(t *testing.T) {
	for _, c := range Commands {
		t.Run(c.Name(), func(t *testing.T) {
			_, err := Dispatch(context.Background(), CommandContext{}, Request{Command: c.Name()})
			if err == nil {
				t.Fatalf("command %q ran with undecided DataPerms", c.Name())
			}
			if !strings.Contains(err.Error(), "no data permissions") {
				t.Errorf("want a data-permissions refusal, got: %v", err)
			}
		})
	}
}

// The check is a precondition on the context, not on the request, so it does
// not depend on the request naming a real command — including the empty command
// that lists the registry.
func TestDispatchRefusesWithoutDataPermsBeforeLookingAtRequest(t *testing.T) {
	for _, req := range []Request{
		{},                           // the command listing
		{Command: "no-such-command"}, // never resolves
		{Command: "query"},           // resolves, but missing required args
	} {
		_, err := Dispatch(context.Background(), CommandContext{}, req)
		if err == nil || !strings.Contains(err.Error(), "no data permissions") {
			t.Errorf("request %+v: want a data-permissions refusal, got: %v", req, err)
		}
	}
}

// withProbe registers a synthetic command, used to prove the check gates the
// command body rather than merely returning an error beside it.
func withProbe(t *testing.T) *bool {
	t.Helper()
	ran := false
	orig := Commands
	t.Cleanup(func() { Commands = orig })
	Commands = append(append([]Command{}, orig...), Define(Def[NoInput, string]{
		Name:  "dataperm-probe",
		Short: "test-only",
		Run: func(context.Context, CommandContext, NoInput) (string, error) {
			ran = true
			return "ok", nil
		},
	}))
	return &ran
}

func dispatchProbe(cc CommandContext) (Response, error) {
	return Dispatch(context.Background(), cc, Request{Command: "dataperm-probe"})
}

// The refused command's body never runs.
func TestRefusalStopsTheCommandBody(t *testing.T) {
	ran := withProbe(t)

	if _, err := dispatchProbe(CommandContext{}); err == nil {
		t.Fatal("expected a refusal with undecided DataPerms")
	}
	if *ran {
		t.Error("the command body ran despite the refusal")
	}
}

// Stating that nothing is restricted is a decision, and it lets the command run.
func TestUnrestrictedIsADecision(t *testing.T) {
	ran := withProbe(t)

	res, err := dispatchProbe(CommandContext{DataPerms: dataperm.Unrestricted()})
	if err != nil {
		t.Fatalf("expected the command to run under Unrestricted, got: %v", err)
	}
	if !*ran {
		t.Error("the command body did not run")
	}
	if res.Status != StatusOK {
		t.Errorf("status = %q, want %q", res.Status, StatusOK)
	}
}

// So is carrying real attributes.
func TestRestrictedIsADecision(t *testing.T) {
	ran := withProbe(t)

	perms := dataperm.Restricted(dataperm.Attributes{"region": "APAC"})
	if _, err := dispatchProbe(CommandContext{DataPerms: perms}); err != nil {
		t.Fatalf("expected the command to run under Restricted, got: %v", err)
	}
	if !*ran {
		t.Error("the command body did not run")
	}
}

// Knowing who the caller is must not substitute for knowing what they may see.
// A fully populated Attribution still leaves the permissions undecided, and the
// command is still refused. This is the mechanical form of "identity is tracing
// baggage": it cannot buy access.
func TestAttributionIsNotAPermission(t *testing.T) {
	ran := withProbe(t)

	_, err := dispatchProbe(CommandContext{
		Attribution: attribution.Fields{"tenant": "acme", "user": "u-1"},
	})
	if err == nil {
		t.Fatal("expected a refusal: an Attribution is not a data-permission decision")
	}
	if !strings.Contains(err.Error(), "no data permissions") {
		t.Errorf("error should name the missing permissions, got: %v", err)
	}
	if *ran {
		t.Error("the command body ran with identity but no permissions")
	}
}
