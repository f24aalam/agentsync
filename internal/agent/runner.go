package agent

import (
	"path/filepath"
	"strings"
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

	writtenGuidelines := make(map[string]StepResult)

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

		destG := resolveGuidelinesTarget(target)
		var gStep StepResult
		if prev, ok := writtenGuidelines[destG]; ok {
			gStep = prev
		} else {
			skipG := plan.SkipGuidelines[target.ID] || plan.SkipGuidelinesDest[destG]
			gStep = installGuidelinesWithLock(target, root, skipG, plan.Lock)
			writtenGuidelines[destG] = gStep
		}

		result := InstallResult{
			Agent: target,
			Steps: []StepResult{
				gStep,
				skillsStep,
				installMCPWithPlan(target, root, plan),
			},
		}
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
		if plan.SkipSkillsDir[key] {
			out[key] = StepResult{Name: "Skills", Target: dir, Status: StepStatusSkipped}
			continue
		}
		if !wouldSkills {
			hasTracked := false
			if plan.Lock != nil && plan.Lock.Skills != nil {
				normDestPrefix := filepath.Clean(dir) + string(filepath.Separator)
				for p := range plan.Lock.Skills {
					if strings.HasPrefix(filepath.Clean(p), normDestPrefix) {
						hasTracked = true
						break
					}
				}
			}
			if !hasTracked {
				out[key] = StepResult{Name: "Skills", Target: dir, Status: StepStatusSkipped}
				continue
			}
		}
		if err := syncSkillTreesWithPlan(root, dir, plan); err != nil {
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
