package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/anfra-ai/anfra/internal/project"
	"github.com/anfra-ai/anfra/internal/sidecar"
	"github.com/spf13/cobra"
)

func newHoldCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hold",
		Short: "Spawn the sidecar and keep it alive until interrupted (orphan-handling test)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withSidecar(func(ctx context.Context, node *sidecar.AnfraNodeClient, _ project.Project) error {
				res, err := node.Ping(ctx)
				if err != nil {
					return err
				}
				fmt.Printf("pong from sidecar: %+v\n", res)
				fmt.Println("holding sidecar; press Ctrl-C to stop")
				sig := make(chan os.Signal, 1)
				signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
				<-sig
				return nil
			})
		},
	}
}
