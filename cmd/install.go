package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	agentpkg "github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/config"
	"github.com/spf13/cobra"
)

var runAgentInstall = agentpkg.Install

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install synced AI configuration for the selected agents",
		RunE:  runInstallCommand,
	}
}

func runInstallCommand(cmd *cobra.Command, args []string) error {
	agentIDs, err := config.ReadLock(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "No .ai/sync.lock found. Run `agentsync init` first.")
			return err
		}
		return err
	}

	headerStyle := lipgloss.NewStyle().Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	skipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n", headerStyle.Render(fmt.Sprintf("Installing for %d agents...", len(agentIDs))))

	configuredCount := 0

	for _, id := range agentIDs {
		target, ok := agentpkg.ByID(id)
		if !ok {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), lipgloss.NewStyle().Bold(true).Render(id))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s %s\n\n", errStyle.Render("✗"), "Unknown agent in .ai/sync.lock")
			continue
		}

		result := runAgentInstall(target)
		if result.Succeeded() {
			configuredCount++
		}

		_, _ = fmt.Fprintln(cmd.OutOrStdout(), lipgloss.NewStyle().Bold(true).Render(target.Name))
		for _, step := range result.Steps {
			icon := okStyle.Render("✓")
			if step.Status == agentpkg.StepStatusSkipped {
				icon = skipStyle.Render("-")
			} else if step.Status == agentpkg.StepStatusError {
				icon = errStyle.Render("✗")
			}

			line := fmt.Sprintf("  %s %-10s → %s", icon, step.Name, step.Target)
			if step.Status == agentpkg.StepStatusError && step.Err != nil {
				line = fmt.Sprintf("%s (%v)", line, step.Err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", headerStyle.Render(fmt.Sprintf("Done! %d agents configured.", configuredCount)))
	return nil
}
