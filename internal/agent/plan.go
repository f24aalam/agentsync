package agent

import "github.com/f24aalam/agentsync/internal/config"

// InstallPlan records user choices for install. When Skip* is true for a key, that step is skipped.
type InstallPlan struct {
	SkipGuidelines     map[string]bool  // agent ID -> skip writing merged guidelines
	SkipGuidelinesDest map[string]bool  // destination path (e.g. "AGENTS.md") -> skip writing
	SkipMCP            map[string]bool  // agent ID -> skip MCP merge/write
	SkipMCPDest        map[string]bool  // destination path (e.g. ".cursor/mcp.json") -> skip MCP merge/write
	SkipSkillsDir      map[string]bool  // skills output dir (as in Agent.SkillsDir) -> skip copy from .ai/skills
	SkipSkillPackages  map[string]bool  // package key (e.g. ".claude/skills/code-review") -> skip package
	Lock               *config.LockFile // ownership lock metadata
}

// NewInstallPlan returns a plan with no skips (full install).
func NewInstallPlan() InstallPlan {
	return InstallPlan{
		SkipGuidelines:     make(map[string]bool),
		SkipGuidelinesDest: make(map[string]bool),
		SkipMCP:            make(map[string]bool),
		SkipMCPDest:        make(map[string]bool),
		SkipSkillsDir:      make(map[string]bool),
		SkipSkillPackages:  make(map[string]bool),
		Lock:               config.NewLockFile(),
	}
}
