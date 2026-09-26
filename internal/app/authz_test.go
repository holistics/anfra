package app

import (
	"context"
	"errors"
	"testing"

	"github.com/holistics/anfra/internal/authz"
)

// Every registered command must state its permissions, exactly as it must state
// its sidecars. Declaring authz.Public is a decision; leaving Requires nil is
// nobody having made one, and that is what this catches — the failure mode being
// a command added later that quietly bypasses authorization.
func TestEveryCommandDeclaresPermissions(t *testing.T) {
	for _, c := range Commands {
		if c.Requires == nil {
			t.Errorf("command %q has a nil Requires; declare authz.Public if it needs none", c.Name)
		}
	}
}

// probe is a synthetic command demanding a permission no policy grants by
// default, used to prove the enforcement point is on the dispatch path rather
// than merely declared on the struct.
func withProbe(t *testing.T, requires func(map[string]any) []authz.Permission) *bool {
	t.Helper()
	ran := false
	orig := Commands
	t.Cleanup(func() { Commands = orig })
	Commands = append(append([]Command{}, orig...), Command{
		Name:     "authz-probe",
		Short:    "test-only",
		Requires: requires,
		Run: func(context.Context, CommandContext, map[string]any) (any, error) {
			ran = true
			return "ok", nil
		},
	})
	return &ran
}

func adminOnRepo(map[string]any) []authz.Permission {
	return []authz.Permission{{Action: authz.ActionAdmin, Resource: authz.Resource{Kind: authz.KindRepo}}}
}

func TestDispatchRefusesWhenPolicyDenies(t *testing.T) {
	ran := withProbe(t, adminOnRepo)

	_, err := Dispatch(context.Background(),
		CommandContext{Policy: authz.DenyAll{}, Principal: authz.Principal{UserID: "u1"}},
		Request{Command: "authz-probe"})

	var denied *authz.DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("err = %v, want *authz.DeniedError", err)
	}
	if *ran {
		t.Error("the command body ran despite the policy denying it")
	}
}

func TestDispatchRunsWhenPolicyAllows(t *testing.T) {
	ran := withProbe(t, adminOnRepo)

	res, err := Dispatch(context.Background(),
		CommandContext{Policy: authz.AllowAll{}}, Request{Command: "authz-probe"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !*ran {
		t.Error("the command body did not run under AllowAll")
	}
	if res.Status != StatusOK {
		t.Errorf("status = %q, want %q", res.Status, StatusOK)
	}
}

// Undecided must read as no. A nil Requires is refused rather than panicking or
// being waved through, so the mistake is loud at runtime too — not only in the
// completeness test above.
func TestDispatchRefusesCommandWithNoDeclaredPermissions(t *testing.T) {
	ran := withProbe(t, nil)

	_, err := Dispatch(context.Background(),
		CommandContext{Policy: authz.AllowAll{}}, Request{Command: "authz-probe"})
	if err == nil {
		t.Fatal("Dispatch succeeded for a command with a nil Requires")
	}
	if *ran {
		t.Error("the command body ran despite declaring no permissions")
	}
}

// A command that needs permissions but was handed no policy is likewise refused:
// a missing policy is an absent decision, not an implicit yes.
func TestDispatchRefusesWhenNoPolicySupplied(t *testing.T) {
	ran := withProbe(t, adminOnRepo)

	_, err := Dispatch(context.Background(), CommandContext{}, Request{Command: "authz-probe"})
	if err == nil {
		t.Fatal("Dispatch succeeded with no policy")
	}
	if *ran {
		t.Error("the command body ran with no policy supplied")
	}
}

// Public commands stay reachable without a policy — that is what Public means,
// and the CLI relies on it.
func TestPublicCommandNeedsNoPolicy(t *testing.T) {
	ran := withProbe(t, authz.Public)

	if _, err := Dispatch(context.Background(), CommandContext{}, Request{Command: "authz-probe"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !*ran {
		t.Error("a public command did not run")
	}
}
