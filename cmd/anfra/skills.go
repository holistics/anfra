package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// The agent skills for anfra, published as a plugin marketplace that Claude
// Code, Codex and Cursor each read in their own format.
const (
	skillsRepo        = "holistics/anfra-skills"
	skillsMarketplace = "anfra-skills"
	skillsPlugin      = "anfra-development"
)

// skillsAgent is a coding agent the skills can be installed into: through its
// own CLI when it has one that installs plugins, else by steps the user follows.
type skillsAgent struct {
	name  string // the --agent value
	label string
	bin   string // its CLI; empty when it cannot install a plugin
	// steps are the args to bin, run in order, for a scope; each is safe to
	// repeat. Nil for a scope the agent cannot install into.
	steps func(scope string) [][]string
	note  string // printed after installing
}

// Scopes: the user's home, for every project; or the current directory, in
// settings that can be committed so a repo's collaborators are offered it.
const (
	scopeUser    = "user"
	scopeProject = "project"
)

var skillsAgents = []skillsAgent{
	{
		name: "claude", label: "Claude Code", bin: "claude",
		steps: func(scope string) [][]string {
			return [][]string{
				{"plugin", "marketplace", "add", skillsRepo, "--scope", scope},
				// add leaves a marketplace that is already there as it was; update
				// fetches the latest, so running this again upgrades the skills.
				{"plugin", "marketplace", "update", skillsMarketplace},
				{"plugin", "install", skillsPlugin + "@" + skillsMarketplace, "--scope", scope},
			}
		},
		note: "Restart Claude Code, or run /reload-plugins, to load it.",
	},
	{
		name: "codex", label: "Codex", bin: "codex",
		// Codex keeps plugins in the user's config only (~/.codex/config.toml).
		steps: func(scope string) [][]string {
			if scope != scopeUser {
				return nil
			}
			return [][]string{
				{"plugin", "marketplace", "add", skillsRepo},
				{"plugin", "marketplace", "upgrade", skillsMarketplace},
				{"plugin", "add", skillsPlugin + "@" + skillsMarketplace},
			}
		},
		note: "Restart Codex to load it. The aql-writer agent is not included: Codex plugins carry skills and hooks only.",
	},
	{
		name: "cursor", label: "Cursor",
		note: "Cursor installs plugins from its own marketplace UI, not from a command:\n" +
			"  1. An admin imports https://github.com/" + skillsRepo + " under Dashboard → Settings → Plugins → Team Marketplaces.\n" +
			"  2. Each user installs " + skillsPlugin + " from the marketplace panel in Cursor, or with /plugin in cursor-agent.",
	},
}

// otherAgentsHint is for agents without a plugin format: the skills alone,
// through the skills CLI (https://github.com/vercel-labs/skills).
const otherAgentsHint = "For other agents, install the skills alone (without the aql-writer agent and the AML validation hook):\n" +
	"  npx skills add " + skillsRepo

// skillsEnv is what installing touches outside this process, swapped in tests.
type skillsEnv struct {
	lookPath func(string) (string, error)
	run      func(ctx context.Context, bin string, args ...string) error
	out      io.Writer
}

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage the anfra skills for coding agents",
	}
	cmd.AddCommand(newSkillsInstallCmd())
	return cmd
}

