package cmd

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold the .ai directory and sync lockfile",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
