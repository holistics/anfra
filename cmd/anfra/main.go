package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/holistics/anfra/internal/envflag"

	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/spf13/cobra"
)

// exitCodeError makes a command exit with a specific code without printing an
// error message — the output was already rendered (see present()).
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

func main() {
	// anfra's switches are 1 or 0. Refuse any other value before anything reads
	// one, so a typo stops the command instead of silently meaning the opposite.
	if err := envflag.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err) // the user's setting, said as it is: not anfra's failure
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
	defer stop()

	// Traces, when an OTEL_* endpoint asks for them; flushed before any exit below.
	flush := startTelemetry(ctx)

	// ExecuteContextC sets that root context and returns the command that actually
	// ran, so we get the real subcommand name (handles flags/args/aliases).
	executed, err := newRootCmd().ExecuteContextC(ctx)
	// After the command runs, surface a cached "update available" notice (and, if
	// opted in, kick a background update). Best-effort; never affects exit status.
	// Not after a failure: the error, or the invalid answer, is what to read, and
	// the notice keeps until a command succeeds.
	if executed != nil && err == nil {
		maybeNotifyUpdate(executed.Name())
	}

	flush()
	if err == nil {
		return
	}
	var ec *exitCodeError
	if errors.As(err, &ec) {
		os.Exit(ec.code) // result already rendered; exit quietly
	}
	printError(os.Stderr, err)
	os.Exit(1)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "anfra",
		Short: "Anfra — local-first agentic analytics infrastructure",
		// Setting Version makes cobra add `--version` (and `-v`, since it's free) to
		// the root. Mirrors the `version` command, which the server also serves.
		Version: meta.Version,
		// We print errors ourselves in main (so exitCodeError stays silent).
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newServeCmd(), newOpenAPICmd())
	root.AddCommand(newUpdateCmd(), newUpdateCheckCmd())
	root.AddCommand(newSkillsCmd(), newInitCmd())
	root.AddCommand(appCommands()...) // ping, query, … generated from the registry
	return root
}
