package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/holistics/anfra/internal/meta"
	"github.com/spf13/cobra"
)

// exitCodeError makes a command exit with a specific code without printing an
// error message — the output was already rendered (see present()).
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

func main() {
	err := newRootCmd().Execute()
	// After the command runs, surface a cached "update available" notice (and, if
	// opted in, kick a background update). Best-effort; never affects exit status.
	maybeNotifyUpdate(invokedCommand())

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

// invokedCommand returns the first non-flag CLI arg (the subcommand name), or ""
// for a bare `anfra`. Used to suppress the notice on update/serve commands.
func invokedCommand() string {
	for _, a := range os.Args[1:] {
		if len(a) > 0 && a[0] != '-' {
			return a
		}
	}
	return ""
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
