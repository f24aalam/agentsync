package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentpkg "github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/detect"
	stepflow "github.com/f24aalam/stepflow/pkg"
)

// 1. Data Loss in Codex TOML
// TestRegression_Codex_PreserveUserConfig reproduces Issue 3.1:
// Installing MCP for Codex wipes out user-owned TOML keys (e.g. model, temperature, custom tables).
func TestRegression_Codex_PreserveUserConfig(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".codex/config.toml", `# User settings
model = "gpt-4o"
temperature = 0.2

[custom_section]
key = "important_value"

[mcp_servers.existing_tool]
command = "echo"
args = ["hello"]
`)

	env.PopulateStandardAI([]string{"codex"})

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	codexContent := env.ReadFile(".codex/config.toml")

	if !strings.Contains(codexContent, `model = "gpt-4o"`) {
		t.Errorf("BUG REPRODUCED (Issue 3.1): model config was wiped from .codex/config.toml!\nContent:\n%s", codexContent)
	}
	if !strings.Contains(codexContent, "temperature = 0.2") {
		t.Errorf("BUG REPRODUCED (Issue 3.1): temperature was wiped from .codex/config.toml!\nContent:\n%s", codexContent)
	}
	if !strings.Contains(codexContent, "[custom_section]") {
		t.Errorf("BUG REPRODUCED (Issue 3.1): [custom_section] was wiped from .codex/config.toml!\nContent:\n%s", codexContent)
	}
}

// TestRegression_InvalidTOML_MustNotDestroyCorruptedContent verifies that
// malformed existing TOML is rejected and preserved without modification.
func TestRegression_InvalidTOML_MustNotDestroyCorruptedContent(t *testing.T) {
	env := newIntegrationEnv(t)

	corruptedTOML := `# Malformed TOML file
model = "unclosed string without end quote
[mcp_servers]
`
	env.WriteFile(".codex/config.toml", corruptedTOML)
	env.PopulateStandardAI([]string{"codex"})

	stdout, stderr, err := env.RunInstall("-y")
	_ = stdout
	_ = stderr
	_ = err

	currentContent := env.ReadFile(".codex/config.toml")
	if currentContent != corruptedTOML {
		t.Errorf("SAFETY VIOLATION: Malformed .codex/config.toml was modified or destroyed!\nContent:\n%s", currentContent)
	}
}

// 2. Tilde Path Expansion in detectMCP
// TestRegression_Detect_CopilotHomeTildeExpansion reproduces Issue 3.2:
// `detectMCP` does not expand "~/" for Copilot global config, searching "./~/.copilot/..." instead of user home.
func TestRegression_Detect_CopilotHomeTildeExpansion(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteHomeFile(".copilot/mcp-config.json", `{
  "servers": {
    "copilot_global_mcp": {
      "type": "stdio",
      "command": "global-tool-cmd",
      "args": ["serve"]
    }
  }
}`)

	detection, err := detect.ScanProject(env.RootDir)
	if err != nil {
		t.Fatalf("scan project failed: %v", err)
	}

	found := false
	for _, srv := range detection.MCPServers {
		if srv.Name == "copilot_global_mcp" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("BUG REPRODUCED (Issue 3.2): detectMCP failed to detect ~/.copilot/mcp-config.json in user HOME directory")
	}
}

// 3. Remote MCP Dropped During Detection
// TestRegression_Detect_RemoteMCPOpenCode reproduces Issue 3.5:
// `detectMCP` drops any MCP server with an empty `Command`, ignoring remote HTTP/SSE servers from OpenCode.
func TestRegression_Detect_RemoteMCPOpenCode(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile("opencode.json", `{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "remote_weather_api": {
      "type": "remote",
      "url": "https://weather.example.com/mcp",
      "enabled": true
    }
  }
}`)

	detection, err := detect.ScanProject(env.RootDir)
	if err != nil {
		t.Fatalf("scan project failed: %v", err)
	}

	found := false
	for _, srv := range detection.MCPServers {
		if srv.Name == "remote_weather_api" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("BUG REPRODUCED (Issue 3.5): detectMCP dropped remote server 'remote_weather_api' because Command is empty")
	}
}

