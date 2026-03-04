package cmd

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "orb",
		Short:         "Kubernetes packaging format transpiler",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newBundleCmd())
	cmd.AddCommand(newCatalogCmd())
	cmd.AddCommand(newHelmPluginCmd())
	return cmd
}

func Execute() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return newRootCmd().ExecuteContext(ctx)
}
