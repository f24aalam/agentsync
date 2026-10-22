package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/f24aalam/agentsync/internal/config"
)

func TestInstallGuidelinesMergesAlphabetically(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/guidelines/b.md", "B")
	mustWriteFile(t, ".ai/guidelines/a.md", "A")
	mustWriteFile(t, ".ai/guidelines/ignore.txt", "X")

	target, _ := ByID("claude-code")
	result := installGuidelines(target, ".", false)
	if result.Status != StepStatusOK {
		t.Fatalf("expected guidelines install ok, got %+v", result)
	}

	data, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatalf("read merged guidelines: %v", err)
	}

	want := ManagedSectionStart + "\nA\n\nB\n" + ManagedSectionEnd + "\n"
	if string(data) != want {
		t.Fatalf("expected alphabetical merge in managed block, got %q", string(data))
	}
}

func TestInstallGuidelinesCursorUsesAgentsyncFile(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/guidelines/core.md", "content")

	target, _ := ByID("cursor")
	result := installGuidelines(target, ".", false)
	if result.Target != ".cursor/rules/agentsync.mdc" {
		t.Fatalf("expected cursor target path, got %q", result.Target)
	}

	assertFileContains(t, ".cursor/rules/agentsync.mdc", "content")
}

func TestInstallSkillsCopiesNestedFiles(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/skills/example-skill/SKILL.md", "skill")
	mustWriteFile(t, ".ai/skills/example-skill/assets/data.txt", "nested")

	target, _ := ByID("codex")
	result := installSkills(target, ".")
	if result.Status != StepStatusOK {
		t.Fatalf("expected skills install ok, got %+v", result)
	}

	assertFileContains(t, ".agents/skills/example-skill/SKILL.md", "skill")
	assertFileContains(t, ".agents/skills/example-skill/assets/data.txt", "nested")
}

func TestInstallMCPSkipsWhenMissing(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	target, _ := ByID("claude-code")
	result := installMCP(target, ".", false)
	if result.Status != StepStatusSkipped {
		t.Fatalf("expected skipped MCP result, got %+v", result)
	}
}

func TestInstallMCPWritesJSONAndTOML(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]
`)

	jsonAgent, _ := ByID("claude-code")
	tomlAgent, _ := ByID("codex")

	jsonResult := installMCP(jsonAgent, ".", false)
	if jsonResult.Status != StepStatusOK {
		t.Fatalf("expected json MCP result ok, got %+v", jsonResult)
	}
	tomlResult := installMCP(tomlAgent, ".", false)
	if tomlResult.Status != StepStatusOK {
		t.Fatalf("expected toml MCP result ok, got %+v", tomlResult)
	}

	assertFileContains(t, ".mcp.json", `"mcpServers"`)
	assertFileContains(t, ".codex/config.toml", "[mcp_servers.postgres]")
}

func TestInstallAggregatesStepErrors(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/guidelines/core.md", "content")
	target := Agent{
		ID:             "broken",
		Name:           "Broken",
		GuidelinesFile: ".",
		SkillsDir:      ".skills",
		MCPConfig:      ".mcp.json",
		MCPFormat:      MCPFormatJSON,
	}

	sk := StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusSkipped}
	result := InstallAgent(target, ".", sk, NewInstallPlan())
	if result.Succeeded() {
		t.Fatalf("expected failed install result")
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	return wd
}

func mustChdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if !strings.Contains(string(data), want) {
		t.Fatalf("expected %s to contain %q, got %q", path, want, string(data))
	}
}

func TestInjectManagedGuidelines_MalformedMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing end marker",
			content: "# Header\n" + ManagedSectionStart + "\nManaged content without end",
			wantErr: "missing counterpart",
		},
		{
			name:    "missing begin marker",
			content: "# Header\nManaged content without start\n" + ManagedSectionEnd,
			wantErr: "missing counterpart",
		},
		{
			name:    "reversed markers",
			content: "# Header\n" + ManagedSectionEnd + "\nInside\n" + ManagedSectionStart,
			wantErr: "end marker appears before begin marker",
		},
		{
			name:    "duplicate begin markers",
			content: ManagedSectionStart + "\nFirst\n" + ManagedSectionStart + "\nSecond\n" + ManagedSectionEnd,
			wantErr: "duplicate managed markers",
		},
		{
			name:    "duplicate end markers",
			content: ManagedSectionStart + "\nContent\n" + ManagedSectionEnd + "\n" + ManagedSectionEnd,
			wantErr: "duplicate managed markers",
		},
		{
			name:    "nested markers",
			content: ManagedSectionStart + "\n" + ManagedSectionStart + "\nInner\n" + ManagedSectionEnd + "\n" + ManagedSectionEnd,
			wantErr: "duplicate managed markers",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := injectManagedGuidelines(tc.content, "new managed content")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestInjectManagedGuidelines_PreservesExactUserBytes(t *testing.T) {
	// Pre-existing user content with exact whitespace, indentation, and newlines
	userBefore := "# Team Rules\n\n   Indented paragraph with spaces.  \n\n\n"
	userAfter := "\n\n## Footer Notes\n\tTab-indented text\n   Trailing spaces on line   \n"
	initialContent := userBefore + ManagedSectionStart + "\nOld guidelines\n" + ManagedSectionEnd + userAfter

	newManaged := "New managed guidelines v1"
	updated, err := injectManagedGuidelines(initialContent, newManaged)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify user content before markers is preserved byte-for-byte
	if !strings.HasPrefix(updated, userBefore) {
		t.Fatalf("expected exact prefix byte preservation:\nwant: %q\ngot:  %q", userBefore, updated[:len(userBefore)])
	}

	// Verify user content after markers is preserved byte-for-byte
	if !strings.HasSuffix(updated, userAfter) {
		t.Fatalf("expected exact suffix byte preservation:\nwant: %q\ngot:  %q", userAfter, updated[len(updated)-len(userAfter):])
	}

	// Repeated call with same managed content must be byte-for-byte identical
	repeated, err := injectManagedGuidelines(updated, newManaged)
	if err != nil {
		t.Fatalf("unexpected error on repeated inject: %v", err)
	}
	if repeated != updated {
		t.Fatalf("expected idempotent output on repeated run:\nFirst:\n%q\nSecond:\n%q", updated, repeated)
	}
}

func TestUpdateTomlMCPServers_PreservesCommentsAndFormatting(t *testing.T) {
	initialTOML := `# Codex Global Configuration
