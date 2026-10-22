package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentpkg "github.com/f24aalam/agentsync/internal/agent"
	stepflow "github.com/f24aalam/stepflow/pkg"
)

// TestIntegration_Install_AllAgents_Idempotency verifies that `agentsync install`
// successfully configures all 7 supported agents and is completely idempotent when run repeatedly.
func TestIntegration_Install_AllAgents_Idempotency(t *testing.T) {
	env := newIntegrationEnv(t)

	allAgents := []string{
		"claude-code",
		"cursor",
		"codex",
		"gemini-cli",
		"github-copilot",
		"junie",
		"opencode",
	}

	env.PopulateStandardAI(allAgents)

	// --- 1. Initial Installation ---
	stdout, stderr, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("first install failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	if !strings.Contains(stdout, "Done! 7 agents configured.") {
		t.Fatalf("expected 7 agents configured on first install, got:\n%s", stdout)
	}

	// Helper to read and assert
	assertCommonInstalledState := func() {
		// 1. Claude Code
		env.AssertFileExists("CLAUDE.md")
		env.AssertFileContains("CLAUDE.md", "Project Core Guidelines")
		env.AssertFileContains("CLAUDE.md", "Coding Style Guidelines")
		env.AssertFileExists(".claude/skills/code-review/SKILL.md")
		env.AssertFileExists(".claude/skills/code-review/scripts/review.sh")
		env.AssertFileExists(".mcp.json")
		env.AssertFileContains(".mcp.json", `"mcpServers"`)
		env.AssertFileContains(".mcp.json", `"postgres"`)
		env.AssertFileContains(".mcp.json", `"filesystem"`)

		// 2. Cursor
		env.AssertFileExists(".cursor/rules/agentsync.mdc")
		env.AssertFileContains(".cursor/rules/agentsync.mdc", "Project Core Guidelines")
		env.AssertFileExists(".cursor/skills/code-review/SKILL.md")
		env.AssertFileExists(".cursor/mcp.json")
		env.AssertFileContains(".cursor/mcp.json", `"postgres"`)

		// 3. Codex
		env.AssertFileExists("AGENTS.md")
		env.AssertFileContains("AGENTS.md", "Project Core Guidelines")
		env.AssertFileExists(".agents/skills/code-review/SKILL.md")
		env.AssertFileExists(".codex/config.toml")
		env.AssertFileContains(".codex/config.toml", "[mcp_servers.postgres]")

		// 4. Gemini CLI (SkillsSupported: false -> no skills directory)
		env.AssertFileExists("GEMINI.md")
		env.AssertFileContains("GEMINI.md", "Project Core Guidelines")
		env.AssertFileExists(".gemini/settings.json")
		env.AssertFileContains(".gemini/settings.json", `"postgres"`)

		// 5. GitHub Copilot
		env.AssertFileExists(".github/skills/code-review/SKILL.md")
		env.AssertFileExists(".vscode/mcp.json")
		env.AssertFileContains(".vscode/mcp.json", `"servers"`)
		env.AssertFileContains(".vscode/mcp.json", `"postgres"`)
		// Verify project-scoped Copilot config and ensure global HOME is untouched
		if env.HomeFileExists(".copilot/mcp-config.json") {
			t.Fatalf("unexpected global Copilot MCP config written to HOME/.copilot/mcp-config.json in project-scoped install")
		}

		// 6. Junie
		env.AssertFileExists(".junie/guidelines.md")
		env.AssertFileContains(".junie/guidelines.md", "Project Core Guidelines")
		env.AssertFileExists(".junie/skills/code-review/SKILL.md")
		env.AssertFileExists(".junie/mcp/mcp.json")
		env.AssertFileContains(".junie/mcp/mcp.json", `"postgres"`)

		// 7. OpenCode
		env.AssertFileExists("opencode.json")
		env.AssertFileContains("opencode.json", `"mcp"`)
		env.AssertFileContains("opencode.json", `"https://opencode.ai/config.json"`)
		env.AssertFileContains("opencode.json", `"type": "local"`)
		env.AssertFileContains("opencode.json", `"postgres"`)
	}

	assertCommonInstalledState()

	// --- 2. Repeated Installation (Idempotency) ---
	stdout2, stderr2, err2 := env.RunInstall("-y")
	if err2 != nil {
		t.Fatalf("second install failed: %v\nStderr: %s\nStdout: %s", err2, stderr2, stdout2)
	}

	if !strings.Contains(stdout2, "Done! 7 agents configured.") {
		t.Fatalf("expected 7 agents configured on repeated install, got:\n%s", stdout2)
	}

	assertCommonInstalledState()
}

