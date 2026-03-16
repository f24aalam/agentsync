package cmd

import "github.com/spf13/cobra"

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured guidelines, skills, MCP servers, and target agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