# Maintained by DevOps

model = "gpt-4o" # default model
temperature = 0.2

# Custom developer tools section
[custom_section]
key = "important_value"
# internal comment inside custom_section
flag = true

# User unmanaged MCP server
[mcp_servers.my_custom_tool]
command = "my-tool"
args = ["--verbose"]

# Old managed server
[mcp_servers.postgres]
command = "old-pg"

# Trailing footer section
[footer_section]
enabled = false
`

	servers := map[string]config.MCPServer{
		"postgres": {
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-postgres"},
			Env:     map[string]string{"PORT": "5432"},
		},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("updateTomlMCPServers failed: %v", err)
	}

	// 1. Comments preserved
	expectedComments := []string{
		"# Codex Global Configuration",
		"# Maintained by DevOps",
		"# default model",
		"# Custom developer tools section",
		"# internal comment inside custom_section",
		"# User unmanaged MCP server",
		"# Trailing footer section",
	}
	for _, c := range expectedComments {
		if !strings.Contains(updated, c) {
			t.Errorf("expected comment %q preserved in output:\n%s", c, updated)
		}
	}

	// 2. Unmanaged settings and sections preserved
	expectedSections := []string{
		`model = "gpt-4o"`,
		"temperature = 0.2",
		"[custom_section]",
		`key = "important_value"`,
		"flag = true",
		"[mcp_servers.my_custom_tool]",
		`command = "my-tool"`,
		`args = ["--verbose"]`,
		"[footer_section]",
		"enabled = false",
	}
	for _, s := range expectedSections {
		if !strings.Contains(updated, s) {
			t.Errorf("expected section %q preserved in output:\n%s", s, updated)
		}
	}

	// 3. Managed server updated
	if strings.Contains(updated, `command = "old-pg"`) {
		t.Errorf("expected old managed server command replaced")
	}
	if !strings.Contains(updated, "[mcp_servers.postgres]") {
		t.Errorf("expected [mcp_servers.postgres] in output")
	}
	if !strings.Contains(updated, `command = "npx"`) {
		t.Errorf("expected command = \"npx\" in output")
	}
	if !strings.Contains(updated, `[mcp_servers.postgres.env]`) || !strings.Contains(updated, `PORT = "5432"`) {
		t.Errorf("expected postgres env in output:\n%s", updated)
	}

	// 4. Repeated update is byte-for-byte identical
	repeated, err := updateTomlMCPServers(updated, servers)
	if err != nil {
		t.Fatalf("repeated update failed: %v", err)
	}
	if repeated != updated {
		t.Fatalf("expected idempotent repeated update:\nFirst:\n%s\nSecond:\n%s", updated, repeated)
	}
}

func TestUpdateTomlMCPServers_RejectsMalformedTOML(t *testing.T) {
	malformed := `# Corrupted TOML
model = "unclosed string without closing quote
[mcp_servers]
`
	servers := map[string]config.MCPServer{
		"postgres": {Command: "npx"},
	}

	_, err := updateTomlMCPServers(malformed, servers)
	if err == nil {
		t.Fatalf("expected error on malformed TOML, got nil")
	}
	if !strings.Contains(err.Error(), "cannot parse existing TOML config") {
		t.Fatalf("expected 'cannot parse existing TOML config' in error, got: %v", err)
	}
}
