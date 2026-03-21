package agent

// InstallPlan records user choices for install. When Skip* is true for a key, that step is skipped.
type InstallPlan struct {
	SkipGuidelines map[string]bool // agent ID -> skip writing merged guidelines
	SkipMCP        map[string]bool // agent ID -> skip MCP merge/write
	SkipSkillsDir  map[string]bool // skills output dir (as in Agent.SkillsDir) -> skip copy from .ai/skills
}

// NewInstallPlan returns a plan with no skips (full install).
func NewInstallPlan() InstallPlan {
	return InstallPlan{
		SkipGuidelines: make(map[string]bool),
		SkipMCP:        make(map[string]bool),
		SkipSkillsDir:  make(map[string]bool),
	}
}
