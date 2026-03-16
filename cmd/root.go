package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "agentsync",
	Short: "Sync AI guidelines, skills, and MCP configuration across coding agents",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(
		newInitCmd(),
		newInstallCmd(),
		newUpdateCmd(),
		newListCmd(),
	)
}
