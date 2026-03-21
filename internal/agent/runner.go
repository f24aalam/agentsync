package agent

import (
	"path/filepath"
)

// SkillDirKey normalizes a skills output path for maps and lookups.
func SkillDirKey(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Clean(dir)
}

// RunSummary aggregates per-agent install results.
type RunSummary struct {
	Results         []InstallResult
	ConfiguredCount int
}

// Run installs guidelines, shared skills trees, and MCP for each agent according to plan.
// root is the project root (usually "."); empty root means ".".
func Run(agents []Agent, plan InstallPlan, root string) RunSummary {
	if root == "" {
		root = "."
	}

	wouldSkills := wouldInstallSkills(root)
	skillStepsByDir := computeSkillStepsByDir(agents, plan, root, wouldSkills)

	results := make([]InstallResult, 0, len(agents))
	configuredCount := 0

	for _, target := range agents {
		var skillsStep StepResult
		if !target.SkillsSupported {
			skillsStep = StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusSkipped}
		} else {
			base, ok := skillStepsByDir[SkillDirKey(target.SkillsDir)]
			if !ok {
				base = StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusSkipped}
			}
			skillsStep = base
			skillsStep.Target = target.SkillsDir
		}

		result := InstallAgent(target, root, skillsStep, plan)
		results = append(results, result)
		if result.Succeeded() {
			configuredCount++
		}
	}

	return RunSummary{
		Results:         results,
		ConfiguredCount: configuredCount,
	}
}

func computeSkillStepsByDir(agents []Agent, plan InstallPlan, root string, wouldSkills bool) map[string]StepResult {
	out := make(map[string]StepResult)

	dirs := uniqueSkillDirsForInstall(agents)
	for _, dir := range dirs {
		key := SkillDirKey(dir)
		if !wouldSkills {
			out[key] = StepResult{Name: "Skills", Target: dir, Status: StepStatusSkipped}
			continue
		}
		if plan.SkipSkillsDir[key] {
			out[key] = StepResult{Name: "Skills", Target: dir, Status: StepStatusSkipped}
			continue
		}
		if err := copySkillTreesFromAI(root, dir); err != nil {
			out[key] = StepResult{Name: "Skills", Target: dir, Status: StepStatusError, Err: err}
			continue
		}
		out[key] = StepResult{Name: "Skills", Target: dir, Status: StepStatusOK}
	}

	return out
}

func uniqueSkillDirsForInstall(agents []Agent) []string {
	seen := make(map[string]bool)
	var dirs []string
	for _, a := range agents {
		if !a.SkillsSupported || a.SkillsDir == "" {
			continue
		}
		key := SkillDirKey(a.SkillsDir)
		if seen[key] {
			continue
		}
		seen[key] = true
		dirs = append(dirs, key)
	}
	return dirs
}