// newSkillsInstallCmd installs the skills into the user's coding agents. It is
// a CLI command only, never an op: it runs programs on this machine, which a
// server must not do for its callers.
func newSkillsInstallCmd() *cobra.Command {
	var agents []string
	var scope string
	names := make([]string, len(skillsAgents))
	for i, a := range skillsAgents {
		names[i] = a.name
	}
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the anfra skills (" + skillsRepo + ") into your coding agents",
		Long: "Install the " + skillsPlugin + " plugin from " + skillsRepo + ": skills for writing AQL and AML, " +
			"an aql-writer agent, and a hook that validates edited AML files.\n\n" +
			"Without --agent, installs into each of Claude Code and Codex whose CLI is on your PATH. " +
			"Running it again upgrades the skills to the latest version.\n\n" +
			"--scope project installs into the current directory's .claude/settings.json, which can be committed " +
			"so the repo's collaborators are offered the skills. Claude Code only: Codex installs plugins per user.\n\n" +
			otherAgentsHint,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env := skillsEnv{lookPath: exec.LookPath, run: runAttached, out: cmd.OutOrStdout()}
			return installSkills(cmd.Context(), env, agents, scope)
		},
	}
	cmd.Flags().StringSliceVar(&agents, "agent", nil, "the agents to install into (one or more of: "+strings.Join(names, ", ")+")")
	cmd.Flags().StringVar(&scope, "scope", scopeUser, "where to install (one of: user, project)")
	return cmd
}

func installSkills(ctx context.Context, env skillsEnv, names []string, scope string) error {
	if scope != scopeUser && scope != scopeProject {
		return fmt.Errorf("unknown scope %q: use user or project", scope)
	}
	agents, err := selectSkillsAgents(env, names, scope)
	if err != nil {
		return err
	}
	for i, a := range agents {
		if i > 0 {
			fmt.Fprintln(env.out)
		}
		if a.bin == "" {
			fmt.Fprintf(env.out, "%s\n", a.note)
			continue
		}
		steps := a.steps(scope)
		if steps == nil {
			fmt.Fprintf(env.out, "Skipped %s: it installs plugins per user only. Run without --scope project to install it there.\n", a.label)
			continue
		}
		fmt.Fprintf(env.out, "Installing %s for %s (%s scope)...\n", skillsPlugin, a.label, scope)
		for _, args := range steps {
			if err := env.run(ctx, a.bin, args...); err != nil {
				return fmt.Errorf("install skills for %s: `%s %s`: %w", a.label, a.bin, strings.Join(args, " "), err)
			}
		}
		fmt.Fprintf(env.out, "Installed %s for %s. %s\n", skillsPlugin, a.label, a.note)
	}
	return nil
}

// selectSkillsAgents resolves --agent; without it, the agents whose CLI is on
// the PATH. An agent named explicitly must have its CLI, if it installs by one,
// and support the scope.
func selectSkillsAgents(env skillsEnv, names []string, scope string) ([]skillsAgent, error) {
	if len(names) == 0 {
		var found []skillsAgent
		for _, a := range skillsAgents {
			if a.bin == "" {
				continue
			}
			if _, err := env.lookPath(a.bin); err == nil {
				found = append(found, a)
			}
		}
		if len(found) == 0 {
			return nil, errors.New("found neither `claude` (Claude Code) nor `codex` (Codex) on your PATH. " +
				"Install one of them, or run `anfra skills install --agent cursor` for Cursor's steps.\n" + otherAgentsHint)
		}
		return found, nil
	}

	var out []skillsAgent
	for _, name := range names {
		i := indexSkillsAgent(name)
		if i < 0 {
			valid := make([]string, len(skillsAgents))
			for j, a := range skillsAgents {
				valid[j] = a.name
			}
			return nil, fmt.Errorf("unknown agent %q: use one of %s.\n%s", name, strings.Join(valid, ", "), otherAgentsHint)
		}
		a := skillsAgents[i]
		if a.steps != nil && a.steps(scope) == nil {
			return nil, fmt.Errorf("%s installs plugins per user only: run without --scope %s", a.label, scope)
		}
		if a.bin != "" {
			if _, err := env.lookPath(a.bin); err != nil {
				return nil, fmt.Errorf("%s's CLI `%s` is not on your PATH: install %s, then run this again", a.label, a.bin, a.label)
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func indexSkillsAgent(name string) int {
	for i, a := range skillsAgents {
		if a.name == name {
			return i
		}
	}
	return -1
}

// runAttached runs a command with this process's terminal, so the user sees
// its progress and can answer any prompt it asks.
func runAttached(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
