package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"syscall"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/envflag"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/repoenv"

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/shared/apperr"
	caller "github.com/holistics/anfra/shared/telemetry"
	"github.com/spf13/cobra"
)

// exitCodeError makes a command exit with a specific code without printing an
// error message — the output was already rendered (see presentTo()).
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

func main() {
	// The repo's .env, before anything reads a setting: each name in it applies
	// as if it were in the environment, which wins (internal/repoenv).
	loadRepoEnv()

	// anfra's switches are 1 or 0. Refuse any other value before anything reads
	// one, so a typo stops the command instead of silently meaning the opposite.
	if err := envflag.Validate(); err != nil {
		output.json = jsonAsked(os.Args[1:])
		printError(os.Stderr, settingRefused(err), "") // the user's setting, said as it is: not anfra's failure
		os.Exit(1)
	}

	// What of the user's data spans record, read once (docs/designs/telemetry.md).
	resolveTraceData()

	// The CLI and `anfra serve` are usually local: their user is the operator, who
	// needs an error's cause where the error shows, not only in a log. An anfra
	// serving other people hides causes with ANFRA_HIDE_ERROR_CAUSES, as a
	// platform embedding the engine does by default.
	if !envflag.On(envflag.HideErrorCauses) {
		apperr.DisableErrorEncapsulation()
	}

	// A single signal-cancelable root context, threaded down through cobra so
	// Ctrl-C (SIGINT/SIGTERM) cancels in-flight work — an update download, a
	// query, or the serve loop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	// Traces, when an OTEL_* endpoint asks for them; flushed before any exit below.
	// Its warnings come before run reads the flags, so --json is looked for here.
	output.json = output.json || jsonAsked(os.Args[1:])
	flush := startTelemetry(ctx)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, runOptions{})
	flush()
	stop()
	os.Exit(code)
}

// runOptions are how a run differs from `anfra <args>` at a shell: inConsole,
// a console's run, a person's, its YAML and JSON colored at a terminal and its
// JSON indented.
type runOptions struct {
	inConsole bool
}

// run runs the command args name on a command tree of its own, as `anfra
// <args>` does, and is its exit code: writing its answer to stdout, its error
// to stderr. A process can run many, one after another (docs/implementations/
// console.md): each tree starts from the flags' defaults.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, opts runOptions) int {
	root := newRootCmd() //nolint:contextcheck // builds commands; each runs on ctx, through ExecuteContextC
	output.inConsole = opts.inConsole
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	endTrace := traceCommands(ctx, root, args)
	// ExecuteContextC sets that root context and returns the command that actually
	// ran, so we get the real subcommand name (handles flags/args/aliases).
	executed, err := root.ExecuteContextC(ctx)
	if _, rendered := errors.AsType[*exitCodeError](err); err != nil && !rendered {
		err = runError(ctx, err)
	}
	endTrace(executed, err)
	// After the command runs, surface a cached "update available" notice (and, if
	// opted in, kick a background update). Best-effort; never affects exit status.
	// Not after a failure: the error, or the invalid answer, is what to read, and
	// the notice keeps until a command succeeds.
	if executed != nil && err == nil {
		maybeNotifyUpdate(executed.Name())
		agents.Succeeded() // ends a streak of failures (agents.Failed)
	}
	code := exitCode(err)
	if _, rendered := errors.AsType[*exitCodeError](err); err == nil || rendered {
		return code // a rendered result exits quietly
	}
	output.json = output.json || jsonAsked(args) // flags that failed to parse set nothing
	output.yaml = output.yaml || flagAsked(args, "--yaml")
	printError(stderr, err, failureHint(err))
	return code
}

// exitCode is the code a run that ended with err exits with: 0 for none, the
// code a rendered result asks for, 130 for an interrupt, as a shell exits on
// one, and 1 for any other failure.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ec, ok := errors.AsType[*exitCodeError](err); ok {
		return ec.code
	}
	if apperr.From(err).Code == clierr.Interrupted.Code() {
		return 130
	}
	return 1
}

// runError is a failed run's err as the CLI tells it: a run the user
// interrupted (Ctrl-C) as interrupted, whatever failed as it stopped; one
// canceled at a question (Esc, or its Cancel) as interrupted too; an unknown
// command, cobra's, as unknown_command.
func runError(ctx context.Context, err error) error {
	switch code := apperr.From(err).Code; {
	case code == clierr.Interrupted.Code():
		return err
	case ctx.Err() != nil:
		return apperr.Encapsulate(err, clierr.Interrupted, "Interrupted.")
	case errors.Is(err, context.Canceled):
		return apperr.Encapsulate(err, clierr.Interrupted, "Canceled.")
	case code == apperr.InternalServerError.Code() && strings.HasPrefix(err.Error(), "unknown command "):
		return apperr.Encapsulate(err, errcode.UnknownCommand, strings.TrimSpace(err.Error()))
	}
	return err
}