// 4. Remote MCP Generates Invalid Empty Command
// TestRegression_MCP_RemoteServerNonOpenCode reproduces Issue 3.3:
// Installing a remote MCP server for Claude Code produces invalid `{"command": ""}`.
func TestRegression_MCP_RemoteServerNonOpenCode(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".ai/mcp.toml", `
[servers.remote_service]
type = "remote"
url = "https://mcp.internal.net/sse"
`)
	env.WriteFile(".ai/guidelines/core.md", "# Core\n")
	env.WriteFile(".ai/sync.lock", `agents = ["claude-code"]`+"\n")

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	claudeContent := env.ReadFile(".mcp.json")
	if strings.Contains(claudeContent, `"command": ""`) {
		t.Errorf("BUG REPRODUCED (Issue 3.3): Claude .mcp.json contains broken empty command for remote MCP server!\nContent:\n%s", claudeContent)
	}
}

// 5. Silent Overwrite of Corrupted JSON
// TestRegression_InvalidJSON_MustNotDestroyCorruptedContent reproduces an unsafe behavior:
// When target JSON contains a syntax error, install silently destroys it with a fresh root instead of aborting/reporting error.
func TestRegression_InvalidJSON_MustNotDestroyCorruptedContent(t *testing.T) {
	env := newIntegrationEnv(t)

	corruptedOriginal := `{"mcpServers": { "important_server": { "command": "tool" } /* UNFINISHED EDIT`
	env.WriteFile(".mcp.json", corruptedOriginal)
	env.PopulateStandardAI([]string{"claude-code"})

	stdout, stderr, err := env.RunInstall("-y")
	_ = stdout
	_ = stderr
	_ = err

	currentContent := env.ReadFile(".mcp.json")

	// Safe behavior: AgentSync must NOT silently wipe user's damaged file.
	// It should fail or preserve the corrupted content with an error.
	if currentContent != corruptedOriginal && !strings.Contains(currentContent, "UNFINISHED EDIT") {
		t.Errorf("BUG REPRODUCED: AgentSync silently destroyed corrupted .mcp.json and wiped user's salvageable data!\nOverwritten Content:\n%s", currentContent)
	}
}

// 6. Repeated Installs Trigger Prompt Fatigue on Unchanged Files
// TestRegression_RepeatedInstall_NoPromptWhenUnchanged proves that interactive install
// prompts the user for conflicts even when all destination files are already up-to-date and identical.
func TestRegression_RepeatedInstall_NoPromptWhenUnchanged(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulateStandardAI([]string{"claude-code"})

	// First install sets up the files
	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("initial install failed: %v", err)
	}

	// Detect conflicts again on the unchanged repository
	target, _ := agentpkg.ByID("claude-code")
	conflicts, err := agentpkg.DetectInstallConflicts([]agentpkg.Agent{target}, ".")
	if err != nil {
		t.Fatalf("detect conflicts failed: %v", err)
	}

	// Safe/smart behavior: If files already match what would be written, no conflicts should be reported!
	if len(conflicts) > 0 {
		var conflictKeys []string
		for _, c := range conflicts {
			conflictKeys = append(conflictKeys, c.StepKey)
		}
		t.Errorf("BUG REPRODUCED: Unnecessary conflict prompts generated on identical unchanged files: %v", conflictKeys)
	}
}

