package cmd

import "github.com/spf13/cobra"

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install synced AI configuration for the selected agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
