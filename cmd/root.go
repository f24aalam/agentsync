package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type silentError struct {
	err error
}

func (e silentError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func wrapSilentError(err error) error {
	if err == nil {
		return nil
	}
	return silentError{err: err}
}

func IsSilentError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(silentError)
	return ok
}

func printInitRequired(cmd *cobra.Command, subject string) {
	label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Render("Error:")
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s %s not found. Run `agentsync init` first.\n", label, subject)
}

var rootCmd = &cobra.Command{
	Use:           "agentsync",
	Short:         "Sync AI guidelines, skills, and MCP configuration across coding agents",
	SilenceUsage:  true,
	SilenceErrors: true,
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
