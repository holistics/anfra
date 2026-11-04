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

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/spf13/cobra"
)

// exitCodeError makes a command exit with a specific code without printing an
// error message — the output was already rendered (see presentTo()).
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

func main() {
	// anfra's switches are 1 or 0. Refuse any other value before anything reads
	// one, so a typo stops the command instead of silently meaning the opposite.
	if err := envflag.Validate(); err != nil {
		output.json = jsonAsked(os.Args[1:])
		printError(os.Stderr, settingRefused(err), "") // the user's setting, said as it is: not anfra's failure
		os.Exit(1)
	}

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
	// ExecuteContextC sets that root context and returns the command that actually
	// ran, so we get the real subcommand name (handles flags/args/aliases).
	executed, err := root.ExecuteContextC(ctx)
	// After the command runs, surface a cached "update available" notice (and, if
	// opted in, kick a background update). Best-effort; never affects exit status.
	// Not after a failure: the error, or the invalid answer, is what to read, and
	// the notice keeps until a command succeeds.
	if executed != nil && err == nil {
		maybeNotifyUpdate(executed.Name())
		agents.Succeeded() // ends a streak of failures (agents.Failed)
	}
	if err == nil {
		return 0
	}
	var ec *exitCodeError
	if errors.As(err, &ec) {
		return ec.code // result already rendered; exit quietly
	}
	output.json = output.json || jsonAsked(args) // flags that failed to parse set nothing
	err = runError(ctx, err)
	printError(stderr, err, failureHint(err))
	if apperr.From(err).Code == clierr.Interrupted.Code() {
		return 130 // as a shell exits on an interrupt
	}
	return 1
}

// runError is a failed run's err as the CLI tells it: a run the user
// interrupted (Ctrl-C) as interrupted, whatever failed as it stopped; an
// unknown command, cobra's, as unknown_command.
func runError(ctx context.Context, err error) error {
	switch code := apperr.From(err).Code; {
	case code == clierr.Interrupted.Code():
		return err
	case ctx.Err() != nil:
		return apperr.Encapsulate(err, clierr.Interrupted, "Interrupted.")
	case code == apperr.InternalServerError.Code() && strings.HasPrefix(err.Error(), "unknown command "):
		return apperr.Encapsulate(err, errcode.UnknownCommand, strings.TrimSpace(err.Error()))
	}
	return err
}

// settingRefused is an invalid switch as a violation of the variable that holds
// it, as a refused arg is one of its flag.
func settingRefused(err error) error {
	e, ok := errors.AsType[*envflag.InvalidError](err)
	if !ok {
		return err
	}
	msg := "Must be 1 or 0, not " + strconv.Quote(e.Value) + "."
	return apperr.NewWith(apperr.ValidationFailed, e.Error(),
		apperr.Violate(apperr.Violation{Field: e.Name, Code: "invalid", Message: msg}))
}

func newRootCmd() *cobra.Command {
	resultTypes = map[*cobra.Command]reflect.Type{} // this tree's, not the trees' before it
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
	markArtifactOutput(serve, "a log, until it is stopped")
	markArtifactOutput(updateCheck, "nothing: it checks in the background") // internal
	update, agent, skills, initCmd, publish := newUpdateCmd(), newAgentCmd(), newSkillsCmd(), newInitCmd(), newPublishCmd()
	root.AddCommand(newConsoleCmd())

	root.AddCommand(serve, newOpenAPICmd())
	root.AddCommand(update, updateCheck)
	root.AddCommand(agent, skills, initCmd)
	root.AddCommand(publish, newAuthCmd())
	core := appCommands() // ping, query, … generated from the registry
	root.AddCommand(core...)
	addOutputFlags(root, core)
	return root
}
