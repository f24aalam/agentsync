package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/config"
	"github.com/f24aalam/agentsync/internal/detect"
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

type importPlan struct {
	Guidelines []importGuideline
	Skills     []importSkill
	MCPServers map[string]config.MCPServer
}

type importGuideline struct {
	Label   string
	Content string
}

type importSkill struct {
	Name      string
	SourceDir string
}

type importSummaryItem struct {
	Label string
	Value string
}

var (
	runOverwriteConfirm  = promptOverwriteConfirm
	runProjectNamePrompt = promptProjectName
	runInitSurvey        = promptInitSurvey
	runImportFlow        = runImportPrompts
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

	projectName, err := runProjectNamePrompt(cmd, defaultProjectName)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			printInitCancelled(cmd)
			return nil
		}
		return err
	}

	var written []string

	detected, err := detect.ScanProject(".")
	if err != nil {
		return err
	}

	imports, err := runImportFlow(cmd, detected)
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

	importedItems, importedMCP, err := applyImports(imports)
	if err != nil {
		return err
	}

	askGuidelines := len(imports.Guidelines) == 0
	askSampleSkill := len(imports.Skills) == 0
	askMCP := !importedMCP

	answers, err := runInitSurvey(cmd, projectName, askGuidelines, askSampleSkill, askMCP)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			printInitCancelled(cmd)
			return nil
		}
		return err
	}

	answers.ProjectName = projectName

	if answers.AddGuidelines && askGuidelines {
		path, err := scaffold.CreateGuidelines(answers.ProjectName)
		if err != nil {
			return err
		}
		written = append(written, path)
	}

	if answers.AddSampleSkill && askSampleSkill {
		path, err := scaffold.CreateSampleSkill()
		if err != nil {
			return err
		}
		written = append(written, path)
	}

	if answers.AddMCPConfig && askMCP {
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

	if len(importedItems) > 0 {
		printImportSummary(cmd, importedItems)
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

func promptProjectName(cmd *cobra.Command, defaultName string) (string, error) {
	name := defaultName
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Project name").
				Placeholder("my-project").
				Value(&name).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return errors.New("project name is required")
					}
					return nil
				}),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())

	if err := form.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(name), nil
}

func promptInitSurvey(cmd *cobra.Command, projectName string, askGuidelines bool, askSampleSkill bool, askMCP bool) (initAnswers, error) {
	answers := initAnswers{
		ProjectName: projectName,
	}

	groups := make([]*huh.Group, 0, 5)
	if askGuidelines {
		groups = append(groups, huh.NewGroup(
			huh.NewConfirm().
				Title("Add a core guidelines file?").
				Value(&answers.AddGuidelines),
		))
	}
	if askSampleSkill {
		groups = append(groups, huh.NewGroup(
			huh.NewConfirm().
				Title("Add a sample skill?").
				Value(&answers.AddSampleSkill),
		))
	}
	if askMCP {
		groups = append(groups, huh.NewGroup(
			huh.NewConfirm().
				Title("Add MCP configuration?").
				Value(&answers.AddMCPConfig),
		))
	}
	groups = append(groups,
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
	)

	form := huh.NewForm(groups...).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())

	if err := form.Run(); err != nil {
		return initAnswers{}, err
	}

	answers.ProjectName = strings.TrimSpace(answers.ProjectName)
	return answers, nil
}

func runImportPrompts(cmd *cobra.Command, detection detect.ProjectDetection) (importPlan, error) {
	if detection.Empty() {
		return importPlan{}, nil
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), detect.Overview(detection))

	groups := detect.DedupGuidelines(detection.Guidelines)
	selectedGuidelines, err := selectGuidelines(cmd, groups)
	if err != nil {
		return importPlan{}, err
	}

	skills := detect.DedupSkills(detection.Skills)
	selectedSkills, err := selectSkills(cmd, skills)
	if err != nil {
		return importPlan{}, err
	}

	mcpGroups := detect.DedupMCP(detection.MCPServers)
	selectedServers, err := selectMCPServers(cmd, mcpGroups)
	if err != nil {
		return importPlan{}, err
	}

	return importPlan{
		Guidelines: selectedGuidelines,
		Skills:     selectedSkills,
		MCPServers: selectedServers,
	}, nil
}

func selectGuidelines(cmd *cobra.Command, groups []detect.GuidelineGroup) ([]importGuideline, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	options := make([]huh.Option[string], 0, len(groups))
	indexByID := make(map[string]detect.GuidelineGroup)
	for i, group := range groups {
		id := fmt.Sprintf("g-%d", i)
		label := guidelineGroupLabel(group)
		options = append(options, huh.NewOption(label, id))
		indexByID[id] = group
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Existing agent configs detected. Select guidelines to import into core.md").
				Options(options...).
				Value(&selected),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())

	if err := form.Run(); err != nil {
		return nil, err
	}

	out := make([]importGuideline, 0, len(selected))
	for _, id := range selected {
		group, ok := indexByID[id]
		if !ok {
			continue
		}
		out = append(out, importGuideline{
			Label:   guidelineGroupLabel(group),
			Content: group.Content,
		})
	}
	return out, nil
}