// TestIntegration_SharedAgentsMD_SequentialWrites verifies that when multiple agents
// target AGENTS.md (Codex, GitHub Copilot, and OpenCode), all write identical content cleanly.
func TestIntegration_SharedAgentsMD_SequentialWrites(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulateStandardAI([]string{"codex", "github-copilot", "opencode"})

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	content := env.ReadFile("AGENTS.md")
	// Verify guidelines are present and not duplicated 3x in the same file
	coreOccurrences := strings.Count(content, "# Project Core Guidelines")
	if coreOccurrences != 1 {
		t.Fatalf("expected exactly 1 occurrence of core guidelines in AGENTS.md, found %d\nContent:\n%s", coreOccurrences, content)
	}
}

// TestIntegration_JSON_PreserveUserConfig verifies that non-MCP user keys
// in existing JSON configs (e.g. opencode.json and .mcp.json) are preserved during install.
func TestIntegration_JSON_PreserveUserConfig(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulatePreExistingConfigs()
	env.PopulateStandardAI([]string{"opencode", "claude-code"})

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// 1. Verify opencode.json preserved user settings outside "mcp"
	opencodeContent := env.ReadFile("opencode.json")
	var opencodeMap map[string]any
	if err := json.Unmarshal([]byte(opencodeContent), &opencodeMap); err != nil {
		t.Fatalf("failed to parse opencode.json: %v", err)
	}

	if opencodeMap["theme"] != "dark" {
		t.Fatalf("expected theme 'dark' preserved, got: %v", opencodeMap["theme"])
	}
	if opencodeMap["developer_notes"] != "Important local settings" {
		t.Fatalf("expected developer_notes preserved, got: %v", opencodeMap["developer_notes"])
	}

	// Verify both user's existing tool and newly synced tools exist
	mcpServers, ok := opencodeMap["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcp map in opencode.json")
	}
	if mcpServers["user_local_tool"] == nil {
		t.Fatalf("expected user_local_tool preserved in opencode.json")
	}
	if mcpServers["postgres"] == nil {
		t.Fatalf("expected synced postgres tool added to opencode.json")
	}

	// 2. Verify .mcp.json preserved custom_setting outside "mcpServers"
	claudeContent := env.ReadFile(".mcp.json")
	var claudeMap map[string]any
	if err := json.Unmarshal([]byte(claudeContent), &claudeMap); err != nil {
		t.Fatalf("failed to parse .mcp.json: %v", err)
	}
	if claudeMap["custom_setting"] != true {
		t.Fatalf("expected custom_setting: true preserved in .mcp.json")
	}
	claudeServers, ok := claudeMap["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers map in .mcp.json")
	}
	if claudeServers["pre_existing_claude"] == nil {
		t.Fatalf("expected pre_existing_claude preserved in .mcp.json")
	}
	if claudeServers["postgres"] == nil {
		t.Fatalf("expected postgres added to .mcp.json")
	}
}

