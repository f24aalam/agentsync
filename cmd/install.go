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
		Use:           "install",
		Short:         "Install synced AI configuration for the selected agents",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runInstallCommand,
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

	lock, err := config.ReadFullLock(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			printInitRequired(cmd, ".ai/sync.lock")
			return wrapSilentError(err)
		}
		return err
	}
	agentIDs := lock.Agents

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
	plan.Lock = lock

	conflicts, err := agentpkg.DetectInstallConflicts(validAgents, ".")
	if err != nil {
		return err
	}

	if yes {
		for _, c := range conflicts {
			switch c.Kind {
			case agentpkg.KindSkillPackage:
				if !c.IsManaged {
					pkgKey := c.SkillsDir + "/" + c.SkillName
					plan.SkipSkillPackages[pkgKey] = true
				}
			case agentpkg.KindMCP:
				if !c.IsManaged {
					if c.DestPath != "" {
						plan.SkipMCPDest[c.DestPath] = true
					} else {
						plan.SkipMCP[c.AgentID] = true
					}
				}
			}
		}
	} else if len(conflicts) > 0 {
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
					if c.DestPath != "" {
						plan.SkipGuidelinesDest[c.DestPath] = true
					}
					for _, id := range c.AgentIDs {
						plan.SkipGuidelines[id] = true
					}
				case agentpkg.KindSkillPackage:
					pkgKey := c.SkillsDir + "/" + c.SkillName
					plan.SkipSkillPackages[pkgKey] = true
				case agentpkg.KindSkillsDir:
					plan.SkipSkillsDir[c.SkillsDir] = true
				case agentpkg.KindMCP:
					if c.DestPath != "" {
						plan.SkipMCPDest[c.DestPath] = true
					} else {
						plan.SkipMCP[c.AgentID] = true
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

	failedCount := 0
	for _, result := range summary.Results {
		if !result.Succeeded() {
			failedCount++
		}
	}

	if err := config.WriteFullLock(lockPath, plan.Lock); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error writing %s: %v\n", lockPath, err)
		return fmt.Errorf("failed to write %s: %w", lockPath, err)
	}

	if failedCount > 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("%d configured, %d failed.", summary.ConfiguredCount, failedCount)))
		return fmt.Errorf("%d agent(s) failed to configure", failedCount)
	}

	doneLabel := "agents"
	if summary.ConfiguredCount == 1 {
		doneLabel = "agents"
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", headerStyle.Render(fmt.Sprintf("Done! %d %s configured.", summary.ConfiguredCount, doneLabel)))
	return nil
}
