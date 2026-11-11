package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/pretty"
)

func ptr[T any](v T) *T { return &v }

// prettyCheck is a check's result with a plugin behind, copies mixed and
// deprecated, and a repo's block.
func prettyCheck() agents.CheckResult {
	return agents.CheckResult{
		Message: "Claude Code's plugin is behind 0.4.5, ~/.agents/skills has skills at different versions and ~/.claude/skills has deprecated skills.",
		Report: agents.Report{
			Skills: agents.LatestSkills{Latest: ptr("0.4.5"), CheckedAt: ptr(time.Now())},
			Agents: []agents.AgentState{
				{ID: "claude-code", Name: "Claude Code", Status: agents.AgentInstalled, Plugins: []agents.PluginState{{Scope: "user", Version: "0.4.4", Status: "behind"}}, Copies: []string{"~/.claude/skills"}},
				{ID: "cursor", Name: "Cursor", Status: agents.AgentInstalled, Plugins: []agents.PluginState{}, Copies: []string{"~/.agents/skills", "~/.cursor/skills"}},
				{ID: "opencode", Name: "OpenCode", Status: agents.AgentNotInstalled, Plugins: []agents.PluginState{}, Copies: []string{}, InstallFrom: "https://github.com/holistics/anfra-skills#opencode"},
			},
			Copies: []agents.CopyState{
				{Dir: "~/.agents/skills", Status: "mixed", Versions: []agents.SkillsVersion{
					{Version: ptr("0.4.4"), Skills: []string{"anfra-aql", "anfra-write-aql"}},
					{Version: ptr("0.4.5"), Skills: []string{"anfra-amql", "anfra-search"}},
				}},
				{Dir: "~/.claude/skills", Status: "deprecated", Versions: []agents.SkillsVersion{}, DeprecatedNames: true},
			},
			AgentsMD: &agents.AgentsMDState{Block: agents.BlockUpToDate, ClaudeMD: agents.ClaudeMDImported},
		},
		Hints: []string{"Run `anfra agent setup` to install or update them."},
	}
}

// agent setup --check's view: its message first, its hints last; objects as
// sections, lists of objects as tables, nested lists in full; empty and
// toned values as their tags say. Off a terminal, nothing is cut.
func TestPrettyCheck(t *testing.T) {
	var out bytes.Buffer
	if err := pretty.Render(&out, prettyCheck(), 0); err != nil {
		t.Fatal(err)
	}
	want := `Claude Code's plugin is behind 0.4.5, ~/.agents/skills has skills at different versions and ~/.claude/skills has deprecated skills.

Skills:
  Latest:      0.4.5
  Checked at:  today

Agents:
  NAME          STATUS          PLUGINS             COPIES
  Claude Code   installed       user 0.4.4 behind   ~/.claude/skills
  Cursor        installed       <none>              ~/.agents/skills,~/.cursor/skills
  OpenCode      not-installed   <none>              <none>

Copies:
  ~/.agents/skills:
    Status:  mixed
    Versions:
      VERSION   SKILLS
      0.4.4     anfra-aql,anfra-write-aql
      0.4.5     anfra-amql,anfra-search
    Deprecated names:  false

  ~/.claude/skills:
    Status:            deprecated
    Versions:          <none>
    Deprecated names:  true

AGENTS.md:
  Block:      up-to-date
  CLAUDE.md:  imported

Hints:
  Run ` + "`anfra agent setup`" + ` to install or update them.
`
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

// agent setup's view on a terminal too narrow: a table cuts the column its
// tag marks, and a table with no such column becomes blocks.
func TestPrettyNarrowTerminal(t *testing.T) {
	res := agents.SetupResult{
		Message: "Installed the anfra skills for Claude Code.",
		Setup: agents.SetupActions{Agents: []agents.AgentSetup{
			{Agent: "opencode", Name: "OpenCode", Outcome: agents.OutcomeInstructionsOnly, Message: "anfra doesn't install the skills for OpenCode: install them as https://github.com/holistics/anfra-skills#opencode says."},
		}},
		Report: prettyCheck().Report,
	}
	render := func(width int) string {
		var out bytes.Buffer
		if err := pretty.Render(&out, res, width); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	at100, at60 := render(100), render(60)
	for _, c := range []struct {
		got, want string
	}{
		{at100, "    OpenCode   instructions_only   anfra doesn't install the skills for OpenCode: install them as h…\n"},
		{at100, "Agents:\n  NAME          STATUS          PLUGINS             COPIES\n"},
		{at60, "Agents:\n  Claude Code:\n    Status:  installed\n    Plugins:\n"},
		{at60, "    Copies:   ~/.agents/skills\n              ~/.cursor/skills\n"},
	} {
		if !strings.Contains(c.got, c.want) {
			t.Errorf("lacks:\n%s\ngot:\n%s", c.want, c.got)
		}
	}
}