func selectSkills(cmd *cobra.Command, groups []detect.SkillGroup) ([]importSkill, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	options := make([]huh.Option[string], 0, len(groups))
	groupByID := make(map[string]detect.SkillGroup)
	for i, group := range groups {
		id := fmt.Sprintf("s-%d", i)
		label := group.Name
		if group.Conflict {
			label += "  (conflict)"
		}
		options = append(options, huh.NewOption(label, id))
		groupByID[id] = group
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select skills to import").
				Options(options...).
				Value(&selected),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())

	if err := form.Run(); err != nil {
		return nil, err
	}

	out := make([]importSkill, 0, len(selected))
	for _, id := range selected {
		group, ok := groupByID[id]
		if !ok {
			continue
		}

		var chosen detect.DetectedSkill
		if group.Conflict {
			var err error
			chosen, err = selectSkillVariant(cmd, group)
			if err != nil {
				return nil, err
			}
		} else if len(group.Variants) > 0 {
			chosen = group.Variants[0]
		}

		if chosen.Name == "" {
			continue
		}
		out = append(out, importSkill{
			Name:      chosen.Name,
			SourceDir: chosen.Path,
		})
	}

	return out, nil
}

func selectSkillVariant(cmd *cobra.Command, group detect.SkillGroup) (detect.DetectedSkill, error) {
	options := make([]huh.Option[string], 0, len(group.Variants))
	variantsByID := make(map[string]detect.DetectedSkill)
	for i, variant := range group.Variants {
		id := fmt.Sprintf("v-%d", i)
		label := fmt.Sprintf("%s (%s)", variant.Agent.Name, variant.Path)
		options = append(options, huh.NewOption(label, id))
		variantsByID[id] = variant
	}

	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Select a version for skill %q", group.Name)).
				Options(options...).
				Value(&choice),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())

	if err := form.Run(); err != nil {
		return detect.DetectedSkill{}, err
	}

	return variantsByID[choice], nil
}

func selectMCPServers(cmd *cobra.Command, groups []detect.MCPGroup) (map[string]config.MCPServer, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	out := make(map[string]config.MCPServer)
	for _, group := range groups {
		var chosen detect.DetectedMCPServer
		if group.Conflict {
			var err error
			chosen, err = selectMCPVariant(cmd, group)
			if err != nil {
				return nil, err
			}
		} else if len(group.Variants) > 0 {
			chosen = group.Variants[0]
		}

		if chosen.Name != "" {
			out[chosen.Name] = chosen.Server
		}
	}
	return out, nil
}

func selectMCPVariant(cmd *cobra.Command, group detect.MCPGroup) (detect.DetectedMCPServer, error) {
	options := make([]huh.Option[string], 0, len(group.Variants))
	variantsByID := make(map[string]detect.DetectedMCPServer)
	for i, variant := range group.Variants {
		id := fmt.Sprintf("m-%d", i)
		label := fmt.Sprintf("%s (command=%s)", variant.Agent.Name, variant.Server.Command)
		options = append(options, huh.NewOption(label, id))
		variantsByID[id] = variant
	}

	var choice string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Select a version for MCP server %q", group.Name)).
				Options(options...).
				Value(&choice),
		),
	).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())

	if err := form.Run(); err != nil {
		return detect.DetectedMCPServer{}, err
	}

	return variantsByID[choice], nil
}

func guidelineGroupLabel(group detect.GuidelineGroup) string {
	if len(group.Sources) == 0 {
		return "unknown"
	}

	files := make([]string, 0, len(group.Sources))
	agents := make([]string, 0, len(group.Sources))
	for _, source := range group.Sources {
		files = append(files, filepath.Base(source.Path))
		agents = append(agents, source.Agent.Name)
	}
	files = uniqueStrings(files)
	agents = uniqueStrings(agents)
	return fmt.Sprintf("%s (%s)", strings.Join(files, ", "), strings.Join(agents, ", "))
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	slices.Sort(out)
	return out
}

func applyImports(plan importPlan) ([]importSummaryItem, bool, error) {
	if len(plan.Guidelines) == 0 && len(plan.Skills) == 0 && len(plan.MCPServers) == 0 {
		return nil, false, nil
	}

	var imported []importSummaryItem
	for _, guideline := range plan.Guidelines {
		path, err := scaffold.ImportGuidelineToCore(guideline.Label, guideline.Content)
		if err != nil {
			return nil, false, err
		}
		imported = append(imported, importSummaryItem{
			Label: "Guidelines",
			Value: path,
		})
	}

	for _, skill := range plan.Skills {
		_, err := scaffold.ImportSkill(skill.Name, skill.SourceDir)
		if err != nil {
			return nil, false, err
		}
		imported = append(imported, importSummaryItem{
			Label: "Skill",
			Value: fmt.Sprintf(".ai/skills/%s/", skill.Name),
		})
	}

	importedMCP := false
	if len(plan.MCPServers) > 0 {
		path, err := scaffold.ImportMCPServers(plan.MCPServers)
		if err != nil {
			return nil, false, err
		}
		if path != "" {
			importedMCP = true
			names := make([]string, 0, len(plan.MCPServers))
			for name := range plan.MCPServers {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				imported = append(imported, importSummaryItem{
					Label: "MCP Server",
					Value: name,
				})
			}
		}
	}

	return imported, importedMCP, nil
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

func printImportSummary(cmd *cobra.Command, items []importSummaryItem) {
	headerStyle := lipgloss.NewStyle().Bold(true)
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render("Importing existing configs..."))
	for _, item := range items {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s %-10s → %s\n", checkStyle.Render("✓"), item.Label, item.Value)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
}
