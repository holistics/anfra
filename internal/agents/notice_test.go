package agents

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttention(t *testing.T) {
	behindPlugin := func(t *testing.T, env *fakeEnv) {
		writeState(state{Plugins: map[string][]Installed{"claude-code": {{"0.4.5", "user"}}}})
	}
	behindCopy := func(dir string) func(*testing.T, *fakeEnv) {
		return func(t *testing.T, env *fakeEnv) {
			writeSkill(t, filepath.Join(env.Home, dir), "anfra-aql", stamped("0.4.5"))
		}
	}
	deprecated := func(t *testing.T, env *fakeEnv) {
		writeSkill(t, filepath.Join(env.Home, ".cursor/skills"), "setup-repo", "Run `anfra version`.\n")
	}
	for _, tc := range []struct {
		name  string
		repo  bool // in an anfra repo with no AGENTS.md
		setup []func(*testing.T, *fakeEnv)
		want  string
	}{
		{"nothing", false, nil, ""},
		{"a plugin behind", false, []func(*testing.T, *fakeEnv){behindPlugin},
			"anfra skills 0.5.0 are out: the Claude Code plugin is behind. Run `anfra agent setup`."},
		{"two behind", false, []func(*testing.T, *fakeEnv){behindPlugin, behindCopy(".agents/skills")},
			"anfra skills 0.5.0 are out: the Claude Code plugin and ~/.agents/skills are behind. Run `anfra agent setup`."},
		{"three behind, and deprecated skills", false, []func(*testing.T, *fakeEnv){behindPlugin, behindCopy(".agents/skills"), behindCopy(".gemini/skills"), deprecated},
			"anfra skills 0.5.0 are out: the Claude Code plugin and ~/.agents/skills are behind, and 2 more things need attention. Run `anfra agent setup`."},
		{"deprecated skills alone", false, []func(*testing.T, *fakeEnv){deprecated},
			"~/.cursor/skills has deprecated anfra skills. Run `anfra agent setup`."},
		{"the block, and deprecated skills", true, []func(*testing.T, *fakeEnv){deprecated},
			"~/.cursor/skills has deprecated anfra skills, and 1 more thing needs attention. Run `anfra agent setup`."},
		{"the block alone", true, nil,
			"This repo's AGENTS.md has no anfra block. Run `anfra agent setup`."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newFakeEnv(t)
			cacheSkills(t, "0.5.0")
			for _, f := range tc.setup {
				f(t, env)
			}
			repoDir := ""
			if tc.repo {
				repoDir = t.TempDir()
			}
			if got := Attention(env.Env, repoDir); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// The notice is the same line, at most once a day.
func TestNoticeOnceADay(t *testing.T) {
	env := newFakeEnv(t)
	cacheSkills(t, "0.5.0")
	writeSkill(t, filepath.Join(env.Home, ".agents/skills"), "anfra-aql", stamped("0.4.5"))
	if got := Notice(env.Env, ""); got == "" {
		t.Fatal("no notice")
	}
	if got := Notice(env.Env, ""); got != "" {
		t.Errorf("a second notice within a day: %q", got)
	}
	s := readState()
	s.NoticedAt = s.NoticedAt.Add(-noticeEvery)
	writeState(s)
	if got := Notice(env.Env, ""); got == "" {
		t.Error("no notice a day later")
	}
}

// What setup installed is recorded for the notice, without asking the agents
// again.
func TestRecordPlugins(t *testing.T) {
	env := newFakeEnv(t, "claude")
	env.lists["claude"] = claudeList
	RecordPlugins(t.Context(), env.Env)
	if got := readState().Plugins["claude-code"]; len(got) != 1 || got[0].Version != "0.4.5" {
		t.Errorf("recorded %v", got)
	}
	if _, err := os.Stat(statePath()); err != nil {
		t.Error(err)
	}
}

// The agents have no skills: said only when an agent is detected and the
// plugins have been recorded, so a machine without agents, or one not checked
// yet, hears nothing.
func TestAttentionWithNoSkills(t *testing.T) {
	const want = "Your coding agents have no anfra skills. Run `anfra agent setup` to install them."

	env := newFakeEnv(t, "opencode")
	cacheSkills(t, "0.15.0")
	if got := Attention(env.Env, ""); got != "" {
		t.Errorf("plugins not recorded yet: %q", got)
	}
	writeState(state{CheckedAt: time.Now()})
	if got := Attention(env.Env, ""); got != want {
		t.Errorf("got %q", got)
	}
	writeSkill(t, filepath.Join(env.Home, ".agents/skills"), "anfra-aql", stamped("0.15.0"))
	if got := Attention(env.Env, ""); got != "" {
		t.Errorf("with skills in a folder: %q", got)
	}

	env = newFakeEnv(t) // no agent on this machine
	cacheSkills(t, "0.15.0")
	writeState(state{CheckedAt: time.Now()})
	if got := Attention(env.Env, ""); got != "" {
		t.Errorf("no agent: %q", got)
	}
}