// 7. Shared AGENTS.md Destination Multi-Prompt & Conflict Violation
// TestRegression_SharedAgentsMD_SingleConflictDecision proves that multiple agents
// sharing AGENTS.md produce multiple confusing prompts, and a 'No' decision can be violated.
func TestRegression_SharedAgentsMD_SingleConflictDecision(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile("AGENTS.md", "# Original User AGENTS.md\n")
	env.PopulateStandardAI([]string{"codex", "github-copilot", "opencode"})

	targets := []agentpkg.Agent{
		mustGetAgent("codex"),
		mustGetAgent("github-copilot"),
		mustGetAgent("opencode"),
	}

	conflicts, err := agentpkg.DetectInstallConflicts(targets, ".")
	if err != nil {
		t.Fatalf("detect conflicts failed: %v", err)
	}

	// Count how many conflict prompts were generated for AGENTS.md
	guidelinesConflicts := 0
	for _, c := range conflicts {
		if c.Kind == agentpkg.KindGuidelines {
			guidelinesConflicts++
		}
	}

	// Intended behavior: A shared destination file should have exactly 1 consolidated prompt,
	// just like shared skill directories (.agents/skills/) do!
	if guidelinesConflicts > 1 {
		t.Errorf("BUG REPRODUCED: %d separate conflict prompts generated for the exact same AGENTS.md destination!", guidelinesConflicts)
	}

	// Furthermore: simulate user saying NO to Codex guidelines, but YES to OpenCode
	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			if s.Key() == "install/codex/guidelines" {
				res[s.Key()] = "No" // User requested to preserve AGENTS.md
			} else {
				res[s.Key()] = "Yes"
			}
		}
		return res, nil
	}

	_, _, _ = env.RunInstall() // interactive

	// If user answered "No" when prompted about AGENTS.md for Codex, OpenCode must not overwrite it!
	content := env.ReadFile("AGENTS.md")
	if strings.Contains(content, "Project Core Guidelines") {
		t.Errorf("BUG REPRODUCED: User's decision to preserve AGENTS.md was overridden by subsequent agent in the loop!")
	}
}

// 8. Global Copilot Configuration Modified Without Permission
// TestRegression_Copilot_DoNotModifyGlobalHomeWithoutPermission proves that installing
// for github-copilot automatically writes to user's global ~/.copilot without explicit opt-in.
func TestRegression_Copilot_DoNotModifyGlobalHomeWithoutPermission(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulateStandardAI([]string{"github-copilot"})

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Intended safe behavior: Local project commands must NOT mutate user's global HOME directory
	// unless explicitly asked (e.g. via --global flag).
	if env.HomeFileExists(".copilot/mcp-config.json") {
		t.Errorf("BUG REPRODUCED: AgentSync mutated user's global HOME directory (~/.copilot/mcp-config.json) without explicit opt-in!")
	}
}

// 9. Existing User-Authored Guidelines Silently Overwritten
// TestRegression_Guidelines_PreserveExistingUserContent proves that installGuidelines
// wipes user instructions in existing files without demarcation or backup.
func TestRegression_Guidelines_PreserveExistingUserContent(t *testing.T) {
	env := newIntegrationEnv(t)

	userCustomInstructions := "# My Custom Team Instructions\nKeep all tests in internal/test/.\n"
	env.WriteFile("CLAUDE.md", userCustomInstructions)
	env.PopulateStandardAI([]string{"claude-code"})

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	claudeContent := env.ReadFile("CLAUDE.md")
	// Safe behavior: User-authored sections should be preserved or managed with delimiters
	if !strings.Contains(claudeContent, "My Custom Team Instructions") {
		t.Errorf("BUG REPRODUCED: Pre-existing user-authored instructions in CLAUDE.md were completely overwritten and erased!")
	}
}

// TestRegression_Guidelines_ManagedSectionReplacementNoDuplicates verifies that
// repeated synchronization replaces the managed section rather than appending duplicate blocks.
func TestRegression_Guidelines_ManagedSectionReplacementNoDuplicates(t *testing.T) {
	env := newIntegrationEnv(t)

	initialFile := `# Header Instructions
Keep tests clean.

` + agentpkg.ManagedSectionStart + `
# Old Managed Guidelines
Old text that should be replaced.
` + agentpkg.ManagedSectionEnd + `

# Footer Instructions
Always benchmark performance.
`
	env.WriteFile("CLAUDE.md", initialFile)
	env.PopulateStandardAI([]string{"claude-code"})

	// First install: replaces managed block
	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	content1 := env.ReadFile("CLAUDE.md")
	if !strings.Contains(content1, "# Header Instructions") {
		t.Errorf("expected # Header Instructions preserved")
	}
	if !strings.Contains(content1, "# Footer Instructions") {
		t.Errorf("expected # Footer Instructions preserved")
	}
	if strings.Contains(content1, "Old text that should be replaced") {
		t.Errorf("expected old managed content replaced")
	}
	if strings.Count(content1, agentpkg.ManagedSectionStart) != 1 {
		t.Errorf("expected exactly 1 managed start marker, got %d", strings.Count(content1, agentpkg.ManagedSectionStart))
	}
	if strings.Count(content1, agentpkg.ManagedSectionEnd) != 1 {
		t.Errorf("expected exactly 1 managed end marker, got %d", strings.Count(content1, agentpkg.ManagedSectionEnd))
	}

	// Second install: repeated sync should not add another block
	_, _, err = env.RunInstall("-y")
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	content2 := env.ReadFile("CLAUDE.md")
	if content2 != content1 {
		t.Errorf("expected repeated sync to produce identical content without duplicate appends!\nFirst:\n%s\nSecond:\n%s", content1, content2)
	}
}

