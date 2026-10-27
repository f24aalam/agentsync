package agent

import (
	"path/filepath"

	"github.com/f24aalam/agentsync/internal/config"
)

// InstallPlan records user choices for install. When Skip* is true for a key, that step is skipped.
type InstallPlan struct {
	SkipGuidelines     map[string]bool  // agent ID -> skip writing merged guidelines
	SkipGuidelinesDest map[string]bool  // destination path (e.g. "AGENTS.md") -> skip writing
	SkipMCP            map[string]bool  // agent ID -> skip MCP merge/write
	SkipMCPDest        map[string]bool  // destination path (e.g. ".cursor/mcp.json") -> skip MCP merge/write
	SkipMCPServers     map[string]bool  // destination path + "/" + serverName -> skip writing this server
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
		SkipMCPServers:     make(map[string]bool),
		SkipSkillsDir:      make(map[string]bool),
		SkipSkillPackages:  make(map[string]bool),
		Lock:               config.NewLockFile(),
	}
}

// SkipMCPServer marks a specific server in a destination config as skipped.
func (p *InstallPlan) SkipMCPServer(destPath, serverName string) {
	if p.SkipMCPServers == nil {
		p.SkipMCPServers = make(map[string]bool)
	}
	p.SkipMCPServers[filepath.Clean(destPath)+"/"+serverName] = true
}

// IsMCPServerSkipped reports whether a specific server in a destination config should be skipped.
func (p *InstallPlan) IsMCPServerSkipped(destPath, serverName string) bool {
	if p == nil || p.SkipMCPServers == nil {
		return false
	}
	return p.SkipMCPServers[filepath.Clean(destPath)+"/"+serverName]
}
