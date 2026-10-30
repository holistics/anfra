package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/repo"
)

// newAgentCmd groups the commands that set up coding agents for anfra.
func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Set up the anfra skills in your coding agents",
	}
	cmd.AddCommand(newAgentSetupCmd("setup", ""))
	return cmd
}

// newSkillsCmd is agent setup's former name, kept as an alias for now: a
// command keeps working, with a warning, before it is removed.
func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "skills",
		Short:  "Former name of `anfra agent setup`",
		Hidden: true,
	}
	install := newAgentSetupCmd("install", "This command is deprecated. Use `anfra agent setup` instead.")
	// Its --scope project installed into the repo's Claude Code settings, which
	// agent setup doesn't: refused with what to run instead.
	var scope string
	install.Flags().StringVar(&scope, "scope", "user", "where to install: user only")
	_ = install.Flags().MarkHidden("scope")
	run := install.RunE
	install.RunE = func(cmd *cobra.Command, args []string) error {
		if scope != "user" {
			return fmt.Errorf("--scope %s is no longer supported: use `anfra agent setup`, which installs per user", scope)
		}
		return run(cmd, args)
	}
	cmd.AddCommand(install)
	return cmd
}

// newAgentSetupCmd installs the skills into the user's coding agents
// (docs/designs/agent-setup.md). It is a CLI command only, never an op: it runs
// programs on this machine, which a server must not do for its callers.
// deprecated, when set, is printed first: the command's former name ran it.
func newAgentSetupCmd(use, deprecated string) *cobra.Command {
	var ids []string
	var check bool
	cmd := &cobra.Command{
		Use:   use,
		Short: "Install or update the anfra skills in your coding agents (" + agents.SkillsRepo + ")",
		Long: "Install the " + agents.Plugin + " plugin from " + agents.SkillsRepo + ": skills for working with anfra, " +
			"an aql-writer agent, and hooks.\n\n" +
			"For Claude Code and Codex, anfra installs it through their CLIs. Any other agent is pointed at the skills " +
			"repo's install steps for it. Without --agent, setup acts on every agent it finds on this machine. " +
			"Running it again updates what it installed. It installs per user, for every project.\n\n" +
			"--check changes nothing: it reports each agent, and the copies of the skills anfra can find.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deprecated != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), deprecated)
			}
			o := agents.Options{Agents: ids, Out: cmd.OutOrStdout()}
			if dir, ok := anfraRepoDir(); ok {
				o.RepoDir = dir
			}
			if check {
				return agents.Check(cmd.Context(), agents.System(), o)
			}
			return agents.Setup(cmd.Context(), agents.System(), o)
		},
	}
	cmd.Flags().StringSliceVar(&ids, "agent", nil, "the agents to set up, by id (one or more of: "+strings.Join(agents.IDs(), ", ")+"); default: those found on this machine")
	cmd.Flags().BoolVar(&check, "check", false, "report what is set up, and change nothing")
	return cmd
}

// anfraRepoDir is the current directory, when it is an anfra repo.
func anfraRepoDir() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	if fi, err := os.Stat(repo.Resolve(dir).ConfigDir); err != nil || !fi.IsDir() {
		return "", false
	}
	return dir, true
}
