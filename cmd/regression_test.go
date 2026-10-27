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

// TestRegression_Codex_PreservesCommentsAndFormatting verifies that TOML comments,
// blank lines, inline comments, and unrelated tables are preserved across repeated installations.
func TestRegression_Codex_PreservesCommentsAndFormatting(t *testing.T) {
	env := newIntegrationEnv(t)

	initialContent := `# Project Configuration
# Maintained by Developer

model = "gpt-4o" # active model
temperature = 0.2

# Custom developer section
[custom_section]
key = "important_value"
# internal note
flag = true

# User unmanaged MCP server
[mcp_servers.existing_tool]
command = "echo" # shell command
args = ["hello"]

# Trailing footer section
[footer_section]
enabled = false
`
	env.WriteFile(".codex/config.toml", initialContent)
	env.PopulateStandardAI([]string{"codex"})

	// First installation
	stdout, stderr, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("first install failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	content1 := env.ReadFile(".codex/config.toml")

	// Comments preserved
	expectedComments := []string{
		"# Project Configuration",
		"# Maintained by Developer",
		"# active model",
		"# Custom developer section",
		"# internal note",
		"# User unmanaged MCP server",
		"# shell command",
		"# Trailing footer section",
	}
	for _, c := range expectedComments {
		if !strings.Contains(content1, c) {
			t.Errorf("expected comment %q preserved in .codex/config.toml!\nContent:\n%s", c, content1)
		}
	}

	// Unrelated settings and sections preserved
	expectedElements := []string{
		`model = "gpt-4o"`,
		"temperature = 0.2",
		"[custom_section]",
		`key = "important_value"`,
		"flag = true",
		"[mcp_servers.existing_tool]",
		`command = "echo"`,
		`args = ["hello"]`,
		"[footer_section]",
		"enabled = false",
	}
	for _, el := range expectedElements {
		if !strings.Contains(content1, el) {
			t.Errorf("expected element %q preserved in .codex/config.toml!\nContent:\n%s", el, content1)
		}
	}

	// Managed servers added
	if !strings.Contains(content1, "[mcp_servers.postgres]") {
		t.Errorf("expected managed [mcp_servers.postgres] in .codex/config.toml!\nContent:\n%s", content1)
	}

	// Repeated installation must be completely idempotent and produce byte-for-byte identical content
	stdout2, stderr2, err2 := env.RunInstall("-y")
	if err2 != nil {
		t.Fatalf("second install failed: %v\nStderr: %s\nStdout: %s", err2, stderr2, stdout2)
	}

	content2 := env.ReadFile(".codex/config.toml")
	if content2 != content1 {
		t.Errorf("expected repeated install to produce byte-for-byte identical TOML!\nFirst:\n%s\nSecond:\n%s", content1, content2)
	}
}

// TestRegression_InvalidTOML_MustNotDestroyCorruptedContent verifies that
// malformed existing TOML is rejected with a meaningful error and preserved byte-for-byte without modification.
func TestRegression_InvalidTOML_MustNotDestroyCorruptedContent(t *testing.T) {
	env := newIntegrationEnv(t)

	corruptedTOML := `# Malformed TOML file
model = "unclosed string without end quote
[mcp_servers]
`
	env.WriteFile(".codex/config.toml", corruptedTOML)
	env.PopulateStandardAI([]string{"codex"})

	stdout, stderr, err := env.RunInstall("-y")
	_ = stderr
	_ = err

	// Verify a meaningful error is reported
	if !strings.Contains(stdout, "cannot parse existing TOML config") && !strings.Contains(stdout, "✗") {
		t.Errorf("expected meaningful error reported for malformed TOML in stdout, got:\n%s", stdout)
	}

	// Verify the original file remains byte-for-byte unchanged
	currentContent := env.ReadFile(".codex/config.toml")
	if currentContent != corruptedTOML {
		t.Errorf("SAFETY VIOLATION: Malformed .codex/config.toml was modified or destroyed!\nWant:\n%q\nGot:\n%q", corruptedTOML, currentContent)
	}

	// Repeated run must also preserve the file byte-for-byte
	stdout2, _, _ := env.RunInstall("-y")
	if !strings.Contains(stdout2, "cannot parse existing TOML config") && !strings.Contains(stdout2, "✗") {
		t.Errorf("expected meaningful error reported on second run for malformed TOML, got:\n%s", stdout2)
	}
	currentContent2 := env.ReadFile(".codex/config.toml")
	if currentContent2 != corruptedTOML {
		t.Errorf("SAFETY VIOLATION: Malformed .codex/config.toml was modified on repeated run!\nWant:\n%q\nGot:\n%q", corruptedTOML, currentContent2)
	}
}

// 2. Project Boundary: Global Copilot Home Config Ignored
// TestRegression_Detect_CopilotHomeTildeExpansion verifies that project-scoped detection
// does NOT read or import global ~/.copilot/mcp-config.json outside the project root.
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

	for _, srv := range detection.MCPServers {
		if srv.Name == "copilot_global_mcp" {
			t.Errorf("Project-scoped detect must NOT scan or import global ~/.copilot/mcp-config.json from user HOME!")
		}
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
// TestRegression_InvalidJSON_MustNotDestroyCorruptedContent verifies that
// when target JSON contains a syntax error, install reports a clear error and preserves the damaged file byte-for-byte.
func TestRegression_InvalidJSON_MustNotDestroyCorruptedContent(t *testing.T) {
	env := newIntegrationEnv(t)

	corruptedOriginal := `{"mcpServers": { "important_server": { "command": "tool" } /* UNFINISHED EDIT`
	env.WriteFile(".mcp.json", corruptedOriginal)
	env.PopulateStandardAI([]string{"claude-code"})

	stdout, stderr, err := env.RunInstall("-y")
	_ = stderr
	_ = err

	// Verify meaningful error is reported
	if !strings.Contains(stdout, "invalid JSON config") && !strings.Contains(stdout, "cannot parse existing JSON") && !strings.Contains(stdout, "✗") {
		t.Errorf("expected meaningful error reported for invalid JSON in stdout, got:\n%s", stdout)
	}

	currentContent := env.ReadFile(".mcp.json")

	// Safe behavior: AgentSync must NOT silently wipe user's damaged file.
	if currentContent != corruptedOriginal {
		t.Errorf("SAFETY VIOLATION: AgentSync modified or destroyed corrupted .mcp.json!\nWant:\n%q\nGot:\n%q", corruptedOriginal, currentContent)
	}

	// Repeated run must also preserve the file byte-for-byte
	stdout2, _, _ := env.RunInstall("-y")
	if !strings.Contains(stdout2, "invalid JSON config") && !strings.Contains(stdout2, "cannot parse existing JSON") && !strings.Contains(stdout2, "✗") {
		t.Errorf("expected meaningful error reported on repeated run for invalid JSON, got:\n%s", stdout2)
	}
	currentContent2 := env.ReadFile(".mcp.json")
	if currentContent2 != corruptedOriginal {
		t.Errorf("SAFETY VIOLATION: AgentSync modified corrupted .mcp.json on repeated run!\nWant:\n%q\nGot:\n%q", corruptedOriginal, currentContent2)
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

// TestRegression_Guidelines_MalformedMarkersPreserveFileAndReportError verifies that
// malformed guideline markers (missing, reversed, duplicate, nested) return a clear error
// and preserve the original file byte-for-byte without appending or overwriting.
func TestRegression_Guidelines_MalformedMarkersPreserveFileAndReportError(t *testing.T) {
	testCases := []struct {
		name        string
		content     string
		errFragment string
	}{
		{
			name:        "missing end marker",
			content:     "# Header\n" + agentpkg.ManagedSectionStart + "\nUnfinished managed block\n",
			errFragment: "missing counterpart managed marker",
		},
		{
			name:        "missing begin marker",
			content:     "# Header\nOrphaned managed block\n" + agentpkg.ManagedSectionEnd + "\n",
			errFragment: "missing counterpart managed marker",
		},
		{
			name:        "reversed markers",
			content:     "# Header\n" + agentpkg.ManagedSectionEnd + "\nReversed block\n" + agentpkg.ManagedSectionStart + "\n",
			errFragment: "end marker appears before begin marker",
		},
		{
			name: "duplicate begin markers",
			content: "# Header\n" + agentpkg.ManagedSectionStart + "\nFirst\n" +
				agentpkg.ManagedSectionStart + "\nSecond\n" + agentpkg.ManagedSectionEnd + "\n",
			errFragment: "duplicate managed markers",
		},
		{
			name: "duplicate end markers",
			content: "# Header\n" + agentpkg.ManagedSectionStart + "\nContent\n" +
				agentpkg.ManagedSectionEnd + "\n" + agentpkg.ManagedSectionEnd + "\n",
			errFragment: "duplicate managed markers",
		},
		{
			name: "nested markers",
			content: agentpkg.ManagedSectionStart + "\n" + agentpkg.ManagedSectionStart +
				"\nNested\n" + agentpkg.ManagedSectionEnd + "\n" + agentpkg.ManagedSectionEnd + "\n",
			errFragment: "duplicate managed markers",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := newIntegrationEnv(t)
			env.WriteFile("CLAUDE.md", tc.content)
			env.PopulateStandardAI([]string{"claude-code"})

			stdout, stderr, err := env.RunInstall("-y")
			_ = stderr
			_ = err

			// 1. Verify a meaningful error is reported in stdout
			if !strings.Contains(stdout, tc.errFragment) {
				t.Errorf("expected error containing %q in stdout, got:\n%s", tc.errFragment, stdout)
			}
			if !strings.Contains(stdout, "✗") {
				t.Errorf("expected error symbol ✗ in stdout, got:\n%s", stdout)
			}

			// 2. Verify file is byte-for-byte preserved
			currentContent := env.ReadFile("CLAUDE.md")
			if currentContent != tc.content {
				t.Errorf("SAFETY VIOLATION: Malformed CLAUDE.md was modified or overwritten!\nWant:\n%q\nGot:\n%q", tc.content, currentContent)
			}

			// 3. Repeated install also preserves the file byte-for-byte
			stdout2, _, _ := env.RunInstall("-y")
			if !strings.Contains(stdout2, tc.errFragment) {
				t.Errorf("expected error containing %q on repeated install, got:\n%s", tc.errFragment, stdout2)
			}
			currentContent2 := env.ReadFile("CLAUDE.md")
			if currentContent2 != tc.content {
				t.Errorf("SAFETY VIOLATION: Malformed CLAUDE.md was modified on repeated install!\nWant:\n%q\nGot:\n%q", tc.content, currentContent2)
			}
		})
	}
}

// TestRegression_Guidelines_PreserveExactUserWhitespaceAndFormatting verifies that
// all bytes outside the managed markers, including whitespace, indentation, tabs, and newlines,
// are preserved exactly without stripping or modification.
func TestRegression_Guidelines_PreserveExactUserWhitespaceAndFormatting(t *testing.T) {
	env := newIntegrationEnv(t)

	// Pre-existing user content with exact whitespace, multiple newlines, and tabs
	userBefore := "# Team Rules\n\n   Indented paragraph with spaces.  \n\n\n"
	userAfter := "\n\n## Footer Notes\n\tTab-indented text\n   Trailing spaces on line   \n"
	initialContent := userBefore + agentpkg.ManagedSectionStart + "\nOld guidelines\n" + agentpkg.ManagedSectionEnd + userAfter

	env.WriteFile("CLAUDE.md", initialContent)
	env.PopulateStandardAI([]string{"claude-code"})

	// First installation
	stdout, stderr, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	content1 := env.ReadFile("CLAUDE.md")

	// Verify exact byte preservation before start marker
	if !strings.HasPrefix(content1, userBefore) {
		t.Fatalf("expected exact prefix byte preservation:\nWant:\n%q\nGot:\n%q", userBefore, content1[:len(userBefore)])
	}

	// Verify exact byte preservation after end marker
	if !strings.HasSuffix(content1, userAfter) {
		t.Fatalf("expected exact suffix byte preservation:\nWant:\n%q\nGot:\n%q", userAfter, content1[len(content1)-len(userAfter):])
	}

	// Verify managed section updated
	if !strings.Contains(content1, "Project Core Guidelines") {
		t.Fatalf("expected synced guidelines inside managed block")
	}

	// Second install: repeated sync must produce 100% byte-for-byte identical file
	stdout2, stderr2, err2 := env.RunInstall("-y")
	if err2 != nil {
		t.Fatalf("second install failed: %v\nStderr: %s\nStdout: %s", err2, stderr2, stdout2)
	}

	content2 := env.ReadFile("CLAUDE.md")
	if content2 != content1 {
		t.Fatalf("expected repeated install to be byte-for-byte identical!\nFirst:\n%q\nSecond:\n%q", content1, content2)
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
