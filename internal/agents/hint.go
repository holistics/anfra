package agents

import (
	"os"
	"path/filepath"
	"time"

	"github.com/holistics/anfra/internal/home"
	"github.com/holistics/anfra/shared/jsonkit"
)

// After hintAfter failures within hintWithin, a failing command's error hints
// at agent setup, when the agents have no anfra skills: an agent that keeps
// failing at anfra's commands is likely working without them.
const (
	hintAfter  = 3
	hintWithin = time.Minute
)

// streak is the recent failures, and whether this streak's error has hinted at
// setup. It has its own file: the background check rewrites agents-check.json,
// and would undo a failure recorded meanwhile.
type streak struct {
	Failures []time.Time `json:"failures,omitempty"`
	Hinted   bool        `json:"hinted,omitempty"`
}

func streakPath() string { return filepath.Join(home.Dir(), "agents-failures.json") }

func readStreak() streak {
	var s streak
	if data, err := os.ReadFile(streakPath()); err == nil {
		_ = jsonkit.Unmarshal(data, &s)
	}
	return s
}

func writeStreak(s streak) {
	p := streakPath()
	if len(s.Failures) == 0 && !s.Hinted {
		_ = os.Remove(p)
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return
	}
	if data, err := jsonkit.Marshal(s); err == nil {
		_ = os.WriteFile(p, data, 0o600)
	}
}

// Failed records a failure the caller could fix (an invalid input, an unknown
// command or flag), off a terminal, and says what to hint in its error: "" but
// for the failure that makes hintAfter within hintWithin, once per streak, and
// only when an agent is detected and none has the skills. When the plugins
// have never been recorded, the hint is worded to hold either way.
func Failed(env Env, repoDir string, now time.Time) string {
	s := readStreak()
	var recent []time.Time
	for _, t := range s.Failures {
		if now.Sub(t) < hintWithin {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	s.Failures = recent
	hint := ""
	if len(s.Failures) >= hintAfter && !s.Hinted {
		hint = setupHint(env, repoDir, readState())
		s.Hinted = hint != ""
	}
	writeStreak(s)
	return hint
}

// Succeeded ends a streak of failures. A command that succeeds with none costs
// a missing file's stat.
func Succeeded() {
	if _, err := os.Stat(streakPath()); err == nil {
		writeStreak(streak{})
	}
}

// setupHint is the hint at agent setup, or "" when an agent has the skills or
// no agent is on this machine.
func setupHint(env Env, repoDir string, s state) string {
	detected := false
	for _, a := range Known {
		if a.Detected(env) {
			detected = true
			break
		}
	}
	if !detected || len(s.Plugins) > 0 || len(Copies(env, repoDir)) > 0 {
		return ""
	}
	if s.CheckedAt.IsZero() {
		return "If the anfra skills aren't installed for you, run `anfra agent setup`, then ask the user to restart you. They know anfra's commands and languages."
	}
	return "The anfra skills aren't installed for your coding agents. They know anfra's commands and languages, so run `anfra agent setup`, then ask the user to restart you."
}
