package task

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/command"
)

// Only an id as task start makes it is a task's id, so a task's id holds
// nothing of the user's.
func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{
		"t_7f3a91c0d2e4":       true,
		"":                     false,
		"t_7F3A91C0D2E4":       false,
		"t_7f3a91c0d2e":        false,
		"t_7f3a91c0d2e4\n":     false,
		"task_7f3a91c0d2e4":    false,
		"revenue last quarter": false,
	} {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v, want %v", id, got, want)
		}
	}
}

// Each task gets an id of its own, which --task then takes.
func TestNewIDsAreValidAndApart(t *testing.T) {
	a, b := newID(), newID()
	if a == b || !ValidID(a) || !ValidID(b) {
		t.Errorf("got %q and %q", a, b)
	}
}

// An empty, oversized or non-UTF-8 goal is refused before the command runs.
func TestStartRefusesABadGoal(t *testing.T) {
	for name, goal := range map[string]string{
		"empty":     "  ",
		"too long":  strings.Repeat("a", maxText+1),
		"not UTF-8": "\xff",
	} {
		if err := Start.Check([]byte(`{"goal":` + strconv.Quote(goal) + `}`)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := Start.Check([]byte(`{"goal":"Revenue by region for Q3"}`)); err != nil {
		t.Errorf("a good goal was refused: %v", err)
	}
}

// A parent that isn't a task's id is refused, so a typo fails at once.
func TestStartRefusesABadParent(t *testing.T) {
	if err := Start.Check([]byte(`{"goal":"q","parent":"t_123"}`)); err == nil {
		t.Error("a malformed parent was accepted")
	}
	if err := Start.Check([]byte(`{"goal":"q","parent":"t_7f3a91c0d2e4"}`)); err != nil {
		t.Errorf("a good parent was refused: %v", err)
	}
}

// With telemetry off, start still gives an id, and both say nothing was
// recorded.
func TestATaskWithoutTelemetryIsNotRecorded(t *testing.T) {
	started, err := start(context.Background(), command.CommandContext{}, StartInput{Goal: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if started.Recorded || !ValidID(started.Task) || !strings.Contains(started.Message, "wasn't recorded") {
		t.Errorf("start: got %+v", started)
	}
	noted, err := note(context.Background(), command.CommandContext{}, NoteInput{Kind: "finding", Text: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if noted.Recorded || !strings.Contains(noted.Message, "wasn't recorded") {
		t.Errorf("note: got %+v", noted)
	}
}
