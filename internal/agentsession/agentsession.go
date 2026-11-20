// Package agentsession finds the session of the coding agent that runs a
// command, so that every call the agent makes for one piece of work can be
// found together (docs/designs/telemetry.md, phase 1).
//
// The agent owns its session: it starts the id and changes it, on /clear say.
// anfra only reads the id where the agent already puts it, in the environment
// of each command it runs, and stores nothing between commands. Each agent
// uses its own variable, so they are listed here, one entry each. For an agent
// that exposes none, whoever launches it sets ExplicitVar, which every command
// it runs inherits.
//
// The id is attribution: it labels what a command records, and never changes
// what the command does.
package agentsession

import "unicode"

// ExplicitVar holds a session id that whoever launches the agent chooses, for
// an agent that exposes no id of its own. A known agent's id wins over it: the
// agent's id matches its own telemetry.
const ExplicitVar = "ANFRA_SESSION_ID"

// Session is the agent session a command runs in.
type Session struct {
	ID    string // the known agent's own id, or else ExplicitVar's
	Agent string // which agent, as anfra names it (internal/agents); empty for ExplicitVar's id
}

// agent is a coding agent that gives its session id to the commands it runs,
// and to the MCP servers it calls.
type agent struct {
	name    string // as internal/agents names it
	idVar   string // the variable that holds the session id
	metaKey string // the tool call's _meta key that holds it, if the agent sends one
}

// agents are the agents whose session id anfra reads, in the order it looks.
// An agent run by another inherits the outer agent's variable too, so the
// first match isn't always the innermost agent.
var agents = []agent{
	{name: "claude-code", idVar: "CLAUDE_CODE_SESSION_ID"},
	{name: "codex", idVar: "CODEX_SESSION_ID", metaKey: "sessionId"}, // the root session, shared with its subagents
	{name: "opencode", idVar: "OPENCODE_SESSION_ID"},
	{name: "goose", idVar: "AGENT_SESSION_ID", metaKey: "agent-session-id"}, // only Goose sets it, though the name sounds generic
}

// maxIDLen bounds an id, which goes into every span and log record.
const maxIDLen = 128

// FromEnv is the session of the agent that runs this command, found in its
// environment through getenv, and whether one was found: the first known
// agent's, else ExplicitVar's. An id that is too long or holds a control
// character is not one, and the next is looked for.
func FromEnv(getenv func(string) string) (Session, bool) {
	for _, a := range agents {
		if id := getenv(a.idVar); validID(id) {
			return Session{ID: id, Agent: a.name}, true
		}
	}
	if id := getenv(ExplicitVar); validID(id) {
		return Session{ID: id}, true
	}
	return Session{}, false
}

// FromMeta is the session of the agent that makes an MCP tool call, found in
// the call's _meta through get (a member's string value, "" for none), and
// whether one was found: the first known agent's key that holds an id. The
// server's own environment is never read: it outlives the sessions it serves.
func FromMeta(get func(key string) string) (Session, bool) {
	for _, a := range agents {
		if a.metaKey == "" {
			continue
		}
		if id := get(a.metaKey); validID(id) {
			return Session{ID: id, Agent: a.name}, true
		}
	}
	return Session{}, false
}

// validID reports whether id can label a span and a log record.
func validID(id string) bool {
	if id == "" || len(id) > maxIDLen {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
