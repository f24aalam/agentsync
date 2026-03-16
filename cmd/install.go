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

var runAgentRunner = agentpkg.Run

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install synced AI configuration for the selected agents",
		RunE:  runInstallCommand,
	}
}

func runInstallCommand(cmd *cobra.Command, args []string) error {
	return runAgentSyncCommand(cmd, "install")
}

func runUpdateCommand(cmd *cobra.Command, args []string) error {
	return runAgentSyncCommand(cmd, "update")
}

func runAgentSyncCommand(cmd *cobra.Command, mode string) error {
	agentIDs, err := config.ReadLock(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			printInitRequired(cmd, ".ai/sync.lock")
			return wrapSilentError(err)
		}
		return err
	}

	headerStyle := lipgloss.NewStyle().Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	skipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	intro := fmt.Sprintf("Installing for %d agents...", len(agentIDs))
	if mode == "update" {
		intro = fmt.Sprintf("Updating %d agents...", len(agentIDs))
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n", headerStyle.Render(intro))

	validAgents := make([]agentpkg.Agent, 0, len(agentIDs))
	unknownAgents := make([]string, 0)
	for _, id := range agentIDs {
		target, ok := agentpkg.ByID(id)
		if !ok {
			unknownAgents = append(unknownAgents, id)
			continue
		}
		validAgents = append(validAgents, target)
	}

	for _, id := range unknownAgents {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), lipgloss.NewStyle().Bold(true).Render(id))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s %s\n\n", errStyle.Render("✗"), "Unknown agent in .ai/sync.lock")
	}

	summary := runAgentRunner(validAgents, mode)
	for _, result := range summary.Results {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), lipgloss.NewStyle().Bold(true).Render(result.Agent.Name))
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

	doneLabel := "agents"
	if summary.ConfiguredCount == 1 {
		doneLabel = "agents"
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", headerStyle.Render(fmt.Sprintf("Done! %d %s configured.", summary.ConfiguredCount, doneLabel)))
	return nil
}
