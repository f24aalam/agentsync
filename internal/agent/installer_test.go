package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	if string(data) != "A\nB" {
		t.Fatalf("expected alphabetical merge, got %q", string(data))
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
