package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/holistics/anfra/internal/meta"
	"github.com/spf13/cobra"
)

// exitCodeError makes a command exit with a specific code without printing an
// error message — the output was already rendered (see present()).
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

func main() {
	// A single signal-cancelable root context, threaded down through cobra so
	// Ctrl-C (SIGINT/SIGTERM) cancels in-flight work — an update download, a
	// query, or the serve loop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ExecuteContextC sets that root context and returns the command that actually
	// ran, so we get the real subcommand name (handles flags/args/aliases).
	executed, err := newRootCmd().ExecuteContextC(ctx)
	// After the command runs, surface a cached "update available" notice (and, if
	// opted in, kick a background update). Best-effort; never affects exit status.
	if executed != nil {
		maybeNotifyUpdate(executed.Name())
	}

	if err == nil {
		return
	}
	var ec *exitCodeError
	if errors.As(err, &ec) {
		os.Exit(ec.code) // result already rendered; exit quietly
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "anfra",
		Short: "Anfra — local-first agentic analytics infrastructure",
		// Setting Version makes cobra add `--version` (and `-v`, since it's free) to
		// the root. Mirrors the `version` subcommand, which also serves it over /call.
		Version: meta.Version,
		// We print errors ourselves in main (so exitCodeError stays silent).
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newServeCmd())
	root.AddCommand(newUpdateCmd(), newUpdateCheckCmd())
	root.AddCommand(appCommands()...) // ping, query, … generated from the registry
	return root
}
