package cmd

import "github.com/spf13/cobra"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "orb",
		Short:         "Kubernetes packaging format transpiler",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newBundleCmd())
	cmd.AddCommand(newCatalogCmd())
	return cmd
}

func Execute() error {
	return newRootCmd().Execute()
}
