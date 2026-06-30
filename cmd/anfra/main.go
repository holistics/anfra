package main

import (
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "anfra",
		Short:         "Anfra — local-first agentic analytics infrastructure",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(newPingCmd(), newHoldCmd(), newQueryCmd())
	return root
}