// TestIntegration_ConflictPrompts_InteractiveVsYes verifies interactive conflict prompts
// when existing files exist vs non-interactive --yes behavior.
func TestIntegration_ConflictPrompts_InteractiveVsYes(t *testing.T) {
	env := newIntegrationEnv(t)

	// Pre-create CLAUDE.md and .mcp.json with initial content
	env.WriteFile("CLAUDE.md", "User Manual Claude Instructions\n")
	env.WriteFile(".claude/skills/user-skill/SKILL.md", "Existing skill\n")
	env.WriteFile(".mcp.json", `{"mcpServers": {"existing": {"command": "tool"}}}`)

	env.PopulateStandardAI([]string{"claude-code"})

	// Override interactive stepflow to simulate user answering:
	// - Guidelines overwrite: NO (preserve user's CLAUDE.md)
	// - Skills overwrite: YES (update skills)
	// - MCP overwrite: NO (preserve user's .mcp.json)
	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			key := s.Key()
			switch key {
			case "install/claude-code/guidelines":
				res[key] = "No"
			case "install/skills/.claude_skills":
				res[key] = "Yes"
			case "install/claude-code/mcp":
				res[key] = "No"
			}
		}
		return res, nil
	}

	stdout, stderr, err := env.RunInstall() // interactive (no -y)
	if err != nil {
		t.Fatalf("interactive install returned error: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	// 1. Guidelines were skipped -> original content preserved
	env.AssertFileContains("CLAUDE.md", "User Manual Claude Instructions")
	env.AssertFileNotContains("CLAUDE.md", "Project Core Guidelines")

	// 2. Skills were accepted -> new skill added
	env.AssertFileExists(".claude/skills/code-review/SKILL.md")

	// 3. MCP was skipped -> original content preserved without new postgres server
	env.AssertFileContains(".mcp.json", "existing")
	env.AssertFileNotContains(".mcp.json", "postgres")

	// Now run with -y: everything should overwrite regardless of previous skip
	stdoutY, stderrY, errY := env.RunInstall("-y")
	if errY != nil {
		t.Fatalf("install -y failed: %v\nStderr: %s\nStdout: %s", errY, stderrY, stdoutY)
	}

	env.AssertFileContains("CLAUDE.md", "Project Core Guidelines")
	env.AssertFileContains(".mcp.json", "postgres")
	_ = stdoutY
}

// TestSafety_MCP_DoNotDeleteUnmanagedUserServers verifies that unmanaged user servers
// in existing JSON files are never deleted by AgentSync.
func TestSafety_MCP_DoNotDeleteUnmanagedUserServers(t *testing.T) {
	env := newIntegrationEnv(t)

	// Pre-existing user-defined MCP server in .mcp.json
	env.WriteFile(".mcp.json", `{
  "mcpServers": {
    "my_private_database": {
      "command": "custom-db-proxy",
      "args": ["--port", "5432"]
    }
  }
}`)
	// .ai/mcp.toml has only standard servers
	env.PopulateStandardAI([]string{"claude-code"})

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	content := env.ReadFile(".mcp.json")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse .mcp.json: %v", err)
	}

	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers map in .mcp.json")
	}

	// The private user server must be preserved
	if servers["my_private_database"] == nil {
		t.Errorf("SAFETY VIOLATION: unmanaged user server 'my_private_database' was deleted!")
	}
	// The synced server must be added
	if servers["postgres"] == nil {
		t.Errorf("expected synced server 'postgres' to be present in .mcp.json")
	}
}

// TestIntegration_Skills_SymlinkAndDirectFiles verifies how symlinks and root files
// in .ai/skills are handled during installation.
func TestIntegration_Skills_SymlinkAndDirectFiles(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulateStandardAI([]string{"codex"})

	// Create a skill with an internal symlink
	env.WriteFile(".ai/skills/symlinked/SKILL.md", "Skill with symlink\n")
	env.WriteFile(".ai/skills/symlinked/data/real.txt", "Original target content\n")

	// Symlink data/link.txt -> data/real.txt
	targetPath := filepath.Join(env.RootDir, ".ai/skills/symlinked/data/real.txt")
	linkPath := filepath.Join(env.RootDir, ".ai/skills/symlinked/data/link.txt")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Skipf("skipping symlink test (symlinks not supported on this filesystem): %v", err)
	}

	// Direct file in .ai/skills root (not in a subdirectory)
	env.WriteFile(".ai/skills/ROOT_README.txt", "Direct root file\n")

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install with symlinked skill failed: %v", err)
	}

	// 1. Verify real file and symlink were copied
	env.AssertFileExists(".agents/skills/symlinked/data/real.txt")
	env.AssertFileExists(".agents/skills/symlinked/data/link.txt")
	env.AssertFileContains(".agents/skills/symlinked/data/link.txt", "Original target content")

	// 2. Direct root files in .ai/skills are currently ignored by design
	env.AssertFileNotExists(".agents/skills/ROOT_README.txt")
}