// 10. Symlinks Inside Skills Dereferenced Rather Than Preserved
// TestRegression_Skills_PreserveSymlinks proves that copyDir converts symlinks into
// regular duplicated files rather than preserving symlinks.
func TestRegression_Skills_PreserveSymlinks(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulateStandardAI([]string{"claude-code"})

	env.WriteFile(".ai/skills/symskill/SKILL.md", "Skill with symlink\n")
	env.WriteFile(".ai/skills/symskill/target.txt", "Target file content\n")

	// Create relative symlink: link.txt -> target.txt
	target := filepath.Join(env.RootDir, ".ai/skills/symskill/target.txt")
	link := filepath.Join(env.RootDir, ".ai/skills/symskill/link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	destLink := filepath.Join(env.RootDir, ".claude/skills/symskill/link.txt")
	fi, err := os.Lstat(destLink)
	if err != nil {
		t.Fatalf("lstat failed: %v", err)
	}

	// Intended behavior: Symlinks should remain symlinks, not dereferenced into normal files
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("BUG REPRODUCED: Symlink in skill was dereferenced into a regular file instead of preserved as a symlink")
	}
}

// 11. Cursor MDC Frontmatter Missing
func TestRegression_Cursor_MDC_Frontmatter(t *testing.T) {
	env := newIntegrationEnv(t)

	env.PopulateStandardAI([]string{"cursor"})

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	content := env.ReadFile(".cursor/rules/agentsync.mdc")
	if !strings.HasPrefix(content, "---") || !strings.Contains(content, "alwaysApply:") {
		t.Errorf("BUG REPRODUCED (Issue 2.2): .cursor/rules/agentsync.mdc is missing required YAML frontmatter (alwaysApply: true)!\nContent:\n%s", content)
	}
}

// 12. Markdown Concatenation Without Blank Lines
func TestRegression_Guidelines_NewlineSeparation(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".ai/guidelines/01.md", "Paragraph in section 1")
	env.WriteFile(".ai/guidelines/02.md", "## Heading in section 2\nParagraph 2\n")
	env.WriteFile(".ai/sync.lock", `agents = ["claude-code"]`+"\n")

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	content := env.ReadFile("CLAUDE.md")
	if !strings.Contains(content, "Paragraph in section 1\n\n## Heading in section 2") {
		t.Errorf("BUG REPRODUCED (Issue 2.1): guidelines joined without blank line separation!\nContent:\n%q", content)
	}
}

// 13. Nested Guidelines Ignored by markdownFiles()
func TestRegression_Guidelines_NestedSubdirectories(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".ai/guidelines/root.md", "# Root Guideline\n")
	env.WriteFile(".ai/guidelines/architecture/system-design.md", "# System Design Architecture Rule\n")
	env.WriteFile(".ai/sync.lock", `agents = ["claude-code"]`+"\n")

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	content := env.ReadFile("CLAUDE.md")
	if !strings.Contains(content, "System Design Architecture Rule") {
		t.Errorf("BUG REPRODUCED (Issue 2.4): nested guideline 'architecture/system-design.md' was ignored by markdownFiles()!\nContent:\n%s", content)
	}
}

func mustGetAgent(id string) agentpkg.Agent {
	a, ok := agentpkg.ByID(id)
	if !ok {
		panic("unknown agent: " + id)
	}
	return a
}
