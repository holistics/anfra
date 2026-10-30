package agents

import (
	"strings"
	"testing"
	"time"
)

func TestFailedHintsOnTheThirdFailureInAMinute(t *testing.T) {
	env := newFakeEnv(t, "opencode")
	writeState(state{CheckedAt: time.Now()})
	now := time.Now()
	for i := range 2 {
		if got := Failed(env.Env, "", now.Add(time.Duration(i)*time.Second)); got != "" {
			t.Fatalf("failure %d hinted: %q", i+1, got)
		}
	}
	got := Failed(env.Env, "", now.Add(2*time.Second))
	if !strings.HasPrefix(got, "The anfra skills aren't installed for your coding agents.") {
		t.Errorf("the third failure: %q", got)
	}
	// Once per streak.
	if got := Failed(env.Env, "", now.Add(3*time.Second)); got != "" {
		t.Errorf("hinted again in the streak: %q", got)
	}
	// A success ends the streak; the next three hint again.
	Succeeded()
	for i := range 3 {
		got = Failed(env.Env, "", now.Add(time.Duration(10+i)*time.Second))
	}
	if got == "" {
		t.Error("no hint in a new streak")
	}
}

// Failures more than a minute apart don't add up.
func TestFailedOutsideTheMinute(t *testing.T) {
	env := newFakeEnv(t, "opencode")
	writeState(state{CheckedAt: time.Now()})
	now := time.Now()
	for i := range 3 {
		if got := Failed(env.Env, "", now.Add(time.Duration(i)*40*time.Second)); got != "" {
			t.Errorf("failure %d, 40s apart, hinted: %q", i+1, got)
		}
	}
}

func TestFailedHintsOnlyWithoutSkills(t *testing.T) {
	three := func(env *fakeEnv) string {
		now, got := time.Now(), ""
		for i := range 3 {
			got = Failed(env.Env, "", now.Add(time.Duration(i)*time.Second))
		}
		return got
	}

	env := newFakeEnv(t) // no agent on this machine
	writeState(state{CheckedAt: time.Now()})
	if got := three(env); got != "" {
		t.Errorf("no agent: %q", got)
	}

	env = newFakeEnv(t, "claude")
	writeState(state{CheckedAt: time.Now(), Plugins: map[string][]Installed{"claude-code": {{"0.15.0", "user"}}}})
	if got := three(env); got != "" {
		t.Errorf("a plugin installed: %q", got)
	}

	env = newFakeEnv(t, "opencode")
	writeSkill(t, env.Home+"/.agents/skills", "anfra-aql", stamped("0.15.0"))
	writeState(state{CheckedAt: time.Now()})
	if got := three(env); got != "" {
		t.Errorf("skills in a folder: %q", got)
	}

	// The plugins never recorded: the hint holds either way.
	env = newFakeEnv(t, "opencode")
	if got := three(env); !strings.HasPrefix(got, "If the anfra skills aren't installed for you,") {
		t.Errorf("never recorded: %q", got)
	}
}
