// Package agents sets up the anfra skills in the coding agents on this machine
// (docs/designs/agent-setup.md): a plugin through the agent's own CLI where
// anfra knows one, a pointer to the skills repo's README for every other agent.
// It runs programs on this machine, so only the CLI uses it, never an op.
package agents

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// The skills, published as a plugin marketplace that Claude Code and Codex
// each install from.
const (
	SkillsRepo  = "holistics/anfra-skills"
	Marketplace = "anfra-skills"
	Plugin      = "anfra"
	PluginID    = Plugin + "@" + Marketplace

	readmeURL = "https://github.com/" + SkillsRepo
)

// Agent is a coding agent anfra knows: how to find it on this machine, and how
// it gets the skills.
type Agent struct {
	ID   string // npx skills' id: the --agent value, and the README section's anchor
	Name string
	// homes are folders, relative to the user's home or starting with an
	// environment variable, whose presence means the agent is installed; bins
	// are CLIs that mean the same.
	homes []string
	bins  []string
	// reads are the skills folders it loads skills from, at the user's home
	// (relative to it, or starting with an environment variable), and
	// repoReads those in a repo. From each agent's docs, as of 2026-10
	// (docs/research/skills-install.md).
	reads, repoReads []string
	// plugin is how anfra installs the plugin into it; nil when anfra doesn't,
	// and points at the README instead.
	plugin *pluginRoute
}

// pluginRoute installs the plugin through an agent's own CLI.
type pluginRoute struct {
	bin string
	// install are the args to bin, run in order, to install the plugin for the
	// user; each is safe to repeat, so running them again updates it.
	install [][]string
	// list prints the installed plugins as JSON; installed reads ours from it.
	list      []string
	installed func(listJSON []byte) []Installed
	note      string // printed after installing
}

// Installed is an installed copy of the plugin: its version, and the scope the
// agent reports it at (user, project, …). anfra installs for the user only.
type Installed struct {
	Version string
	Scope   string
}

// Known are the agents anfra knows, in the order setup reports them. Each has a
// section in the skills repo's README, anchored by its id; adding one here
// means adding its section there.
var Known = []Agent{
	{
		ID: "claude-code", Name: "Claude Code", homes: []string{"$CLAUDE_CONFIG_DIR", ".claude"}, bins: []string{"claude"},
		reads: []string{"$CLAUDE_CONFIG_DIR/skills", ".claude/skills"}, repoReads: []string{".claude/skills"},
		plugin: &pluginRoute{
			bin: "claude",
			install: [][]string{
				{"plugin", "marketplace", "add", SkillsRepo, "--scope", "user"},
				// add leaves a marketplace that is already there as it was;
				// update fetches the latest, and plugin update moves an
				// installed plugin to it, so running this again upgrades.
				{"plugin", "marketplace", "update", Marketplace},
				{"plugin", "install", PluginID, "--scope", "user"},
				{"plugin", "update", PluginID, "--scope", "user"},
			},
			list:      []string{"plugin", "list", "--json"},
			installed: claudeInstalled,
			note:      "Restart Claude Code, or run /reload-plugins, to load it.",
		},
	},
	{
		ID: "codex", Name: "Codex", homes: []string{"$CODEX_HOME", ".codex"}, bins: []string{"codex"},
		reads: []string{".agents/skills"}, repoReads: []string{".agents/skills"},
		plugin: &pluginRoute{
			bin: "codex",
			install: [][]string{
				{"plugin", "marketplace", "add", SkillsRepo},
				{"plugin", "marketplace", "upgrade", Marketplace},
				{"plugin", "add", PluginID},
			},
			list:      []string{"plugin", "list", "--marketplace", Marketplace, "--json"},
			installed: codexInstalled,
			note:      "Restart Codex to load it. The aql-writer agent is not included: Codex plugins carry skills and hooks only.",
		},
	},
	{
		ID: "cursor", Name: "Cursor", homes: []string{".cursor"}, bins: []string{"cursor-agent"},
		reads:     []string{".agents/skills", ".cursor/skills", ".claude/skills", ".codex/skills"},
		repoReads: []string{".agents/skills", ".cursor/skills", ".claude/skills", ".codex/skills"},
	},
	{
		ID: "github-copilot", Name: "GitHub Copilot", homes: []string{".copilot"}, bins: []string{"copilot"},
		reads:     []string{".agents/skills", ".copilot/skills", ".claude/skills"},
		repoReads: []string{".agents/skills", ".github/skills", ".claude/skills"},
	},
	{
		ID: "opencode", Name: "OpenCode", homes: []string{"$XDG_CONFIG_HOME/opencode", ".config/opencode"}, bins: []string{"opencode"},
		reads:     []string{".agents/skills", "$XDG_CONFIG_HOME/opencode/skills", ".config/opencode/skills", ".claude/skills"},
		repoReads: []string{".agents/skills", ".opencode/skills", ".claude/skills"},
	},
	{
		ID: "gemini-cli", Name: "Gemini CLI", homes: []string{".gemini"}, bins: []string{"gemini"},
		reads: []string{".agents/skills", ".gemini/skills"}, repoReads: []string{".agents/skills", ".gemini/skills"},
	},
	{
		ID: "antigravity", Name: "Antigravity", homes: []string{".gemini/antigravity", ".gemini/antigravity-cli"}, bins: []string{"agy"},
		reads:     []string{".gemini/config/skills", ".gemini/antigravity/skills", ".gemini/antigravity-cli/skills"},
		repoReads: []string{".agents/skills"},
	},
	{
		ID: "junie", Name: "Junie", homes: []string{".junie"}, bins: []string{"junie"},
		reads: []string{".agents/skills", ".junie/skills"}, repoReads: []string{".agents/skills", ".junie/skills"},
	},
	{
		ID: "windsurf", Name: "Windsurf", homes: []string{".codeium/windsurf"},
		reads: []string{".agents/skills", ".codeium/windsurf/skills"}, repoReads: []string{".agents/skills", ".windsurf/skills", ".devin/skills"},
	},
	{
		ID: "cline", Name: "Cline", homes: []string{".cline"}, bins: []string{"cline"},
		reads:     []string{".agents/skills", ".cline/skills"},
		repoReads: []string{".agents/skills", ".cline/skills", ".clinerules/skills", ".claude/skills"},
	},
	{
		ID: "warp", Name: "Warp", homes: []string{".warp"},
		reads:     []string{".agents/skills", ".warp/skills", ".claude/skills", ".codex/skills", ".cursor/skills", ".gemini/skills", ".copilot/skills"},
		repoReads: []string{".agents/skills", ".warp/skills", ".claude/skills", ".codex/skills", ".cursor/skills", ".gemini/skills"},
	},
}

