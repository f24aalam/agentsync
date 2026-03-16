package cmd

import "github.com/spf13/cobra"

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Re-apply synced AI configuration after project changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
