package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// exitCodeError makes a command exit with a specific code without printing an
// error message — the output was already rendered (see present()).
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.code) }

func main() {
	err := newRootCmd().Execute()
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
		// We print errors ourselves in main (so exitCodeError stays silent).
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newServeCmd())
	root.AddCommand(appCommands()...) // ping, query, … generated from the registry
	return root
}