// TestIntegration_Phase2_CompleteLifecycle verifies the complete
// first-install -> repeat-install -> update -> user-modify -> delete lifecycle.
func TestIntegration_Phase2_CompleteLifecycle(t *testing.T) {
	env := newIntegrationEnv(t)

	// Agents: Claude Code and Codex
	env.PopulateStandardAI([]string{"claude-code", "codex"})

	// --- Step 1: First install ---
	stdout1, stderr1, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("first install failed: %v\nStderr: %s\nStdout: %s", err, stderr1, stdout1)
	}

	env.AssertFileExists("CLAUDE.md")
	env.AssertFileExists("AGENTS.md")
	env.AssertFileExists(".claude/skills/code-review/SKILL.md")
	env.AssertFileExists(".mcp.json")
	env.AssertFileExists(".codex/config.toml")
	env.AssertFileExists(".ai/sync.lock")

	lockContent := env.ReadFile(".ai/sync.lock")
	if !strings.Contains(lockContent, "CLAUDE.md") || !strings.Contains(lockContent, "AGENTS.md") {
		t.Fatalf("expected guidelines tracked in sync.lock:\n%s", lockContent)
	}
	if !strings.Contains(lockContent, ".claude/skills/code-review/SKILL.md") {
		t.Fatalf("expected skills tracked in sync.lock:\n%s", lockContent)
	}
	if !strings.Contains(lockContent, "postgres") {
		t.Fatalf("expected MCP tracked in sync.lock:\n%s", lockContent)
	}

	// --- Step 2: Repeat install (must be 100% idempotent with zero conflict prompts) ---
	targets := []agentpkg.Agent{
		mustGetAgent("claude-code"),
		mustGetAgent("codex"),
	}
	conflicts, err := agentpkg.DetectInstallConflicts(targets, env.RootDir)
	if err != nil {
		t.Fatalf("detect conflicts on repeat install failed: %v", err)
	}
	if len(conflicts) > 0 {
		var conflictKeys []string
		for _, c := range conflicts {
			conflictKeys = append(conflictKeys, c.StepKey)
		}
		t.Fatalf("expected 0 conflicts on repeat install, got: %v", conflictKeys)
	}

	stdout2, stderr2, err2 := env.RunInstall("-y")
	if err2 != nil {
		t.Fatalf("repeat install failed: %v\nStderr: %s\nStdout: %s", err2, stderr2, stdout2)
	}
	if !strings.Contains(stdout2, "Done! 2 agents configured.") {
		t.Fatalf("expected 2 agents configured on repeat install, got:\n%s", stdout2)
	}

	// --- Step 3: Update .ai/ source (safe update without prompt fatigue) ---
	env.WriteFile(".ai/guidelines/01-core.md", "# Project Core Guidelines\nUpdated new requirement.\n")
	conflictsAfterUpdate, err := agentpkg.DetectInstallConflicts(targets, env.RootDir)
	if err != nil {
		t.Fatalf("detect conflicts after update failed: %v", err)
	}
	if len(conflictsAfterUpdate) > 0 {
		t.Fatalf("expected 0 conflicts for safe update from .ai/, got: %+v", conflictsAfterUpdate)
	}

	_, _, err = env.RunInstall("-y")
	if err != nil {
		t.Fatalf("update install failed: %v", err)
	}
	env.AssertFileContains("CLAUDE.md", "Updated new requirement")
	env.AssertFileContains("AGENTS.md", "Updated new requirement")

	// --- Step 4: User modification detection (genuine conflict) ---
	// User edits inside managed section of CLAUDE.md
	userModifiedCLAUDE := `# Header
` + agentpkg.ManagedSectionStart + `
# Project Core Guidelines
User edited directly inside the managed block!
` + agentpkg.ManagedSectionEnd + `
`
	env.WriteFile("CLAUDE.md", userModifiedCLAUDE)

	conflictsUserMod, err := agentpkg.DetectInstallConflicts([]agentpkg.Agent{mustGetAgent("claude-code")}, env.RootDir)
	if err != nil {
		t.Fatalf("detect conflicts on user modification failed: %v", err)
	}
	foundConflict := false
	for _, c := range conflictsUserMod {
		if c.Kind == agentpkg.KindGuidelines && c.AgentID == "claude-code" {
			foundConflict = true
			break
		}
	}
	if !foundConflict {
		t.Fatalf("expected genuine conflict detected for user-modified CLAUDE.md, got none!")
	}

	// --- Step 5: No Orphan Pruning on Deletion from .ai/ (v1 contract) ---
	// Add an unmanaged file in destination skill dir
	unmanagedCustomFile := ".claude/skills/code-review/custom-notes.txt"
	env.WriteFile(unmanagedCustomFile, "Keep this file safe!")

	// Delete code-review skill from .ai/skills
	_ = os.RemoveAll(filepath.Join(env.RootDir, ".ai/skills/code-review"))

	// Install with -y
	_, _, err = env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install after deletion failed: %v", err)
	}

	// In v1, destination files are left alone when source is removed from .ai/
	env.AssertFileExists(".claude/skills/code-review/SKILL.md")
	// Unmanaged user-authored file must also be preserved
	env.AssertFileExists(unmanagedCustomFile)
	env.AssertFileContains(unmanagedCustomFile, "Keep this file safe!")
}