// Find is the known agent with id.
func Find(id string) (Agent, bool) {
	i := slices.IndexFunc(Known, func(a Agent) bool { return a.ID == id })
	if i < 0 {
		return Agent{}, false
	}
	return Known[i], true
}

// IDs are the known agents' ids.
func IDs() []string {
	ids := make([]string, len(Known))
	for i, a := range Known {
		ids[i] = a.ID
	}
	return ids
}

// Installs reports whether anfra installs the plugin into the agent itself,
// rather than pointing at the README.
func (a Agent) Installs() bool { return a.plugin != nil }

// README is the skills repo's install section for the agent.
func (a Agent) README() string { return ReadmeFor(a.ID) }

// ReadmeFor is the README's install section for an agent id: its own for a
// known agent, the one for any other agent otherwise, so the link always lands
// on a section that exists.
func ReadmeFor(id string) string {
	if _, ok := Find(id); ok {
		return readmeURL + "#" + id
	}
	return readmeURL + "#other-agents"
}

// Env is what the package touches outside this process, swapped in tests.
type Env struct {
	Home     string                       // the user's home folder
	Getenv   func(string) string          // the environment
	LookPath func(string) (string, error) // finds a CLI on the PATH
	// Run runs a command where the user sees it, and can answer what it asks.
	Run func(ctx context.Context, bin string, args ...string) error
	// Output runs a command and returns what it prints.
	Output func(ctx context.Context, bin string, args ...string) ([]byte, error)
}

// System is this machine's Env.
func System() Env {
	home, _ := os.UserHomeDir()
	return Env{
		Home:     home,
		Getenv:   os.Getenv,
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, bin string, args ...string) error {
			cmd, err := agentCommand(ctx, bin, args)
			if err != nil {
				return err
			}
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return cmd.Run()
		},
		Output: func(ctx context.Context, bin string, args ...string) ([]byte, error) {
			cmd, err := agentCommand(ctx, bin, args)
			if err != nil {
				return nil, err
			}
			return cmd.Output()
		},
	}
}

// agentCommand is a command of an agent CLI anfra installs through, named by a
// literal: whatever reaches bin, only these programs run. The args are passed
// as they are, with no shell to read them.
func agentCommand(ctx context.Context, bin string, args []string) (*exec.Cmd, error) {
	switch bin {
	case "claude":
		return exec.CommandContext(ctx, "claude", args...), nil
	case "codex":
		return exec.CommandContext(ctx, "codex", args...), nil
	}
	return nil, fmt.Errorf("anfra runs no program %q: only the agent CLIs it installs through", bin)
}

// Detected reports whether the agent is on this machine: one of its CLIs is on
// the PATH, or one of its folders exists.
func (a Agent) Detected(env Env) bool {
	for _, b := range a.bins {
		if _, err := env.LookPath(b); err == nil {
			return true
		}
	}
	for _, h := range a.homes {
		if dir := env.path(h); dir != "" {
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
				return true
			}
		}
	}
	return false
}

// path resolves a folder relative to the home, or starting with an environment
// variable ($CODEX_HOME/…); "" when that variable is unset.
func (env Env) path(p string) string {
	if strings.HasPrefix(p, "$") {
		name, rest, _ := strings.Cut(p[1:], "/")
		v := strings.TrimSpace(env.Getenv(name))
		if v == "" {
			return ""
		}
		return filepath.Join(v, rest)
	}
	return filepath.Join(env.Home, p)
}

// InstalledPlugin lists the agent's installed copies of the plugin, through
// its CLI. Nil when anfra doesn't install into it, its CLI isn't on the PATH,
// or the list fails. Agents' CLIs start slowly (Claude Code's starts Node), so
// this is for setup and update, never an ordinary command.
func (a Agent) InstalledPlugin(ctx context.Context, env Env) []Installed {
	if a.plugin == nil {
		return nil
	}
	if _, err := env.LookPath(a.plugin.bin); err != nil {
		return nil
	}
	out, err := env.Output(ctx, a.plugin.bin, a.plugin.list...)
	if err != nil {
		return nil
	}
	return a.plugin.installed(out)
}
