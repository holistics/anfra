package main

import (
	"context"
	"fmt"

	"github.com/anfra-ai/anfra/internal/project"
	"github.com/anfra-ai/anfra/internal/sidecar"
	"github.com/spf13/cobra"
)

func newPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Spawn the sidecar and round-trip a JSON-RPC ping",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withSidecar(func(ctx context.Context, node *sidecar.AnfraNodeClient, _ project.Project) error {
				res, err := node.Ping(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("pong from sidecar: %+v\n", res)
				return nil
			})
		},
	}
}