// loadRepoEnv loads the .env of the repo anfra runs in, the current
// directory. A file it can't read stops the command, as a bad switch does: a
// setting the user wrote must not be silently dropped. A file Git tracks is
// never read, and a warning says so when it sets anfra's or OpenTelemetry's
// names.
func loadRepoEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	loaded, err := repoenv.Load(dir)
	if err != nil {
		output.json = jsonAsked(os.Args[1:])
		printError(os.Stderr, apperr.New(apperr.ValidationFailed, fmt.Sprintf("Can't read the repo's settings: %v.", err)), "")
		os.Exit(1)
	}
	if len(loaded.Ignored) == 0 {
		return
	}
	output.json = jsonAsked(os.Args[1:])
	names := strings.Join(loaded.Ignored, ", ")
	if loaded.Tracked {
		printWarning(os.Stderr, "env_file_tracked", fmt.Sprintf(
			"The repo's %s is tracked by Git, so anfra doesn't read it, and %s don't apply. "+
				"A file everyone clones must not choose where your traces go. "+
				"To use it, untrack it with `git rm --cached %s` and add it to .gitignore.",
			repoenv.FileName, names, repoenv.FileName))
		return
	}
	printWarning(os.Stderr, "env_file_unchecked", fmt.Sprintf(
		"The repo is in a Git work tree, but Git couldn't say whether it tracks %s, so anfra doesn't read it, and %s don't apply. "+
			"Set them in the environment instead, or make git work here: install it, or trust the folder with `git config --global --add safe.directory`.",
		repoenv.FileName, names))
}

// settingRefused is an invalid setting as a violation of the variable that
// holds it, as a refused arg is one of its flag: a switch that isn't 1 or 0, or
// a telemetry setting it doesn't take.
func settingRefused(err error) error {
	var name, msg, code string
	if e, ok := errors.AsType[*envflag.InvalidError](err); ok {
		name, msg, code = e.Name, "Must be 1 or 0, not "+strconv.Quote(e.Value)+".", "invalid"
	} else if e, ok := errors.AsType[*caller.InvalidSettingError](err); ok {
		name, msg, code = e.Name, "Must be "+e.Want+", not "+strconv.Quote(e.Value)+".", "invalid"
	} else {
		return err
	}
	summary := err.Error()
	if line, ok := repoenv.Origin(name); ok {
		summary += ", set in " + repoenv.FileName + " " + line
		msg += " Fix it in " + repoenv.FileName + " " + line + "."
	}
	return apperr.NewWith(apperr.ValidationFailed, summary,
		apperr.Violate(apperr.Violation{Field: name, Code: code, Message: msg}))
}

func newRootCmd() *cobra.Command {
	resultTypes = map[*cobra.Command]reflect.Type{} // this tree's, not the trees' before it
	inputChecks = map[*cobra.Command]func([]string) error{}
	root := &cobra.Command{
		Use:   "anfra",
		Short: "Anfra — local-first agentic analytics framework",
		Long: "Anfra — local-first agentic analytics framework\n\n" +
			"Every command writes its answer to stdout as a YAML document, except CLI-only commands\n" +
			"like agent setup, auth, init and publish, which write it in the pretty format by default\n" +
			"(the lists below tell core and CLI-only commands apart); --yaml or --json writes it as\n" +
			"YAML or JSON from any command. Warnings and errors go to stderr as multiple YAML\n" +
			"documents, or JSONL documents with --json, and the exit code is non-zero on failure.\n" +
			"--non-interactive asks no questions.",
		// Setting Version makes cobra add `--version` (and `-v`, since it's free) to
		// the root. Mirrors the `version` command, which the server also serves.
		Version: meta.Version,
		// We print errors ourselves in main (so exitCodeError stays silent).
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	serve, updateCheck := newServeCmd(), newUpdateCheckCmd()
	markArtifactOutput(serve, "a log")
	markArtifactOutput(updateCheck, "nothing: it checks in the background") // internal
	update, agent, skills, initCmd, publish := newUpdateCmd(), newAgentCmd(), newSkillsCmd(), newInitCmd(), newPublishCmd()
	root.AddCommand(newConsoleCmd())

	root.AddCommand(serve, newOpenAPICmd())
	root.AddCommand(update, updateCheck)
	root.AddCommand(agent, skills, initCmd, newExperimentalCmd())
	root.AddCommand(publish, newPullCmd(), newAuthCmd())
	root.AddCommand(newLogsCmd())
	core := appCommands() // ping, query, … generated from the registry
	root.AddCommand(core...)
	addOutputFlags(root, core)
	return root
}
