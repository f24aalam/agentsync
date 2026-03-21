package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	agentpkg "github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/config"
	stepflow "github.com/f24aalam/stepflow/pkg"
	"github.com/spf13/cobra"
)

var runAgentRunner = agentpkg.Run

// runInstallStepflow runs stepflow for install conflict resolution (overridable in tests).
var runInstallStepflow = defaultRunInstallStepflow

func defaultRunInstallStepflow(steps []stepflow.Step) (stepflow.Result, error) {
	return stepflow.New().
		WithAltScreen(false).
		WithTheme(stepflow.DefaultTheme()).
		WithSteps(steps...).
		Run()
}

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install synced AI configuration for the selected agents",
		RunE:  runInstallCommand,
	}
	cmd.Flags().BoolP("yes", "y", false, "Overwrite existing agent files without prompting (non-interactive)")
	return cmd
}

func runInstallCommand(cmd *cobra.Command, args []string) error {
	printAgentsyncBanner(cmd.ErrOrStderr())

	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	agentIDs, err := config.ReadLock(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			printInitRequired(cmd, ".ai/sync.lock")
			return wrapSilentError(err)
		}
		return err
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(ThemeGreen)
	okStyle := lipgloss.NewStyle().Foreground(ThemeGreen)
	errStyle := lipgloss.NewStyle().Foreground(ThemeGreen).Bold(true)
	skipStyle := lipgloss.NewStyle().Foreground(ThemeGreenMuted)

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

	plan := agentpkg.NewInstallPlan()
	if !yes {
		conflicts, err := agentpkg.DetectInstallConflicts(validAgents, ".")
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			steps := make([]stepflow.Step, 0, len(conflicts))
			for _, c := range conflicts {
				steps = append(steps, stepflow.Confirm(c.StepKey, c.Question).Default("No"))
			}
			res, err := runInstallStepflow(steps)
			if err != nil {
				if errors.Is(err, stepflow.ErrCancelled) {
					return ErrUserAborted
				}
				return err
			}
			for _, c := range conflicts {
				overwrite := res.Bool(c.StepKey)
				if !overwrite {
					switch c.Kind {
					case agentpkg.KindGuidelines:
						plan.SkipGuidelines[c.AgentID] = true
					case agentpkg.KindMCP:
						plan.SkipMCP[c.AgentID] = true
					case agentpkg.KindSkillsDir:
						plan.SkipSkillsDir[c.SkillsDir] = true
					}
				}
			}
		}
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n", headerStyle.Render(fmt.Sprintf("Installing for %d agents...", len(agentIDs))))

	summary := runAgentRunner(validAgents, plan, ".")
	for _, result := range summary.Results {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), lipgloss.NewStyle().Bold(true).Foreground(ThemeGreen).Render(result.Agent.Name))
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
