package agentsession

import (
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

// Each agent's own variable is read, and names the agent.
func TestEachAgentsSessionIsFound(t *testing.T) {
	for _, a := range agents {
		got, ok := FromEnv(env(map[string]string{a.idVar: "s-1"}))
		if !ok || got != (Session{ID: "s-1", Agent: a.name}) {
			t.Errorf("%s: got %+v, %v", a.idVar, got, ok)
		}
	}
}

// Outside an agent, there is no session.
func TestNoAgentNoSession(t *testing.T) {
	if got, ok := FromEnv(env(nil)); ok {
		t.Errorf("got %+v", got)
	}
}

// An id too long, or with a control character, is not one, and the next
// agent's is looked for.
func TestABadIDIsNotOne(t *testing.T) {
	for _, bad := range []string{strings.Repeat("a", maxIDLen+1), "s-1\nforged=1"} {
		got, ok := FromEnv(env(map[string]string{"CLAUDE_CODE_SESSION_ID": bad, "CODEX_SESSION_ID": "c-1"}))
		if !ok || got.Agent != "codex" {
			t.Errorf("%q: got %+v, %v, want codex's", bad, got, ok)
		}
	}
}

// An id the launcher sets is the session when no known agent gives one; a
// known agent's own wins over it. A bad one is not one.
func TestTheLaunchersSessionIsTheFallback(t *testing.T) {
	got, ok := FromEnv(env(map[string]string{ExplicitVar: "run-1"}))
	if !ok || got != (Session{ID: "run-1"}) {
		t.Errorf("alone: got %+v, %v", got, ok)
	}
	got, ok = FromEnv(env(map[string]string{ExplicitVar: "run-1", "CLAUDE_CODE_SESSION_ID": "s-1"}))
	if !ok || got != (Session{ID: "s-1", Agent: "claude-code"}) {
		t.Errorf("with an agent: got %+v, %v, want the agent's", got, ok)
	}
	if got, ok := FromEnv(env(map[string]string{ExplicitVar: "run-1\n"})); ok {
		t.Errorf("bad: got %+v", got)
	}
}

// A tool call's _meta gives the session of the agents that send one, under
// their own key; another agent's variable name isn't one.
func TestAToolCallsAgentSessionIsFound(t *testing.T) {
	for key, agent := range map[string]string{"sessionId": "codex", "agent-session-id": "goose"} {
		got, ok := FromMeta(env(map[string]string{key: "s-1"}))
		if !ok || got != (Session{ID: "s-1", Agent: agent}) {
			t.Errorf("%s: got %+v, %v", key, got, ok)
		}
	}
	for _, meta := range []map[string]string{nil, {"CLAUDE_CODE_SESSION_ID": "s-1"}, {"sessionId": "s-1\n"}} {
		if got, ok := FromMeta(env(meta)); ok {
			t.Errorf("%v: got %+v", meta, got)
		}
	}
}
