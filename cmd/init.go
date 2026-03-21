package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/config"
	"github.com/f24aalam/agentsync/internal/detect"
	"github.com/f24aalam/agentsync/internal/scaffold"
	stepflow "github.com/f24aalam/stepflow/pkg"
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

// wizardStored answers are produced by the stepflow-based UI inside
// promptProjectName (so the whole init experience is a single wizard).
var (
	wizardHasStoredResults bool
	wizardStoredImports    importPlan
	wizardStoredAnswers    initAnswers
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Scaffold the .ai directory and sync lockfile",
		RunE:  runInitCommand,
	}
}

func runInitCommand(cmd *cobra.Command, args []string) error {
	// Banner on stderr so it renders before the stepflow TUI (which uses stderr);
	// stdout may stay buffered until exit, which made the banner appear only after quit.
	printAgentsyncBanner(cmd.ErrOrStderr())

	existing, err := initTargetsExist()
	if err != nil {
		return err
	}

	if existing {
		overwrite, err := runOverwriteConfirm(cmd)
		if err != nil {
			if errors.Is(err, ErrUserAborted) {
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

	wizardHasStoredResults = false
	wizardStoredImports = importPlan{}
	wizardStoredAnswers = initAnswers{}

	projectName, err := runProjectNamePrompt(cmd, defaultProjectName)
	if err != nil {
		if errors.Is(err, ErrUserAborted) {
			printInitCancelled(cmd)
			return nil
		}
		return err
	}

	var written []string

	// The stepflow UI (inside runProjectNamePrompt) already scanned and
	// collected imports. We keep the call here to preserve override points
	// used by tests.
	imports, err := runImportFlow(cmd, detect.ProjectDetection{})
	if err != nil {
		if errors.Is(err, ErrUserAborted) {
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
		if errors.Is(err, ErrUserAborted) {
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

	// Gemini CLI (and potentially other agents) may not support native skills.
	// In that case, never scaffold the sample skill even if the UI toggled it.
	selectedTargets := selectedAgents(answers.Agents)
	anyTargetSupportsSkills := false
	for _, t := range selectedTargets {
		if t.SkillsSupported {
			anyTargetSupportsSkills = true
			break
		}
	}

	if answers.AddSampleSkill && askSampleSkill && anyTargetSupportsSkills {
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
	// stepflow is a terminal UI; it doesn't support injecting Cobra's IO
	// streams. We rely on the default TTY wiring.
	res, err := stepflow.New().
		WithAltScreen(false).
		WithTheme(stepflow.DefaultTheme()).
		WithSteps(
			stepflow.Confirm(
				"overwrite",
				".ai configuration already exists. Overwrite managed files?",
			).Default("No"),
		).
		Run()
	if err != nil {
		if errors.Is(err, stepflow.ErrCancelled) {
			return false, ErrUserAborted
		}
		return false, err
	}

	return res.Bool("overwrite"), nil
}

func promptProjectName(cmd *cobra.Command, defaultName string) (string, error) {
	_ = cmd

	// Spinner scan step that dynamically decides which import-selection steps to show.
	scanStep := &initScanStep{}
	scanStep.LoadStep = stepflow.Load("scan", "Scanning existing agent configs").
		Run(func(status chan<- string) (string, error) {
			status <- "looking for guidelines, skills, and MCP config…"
			detection, err := detect.ScanProject(".")
			if err != nil {
				return "", err
			}
			scanStep.detection = detection
			summary := fmt.Sprintf(
				"found %d guidelines, %d skills, %d MCP",
				len(detection.Guidelines),
				len(detection.Skills),
				len(detection.MCPServers),
			)
			return summary, nil
		})

	res, err := stepflow.New().
		WithAltScreen(false).
		WithTheme(stepflow.DefaultTheme()).
		WithSteps(
			stepflow.Text(wizardKeyProjectName, "Project name").
				Placeholder("my-project").
				Default(defaultName),
			scanStep,
		).
		Run()
	if err != nil {
		if errors.Is(err, stepflow.ErrCancelled) {
			return "", ErrUserAborted
		}
		return "", err
	}

	projectName := strings.TrimSpace(res.Get(wizardKeyProjectName))

	// Build import plan from wizard answers.
	wizardStoredImports = importPlan{
		Guidelines: nil,
		Skills:     nil,
		MCPServers: nil,
	}

	selectedGuidelines := wizardSplitMultiAnswer(res.Get(wizardKeyImportGuidelines))
	for _, label := range selectedGuidelines {
		idx, ok := scanStep.guidelineLabelToIdx[label]
		if !ok {
			continue
		}
		group := scanStep.guidelineGroups[idx]
		wizardStoredImports.Guidelines = append(wizardStoredImports.Guidelines, importGuideline{
			Label:   guidelineGroupLabel(group),
			Content: group.Content,
		})
	}

	selectedSkills := wizardSplitMultiAnswer(res.Get(wizardKeyImportSkills))
	for _, label := range selectedSkills {
		idx, ok := scanStep.skillLabelToIdx[label]
		if !ok {
			continue
		}
		group := scanStep.skillGroups[idx]

		var chosen detect.DetectedSkill
		if group.Conflict {
			variantKey := wizardSkillVariantStepKey(group.Name)
			chosenLabel := res.Get(variantKey)
			for _, v := range group.Variants {
				if wizardSkillVariantOptionLabel(v) == chosenLabel {
					chosen = v
					break
				}
			}
		} else if len(group.Variants) > 0 {
			chosen = group.Variants[0]
		}

		if chosen.Name == "" {
			continue
		}

		wizardStoredImports.Skills = append(wizardStoredImports.Skills, importSkill{
			Name:      chosen.Name,
			SourceDir: chosen.Path,
		})
	}

	selectedMCP := wizardSplitMultiAnswer(res.Get(wizardKeyImportMCP))
	for _, label := range selectedMCP {
		idx, ok := scanStep.mcpLabelToIdx[label]
		if !ok {
			continue
		}
		group := scanStep.mcpGroups[idx]

		var chosen detect.DetectedMCPServer
		if group.Conflict {
			variantKey := wizardMCPVariantStepKey(group.Name)
			chosenLabel := res.Get(variantKey)
			for _, v := range group.Variants {
				if wizardMCPVariantOptionLabel(v) == chosenLabel {
					chosen = v
					break
				}
			}
		} else if len(group.Variants) > 0 {
			chosen = group.Variants[0]
		}

		if chosen.Name == "" {
			continue
		}
		if wizardStoredImports.MCPServers == nil {
			wizardStoredImports.MCPServers = map[string]config.MCPServer{}
		}
		wizardStoredImports.MCPServers[chosen.Name] = chosen.Server
	}

	// Build init survey answers.
	agents := wizardSplitMultiAnswer(res.Get(wizardKeyAgents))
	wizardStoredAnswers = initAnswers{
		ProjectName: projectName,

		AddGuidelines:  res.Bool(wizardKeyAddCoreGuidelines),
		AddSampleSkill: res.Bool(wizardKeyAddSampleSkill),
		AddMCPConfig:   res.Bool(wizardKeyAddMCPConfig),
		Agents:         agents,
		AddGitignore:   res.Bool(wizardKeyAddGitignore),
	}

	wizardHasStoredResults = true
	return projectName, nil
}

func promptInitSurvey(cmd *cobra.Command, projectName string, askGuidelines bool, askSampleSkill bool, askMCP bool) (initAnswers, error) {
	_ = cmd
	_ = projectName
	_ = askGuidelines
	_ = askSampleSkill
	_ = askMCP

	if !wizardHasStoredResults {
		return initAnswers{}, errors.New("agentsync init: internal wizard state missing")
	}
	// runInitCommand will set answers.ProjectName again after this returns.
	return wizardStoredAnswers, nil
}

func runImportPrompts(cmd *cobra.Command, detection detect.ProjectDetection) (importPlan, error) {
	_ = cmd
	_ = detection

	if !wizardHasStoredResults {
		return importPlan{}, errors.New("agentsync init: internal wizard state missing")
	}
	return wizardStoredImports, nil
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
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(ThemeGreen)
	checkStyle := lipgloss.NewStyle().Foreground(ThemeGreen)

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render("Creating .ai/"))
	for _, path := range written {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s %s\n", checkStyle.Render("✓"), path)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render("Done! Run agentsync install to configure your agents."))
}

const (
	wizardKeyProjectName      = "project_name"
	wizardKeyImportGuidelines = "import_guidelines"
	wizardKeyImportSkills     = "import_skills"
	wizardKeyImportMCP        = "import_mcp"

	wizardKeyAddCoreGuidelines = "add_core_guidelines"
	wizardKeyAddSampleSkill    = "add_sample_skill"
	wizardKeyAddMCPConfig      = "add_mcp_config"
	wizardKeyAgents            = "agents"
	wizardKeyAddGitignore      = "add_gitignore"

	wizardSkillVariantPrefix = "skill_variant_"
	wizardMCPVariantPrefix   = "mcp_variant_"
)

func wizardSplitMultiAnswer(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, ", ")
}

func wizardSanitizeKey(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

func wizardNoComma(s string) string {
	return strings.ReplaceAll(s, ",", ";")
}

func wizardGuidelineOptionLabel(idx int, g detect.GuidelineGroup) string {
	files := make([]string, 0, len(g.Sources))
	agents := make([]string, 0, len(g.Sources))
	for _, source := range g.Sources {
		files = append(files, filepath.Base(source.Path))
		agents = append(agents, source.Agent.Name)
	}
	files = uniqueStrings(files)
	agents = uniqueStrings(agents)
	// Avoid commas so ListStep's answer join (", ") stays unambiguous.
	lbl := fmt.Sprintf("g%d: %s [%s]", idx, strings.Join(files, " + "), strings.Join(agents, " + "))
	return wizardNoComma(lbl)
}

func wizardSkillGroupOptionLabel(g detect.SkillGroup) string {
	lbl := g.Name
	if g.Conflict {
		lbl += " (conflict)"
	}
	return wizardNoComma(lbl)
}

func wizardMCPGroupOptionLabel(g detect.MCPGroup) string {
	lbl := g.Name
	if g.Conflict {
		lbl += " (conflict)"
	}
	return wizardNoComma(lbl)
}

func wizardSkillVariantOptionLabel(v detect.DetectedSkill) string {
	base := filepath.Base(v.Path)
	base = wizardNoComma(base)
	return wizardNoComma(v.Agent.ID) + "|" + base
}

func wizardMCPVariantOptionLabel(v detect.DetectedMCPServer) string {
	typ := strings.TrimSpace(v.Server.Type)
	if typ == "" {
		typ = "local"
	}

	args := strings.Join(v.Server.Args, ";")
	args = wizardNoComma(args)

	var envPairs string
	if len(v.Server.Env) > 0 {
		keys := make([]string, 0, len(v.Server.Env))
		for k := range v.Server.Env {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, wizardNoComma(k)+"="+wizardNoComma(v.Server.Env[k]))
		}
		envPairs = strings.Join(parts, ";")
	}

	var headerPairs string
	if len(v.Server.Headers) > 0 {
		keys := make([]string, 0, len(v.Server.Headers))
		for k := range v.Server.Headers {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, wizardNoComma(k)+"="+wizardNoComma(v.Server.Headers[k]))
		}
		headerPairs = strings.Join(parts, ";")
	}

	var oauthStr string
	if v.Server.OAuth != nil {
		oauthStr = "cid=" + wizardNoComma(v.Server.OAuth.ClientID) +
			";cs=" + wizardNoComma(v.Server.OAuth.ClientSecret) +
			";scope=" + wizardNoComma(v.Server.OAuth.Scope)
	}

	cmd := wizardNoComma(strings.TrimSpace(v.Server.Command))
	url := wizardNoComma(strings.TrimSpace(v.Server.URL))

	// Compose a comma-free label so stepflow's List answer joining stays unambiguous.
	return wizardNoComma(v.Agent.ID) + "|" + wizardNoComma(v.Name) +
		"|type=" + wizardNoComma(typ) +
		"|url=" + url +
		"|cmd=" + cmd +
		"|args=" + args +
		"|env=" + wizardNoComma(envPairs) +
		"|headers=" + wizardNoComma(headerPairs) +
		"|oauth=" + wizardNoComma(oauthStr)
}

func wizardSkillVariantStepKey(skillName string) string {
	return wizardSkillVariantPrefix + wizardSanitizeKey(skillName)
}

func wizardMCPVariantStepKey(mcpName string) string {
	return wizardMCPVariantPrefix + wizardSanitizeKey(mcpName)
}

func wizardAgentItems() []stepflow.ListItem {
	agents := agent.All()
	items := make([]stepflow.ListItem, 0, len(agents))
	for _, a := range agents {
		items = append(items, stepflow.Item(a.ID))
	}
	return items
}

func wizardMakeSurveySteps(askGuidelines bool, askSampleSkill bool, askMCP bool) []stepflow.Step {
	steps := make([]stepflow.Step, 0, 5)

	if askGuidelines {
		steps = append(steps,
			stepflow.Confirm(wizardKeyAddCoreGuidelines, "Add a core guidelines file?").Default("No"),
		)
	}
	if askSampleSkill {
		steps = append(steps,
			stepflow.Confirm(wizardKeyAddSampleSkill, "Add a sample skill?").Default("No"),
		)
	}
	if askMCP {
		steps = append(steps,
			stepflow.Confirm(wizardKeyAddMCPConfig, "Add MCP configuration?").Default("No"),
		)
	}

	steps = append(steps,
		stepflow.List(wizardKeyAgents, "Select target agents").
			Items(wizardAgentItems()...).
			MultiSelect(true).
			VisibleRows(8),
		stepflow.Confirm(wizardKeyAddGitignore, "Add generated agent files to .gitignore?").Default("No"),
	)

	return steps
}

type initScanStep struct {
	*stepflow.LoadStep

	detection detect.ProjectDetection

	guidelineGroups     []detect.GuidelineGroup
	skillGroups         []detect.SkillGroup
	mcpGroups           []detect.MCPGroup
	guidelineLabelToIdx map[string]int
	skillLabelToIdx     map[string]int
	mcpLabelToIdx       map[string]int
}

func (s *initScanStep) NextSteps(_ stepflow.Result) []stepflow.Step {
	s.guidelineGroups = detect.DedupGuidelines(s.detection.Guidelines)
	s.skillGroups = detect.DedupSkills(s.detection.Skills)
	s.mcpGroups = detect.DedupMCP(s.detection.MCPServers)

	// Default to asking for sample files when there's nothing detected.
	if len(s.guidelineGroups) == 0 && len(s.skillGroups) == 0 && len(s.mcpGroups) == 0 {
		return wizardMakeSurveySteps(true, true, true)
	}

	s.guidelineLabelToIdx = map[string]int{}
	s.skillLabelToIdx = map[string]int{}
	s.mcpLabelToIdx = map[string]int{}

	hasGuidelines := len(s.guidelineGroups) > 0
	hasSkills := len(s.skillGroups) > 0
	hasMCP := len(s.mcpGroups) > 0

	steps := make([]stepflow.Step, 0, 4)

	// Guidelines selection.
	if hasGuidelines {
		gItems := make([]stepflow.ListItem, 0, len(s.guidelineGroups))
		for idx, g := range s.guidelineGroups {
			lbl := wizardGuidelineOptionLabel(idx, g)
			s.guidelineLabelToIdx[lbl] = idx
			gItems = append(gItems, stepflow.Item(lbl))
		}

		gStep := stepflow.List(
			wizardKeyImportGuidelines,
			"Existing agent configs detected. Select guidelines to import into core.md",
		).Items(gItems...).MultiSelect(true).VisibleRows(8)

		// If guidelines are the only detected import category, we need a dynamic
		// step to decide whether to ask for core guidelines creation.
		if !hasSkills && !hasMCP {
			steps = append(steps, &dynamicGuidelinesSelectionStep{ListStep: gStep})
		} else {
			steps = append(steps, gStep)
		}
	}

	// Skills selection (dynamic; can insert per-skill variants and/or survey).
	if hasSkills {
		sItems := make([]stepflow.ListItem, 0, len(s.skillGroups))
		for idx, g := range s.skillGroups {
			lbl := wizardSkillGroupOptionLabel(g)
			s.skillLabelToIdx[lbl] = idx
			sItems = append(sItems, stepflow.Item(lbl))
		}

		// Even if we return early after adding the skill selection step, the later
		// MCP conflict resolver relies on mcpLabelToIdx being populated.
		if hasMCP {
			for idx, g := range s.mcpGroups {
				lbl := wizardMCPGroupOptionLabel(g)
				s.mcpLabelToIdx[lbl] = idx
			}
		}

		sList := stepflow.List(wizardKeyImportSkills, "Select skills to import").
			Items(sItems...).
			MultiSelect(true).
			VisibleRows(8)

		steps = append(steps, &dynamicSkillSelectionStep{
			ListStep:        sList,
			skillGroups:     s.skillGroups,
			skillLabelToIdx: s.skillLabelToIdx,
			mcpGroups:       s.mcpGroups,
			mcpLabelToIdx:   s.mcpLabelToIdx,
			hasMCP:          hasMCP,
		})
		return steps
	}

	// MCP selection (dynamic).
	if hasMCP {
		mItems := make([]stepflow.ListItem, 0, len(s.mcpGroups))
		for idx, g := range s.mcpGroups {
			lbl := wizardMCPGroupOptionLabel(g)
			s.mcpLabelToIdx[lbl] = idx
			mItems = append(mItems, stepflow.Item(lbl))
		}

		mList := stepflow.List(wizardKeyImportMCP, "Select MCP servers to import").
			Items(mItems...).
			MultiSelect(true).
			VisibleRows(8)

		steps = append(steps, &dynamicMCPSelectionStep{
			ListStep:      mList,
			mcpGroups:     s.mcpGroups,
			mcpLabelToIdx: s.mcpLabelToIdx,
		})
		return steps
	}

	// Should be unreachable because either skills or MCP (or both) would be
	// non-empty when guidelines are the only category (handled above).
	return steps
}

type dynamicGuidelinesSelectionStep struct {
	*stepflow.ListStep
}

func (s *dynamicGuidelinesSelectionStep) NextSteps(completed stepflow.Result) []stepflow.Step {
	askGuidelines := completed[wizardKeyImportGuidelines] == ""
	return wizardMakeSurveySteps(askGuidelines, true, true)
}

type dynamicSkillSelectionStep struct {
	*stepflow.ListStep

	skillGroups     []detect.SkillGroup
	skillLabelToIdx map[string]int

	mcpGroups     []detect.MCPGroup
	mcpLabelToIdx map[string]int
	hasMCP        bool
}

func (s *dynamicSkillSelectionStep) NextSteps(completed stepflow.Result) []stepflow.Step {
	selected := wizardSplitMultiAnswer(completed[wizardKeyImportSkills])

	// Insert per-skill variant steps only for selected conflict groups.
	var steps []stepflow.Step
	for _, label := range selected {
		idx, ok := s.skillLabelToIdx[label]
		if !ok {
			continue
		}
		group := s.skillGroups[idx]
		if !group.Conflict {
			continue
		}

		variantKey := wizardSkillVariantStepKey(group.Name)
		vItems := make([]stepflow.ListItem, 0, len(group.Variants))
		for _, v := range group.Variants {
			vItems = append(vItems, stepflow.Item(wizardSkillVariantOptionLabel(v)))
		}

		steps = append(steps,
			stepflow.List(variantKey, fmt.Sprintf("Select a version for skill %q", group.Name)).
				Items(vItems...).
				MultiSelect(false).
				VisibleRows(8),
		)
	}

	if s.hasMCP {
		// Insert MCP selection next (it will decide whether to ask for sample MCP).
		mItems := make([]stepflow.ListItem, 0, len(s.mcpGroups))
		for _, g := range s.mcpGroups {
			mItems = append(mItems, stepflow.Item(wizardMCPGroupOptionLabel(g)))
		}

		mList := stepflow.List(wizardKeyImportMCP, "Select MCP servers to import").
			Items(mItems...).
			MultiSelect(true).
			VisibleRows(8)

		steps = append(steps, &dynamicMCPSelectionStep{
			ListStep:      mList,
			mcpGroups:     s.mcpGroups,
			mcpLabelToIdx: s.mcpLabelToIdx,
		})
		return steps
	}

	askGuidelines := completed[wizardKeyImportGuidelines] == ""
	askSampleSkill := completed[wizardKeyImportSkills] == ""
	// With no MCP detected/imported, we always ask about sample MCP.
	askMCP := true

	steps = append(steps, wizardMakeSurveySteps(askGuidelines, askSampleSkill, askMCP)...)
	return steps
}

type dynamicMCPSelectionStep struct {
	*stepflow.ListStep

	mcpGroups     []detect.MCPGroup
	mcpLabelToIdx map[string]int
}

func (s *dynamicMCPSelectionStep) NextSteps(completed stepflow.Result) []stepflow.Step {
	selected := wizardSplitMultiAnswer(completed[wizardKeyImportMCP])

	var steps []stepflow.Step
	for _, label := range selected {
		idx, ok := s.mcpLabelToIdx[label]
		if !ok {
			continue
		}
		group := s.mcpGroups[idx]
		if !group.Conflict {
			continue
		}

		variantKey := wizardMCPVariantStepKey(group.Name)
		vItems := make([]stepflow.ListItem, 0, len(group.Variants))
		for _, v := range group.Variants {
			vItems = append(vItems, stepflow.Item(wizardMCPVariantOptionLabel(v)))
		}

		steps = append(steps,
			stepflow.List(variantKey, fmt.Sprintf("Select a version for MCP server %q", group.Name)).
				Items(vItems...).
				MultiSelect(false).
				VisibleRows(8),
		)
	}

	askGuidelines := completed[wizardKeyImportGuidelines] == ""
	askSampleSkill := completed[wizardKeyImportSkills] == ""
	askMCP := completed[wizardKeyImportMCP] == ""

	steps = append(steps, wizardMakeSurveySteps(askGuidelines, askSampleSkill, askMCP)...)
	return steps
}

func printImportSummary(cmd *cobra.Command, items []importSummaryItem) {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(ThemeGreen)
	checkStyle := lipgloss.NewStyle().Foreground(ThemeGreen)

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render("Importing existing configs..."))
	for _, item := range items {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s %-10s → %s\n", checkStyle.Render("✓"), item.Label, item.Value)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
}
