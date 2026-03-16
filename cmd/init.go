package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/config"
	"github.com/f24aalam/agentsync/internal/scaffold"
	"github.com/spf13/cobra"
)

const (
	aiDirName   = ".ai"
	lockPath    = ".ai/sync.lock"
	gitignoreFn = ".gitignore"
)

type initAnswers struct {
	ProjectName    string
	AddGuidelines  bool
	AddSampleSkill bool
	AddMCPConfig   bool
	Agents         []string
	AddGitignore   bool
}

var (
	runOverwriteConfirm = promptOverwriteConfirm
	runInitSurvey       = promptInitSurvey
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold the .ai directory and sync lockfile",
		RunE:  runInitCommand,
	}
}

func runInitCommand(cmd *cobra.Command, args []string) error {
	existing, err := initTargetsExist()
	if err != nil {
		return err
	}

	if existing {
		overwrite, err := runOverwriteConfirm(cmd)
		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				printInitCancelled(cmd)
				return nil
			}
			return err
		}
		if !overwrite {
			printInitCancelled(cmd)
			return nil
		}
	}

	defaultProjectName, err := defaultProjectName()
	if err != nil {
		return err
	}

	answers, err := runInitSurvey(cmd, defaultProjectName)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			printInitCancelled(cmd)
			return nil
		}
		return err
	}

	if err := scaffold.EnsureDirs(); err != nil {
		return err
	}

	var written []string

	if answers.AddGuidelines {
		path, err := scaffold.CreateGuidelines(answers.ProjectName)
		if err != nil {
			return err
		}
		written = append(written, path)
	}

	if answers.AddSampleSkill {
		path, err := scaffold.CreateSampleSkill()
		if err != nil {
			return err
		}
		written = append(written, path)
	}

	if answers.AddMCPConfig {
		path, err := scaffold.CreateMCPConfig()
		if err != nil {
			return err
		}
		written = append(written, path)
	}

	if err := config.WriteLock(lockPath, answers.Agents); err != nil {
		return err
	}
	written = append(written, lockPath)

	if answers.AddGitignore {
		selectedAgents := selectedAgents(answers.Agents)
		updated, err := scaffold.UpdateGitignore(gitignoreFn, selectedAgents)
		if err != nil {
			return err
		}
		if updated {
			written = append(written, gitignoreFn)
		}
	}

	printInitSummary(cmd, written)
	return nil
}

func promptOverwriteConfirm(cmd *cobra.Command) (bool, error) {
	var overwrite bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(".ai configuration already exists. Overwrite managed files?").
				Description("This updates the files created by agentsync init and keeps unrelated files intact.").
				Affirmative("Overwrite").
				Negative("Cancel").
				Value(&overwrite),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())

	if err := form.Run(); err != nil {
		return false, err
	}

	return overwrite, nil
}

func promptInitSurvey(cmd *cobra.Command, defaultName string) (initAnswers, error) {
	answers := initAnswers{
		ProjectName: defaultName,
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Placeholder("my-project").
				Value(&answers.ProjectName).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("project name is required")
					}
					return nil
				}),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add a core guidelines file?").
				Value(&answers.AddGuidelines),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add a sample skill?").
				Value(&answers.AddSampleSkill),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add MCP configuration?").
				Value(&answers.AddMCPConfig),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select target agents").
				Description("Choose the coding agents that agentsync should manage.").
				Options(agentOptions()...).
				Value(&answers.Agents),
		),
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add generated agent files to .gitignore?").
				Description("Keeps agent-specific output like .cursor/, .codex/, and AGENTS.md out of version control.").
				Value(&answers.AddGitignore),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())

	if err := form.Run(); err != nil {
		return initAnswers{}, err
	}

	answers.ProjectName = strings.TrimSpace(answers.ProjectName)
	return answers, nil
}

func agentOptions() []huh.Option[string] {
	agents := agent.All()
	options := make([]huh.Option[string], 0, len(agents))
	for _, target := range agents {
		options = append(options, huh.NewOption(target.Name, target.ID))
	}
	return options
}

func selectedAgents(ids []string) []agent.Agent {
	selected := make([]agent.Agent, 0, len(ids))
	for _, id := range ids {
		target, ok := agent.ByID(id)
		if ok {
			selected = append(selected, target)
		}
	}
	return selected
}

func initTargetsExist() (bool, error) {
	targets := []string{aiDirName, lockPath}
	for _, target := range targets {
		_, err := os.Stat(target)
		if err == nil {
			return true, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return false, err
		}
	}
	return false, nil
}

func defaultProjectName() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	base := filepath.Base(wd)
	if base == "." || base == string(filepath.Separator) {
		return "project", nil
	}

	return base, nil
}

func printInitCancelled(cmd *cobra.Command) {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Initialization cancelled. No changes were made.")
}

func printInitSummary(cmd *cobra.Command, written []string) {
	headerStyle := lipgloss.NewStyle().Bold(true)
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render("Creating .ai/"))
	for _, path := range written {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s %s\n", checkStyle.Render("✓"), path)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render("Done! Run agentsync install to configure your agents."))
}
