package main

import (
	"errors"
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
			return refused("--scope", "invalid", fmt.Sprintf("--scope %s is no longer supported: use `anfra agent setup`, which installs per user.", scope))
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
			"Running it again updates what it installed. It installs per user, for every project.\n" +
			"`anfra agent setup check` changes nothing: it reports what setup would find.\n\n" +
			"Its result is what setup did for each agent, then the state of the agents' skills as setup leaves\n" +
			"it. When setup leaves an agent without the skills, it fails with command_failed, whose details are\n" +
			"that result.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deprecated != "" {
				printWarning(cmd.ErrOrStderr(), "deprecated", deprecated)
			}
			if check {
				printWarning(cmd.ErrOrStderr(), "deprecated", "--check is deprecated: use `anfra agent setup check`.")
			}
			return agentSetupRun(cmd, ids, check)
		},
	}
	markPrettyOutput(cmd)
	resultIs[agents.SetupResult](cmd)
	agentFlag := func(c *cobra.Command) {
		c.Flags().StringSliceVar(&ids, "agent", nil, "the agents to set up, by id (one or more of: "+strings.Join(agents.IDs(), ", ")+"); default: those found on this machine")
	}
	agentFlag(cmd)
	// --check was setup check's name: it still works, warning.
	cmd.Flags().BoolVar(&check, "check", false, "former name of `anfra agent setup check`")
	_ = cmd.Flags().MarkHidden("check")
	if deprecated == "" {
		checkCmd := &cobra.Command{
			Use:   "check",
			Short: "Check the anfra skills in your coding agents, without changing anything",
			Long: "Check the anfra skills in your coding agents, as setup would find them, and change nothing:\n" +
				"each agent, the copies of the skills anfra can find, and, in an anfra repo, its AGENTS.md block.\n\n" +
				"Its result is the state of the agents' skills.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return agentSetupRun(cmd, ids, true) },
		}
		agentFlag(checkCmd)
		markPrettyOutput(checkCmd)
		resultIs[agents.CheckResult](checkCmd)
		cmd.AddCommand(checkCmd)
	}
	return cmd
}

// agentSetupRun runs agent setup, or its check, and writes its result.
func agentSetupRun(cmd *cobra.Command, ids []string, check bool) error {
	o := agents.Options{Agents: ids, RepoDir: repoDirIfAny()}
	if stderrIsInteractive() {
		o.Progress = cmd.ErrOrStderr()
	}
	if check {
		r, err := agents.Check(cmd.Context(), agents.System(), o)
		if err != nil {
			return err
		}
		return writeResult(cmd, r)
	}
	res, err := agents.Setup(cmd.Context(), agents.System(), o)
	if errors.Is(err, agents.ErrIncomplete) {
		return commandFailedWith("The anfra skills are not set up for every agent.", res)
	}
	if err != nil {
		return err
	}
	return writeResult(cmd, res)
}

// repoDirIfAny is the current directory when it is an anfra repo, else "".
func repoDirIfAny() string {
	dir, _ := anfraRepoDir()
	return dir
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
