package agent

import (
	"os"
	"strings"
	"testing"
)

func TestInstallMCPWritesOpenCodeShape(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]
env = { API_KEY = "${MY_KEY}" }
`)

	target := Agent{
		ID:             "opencode",
		Name:           "OpenCode",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      "opencode.json",
		MCPConfigs:     nil,
		MCPFormat:      MCPFormatJSON,
		SkillsSupported: true,
	}

	result := installMCP(target)
	if result.Status != StepStatusOK {
		t.Fatalf("expected OpenCode MCP result ok, got %+v", result)
	}

	assertFileContains(t, "opencode.json", `"mcp"`)
	assertFileContains(t, "opencode.json", `"type": "local"`)
	assertFileContains(t, "opencode.json", `"environment"`)
	assertFileContains(t, "opencode.json", `"API_KEY"`)
	assertFileContains(t, "opencode.json", `"command"`)
}

func TestInstallMCPWritesOpenCodeRemoteShape(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
type = "remote"
url = "https://mcp.example.com/mcp"

[servers.postgres.headers]
Authorization = "Bearer MY_API_KEY"

[servers.postgres.oauth]
clientId = "{env:CLIENT_ID}"
clientSecret = "{env:CLIENT_SECRET}"
scope = "tools:read"
`)

	target := Agent{
		ID:             "opencode",
		Name:           "OpenCode",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      "opencode.json",
		MCPConfigs:     nil,
		MCPFormat:      MCPFormatJSON,
		SkillsSupported: true,
	}

	result := installMCP(target)
	if result.Status != StepStatusOK {
		t.Fatalf("expected OpenCode MCP result ok, got %+v", result)
	}

	assertFileContains(t, "opencode.json", `"mcp"`)
	assertFileContains(t, "opencode.json", `"type": "remote"`)
	assertFileContains(t, "opencode.json", `"url":`)
	assertFileContains(t, "opencode.json", `"headers"`)
	assertFileContains(t, "opencode.json", `"Authorization"`)
	assertFileContains(t, "opencode.json", `"oauth"`)
	assertFileContains(t, "opencode.json", `"clientId"`)
	assertFileContains(t, "opencode.json", `"scope"`)
}

func TestInstallMCPWritesCopilotToMultipleDestinations(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "npx"
args = ["-y", "pkg"]
env = { API_KEY = "value" }
`)

	dest1 := "copilot-vscode.json"
	dest2 := "copilot-cli.json"
	target := Agent{
		ID:             "github-copilot",
		Name:           "Copilot",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".github/skills/",
		MCPConfig:      dest1,
		MCPConfigs:     []string{dest2},
		MCPFormat:      MCPFormatJSON,
		SkillsSupported: true,
	}

	result := installMCP(target)
	if result.Status != StepStatusOK {
		t.Fatalf("expected Copilot MCP result ok, got %+v", result)
	}

	for _, dest := range []string{dest1, dest2} {
		data, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("read %s: %v", dest, err)
		}
		text := string(data)
		if !strings.Contains(text, `"servers"`) {
			t.Fatalf("%s expected root servers key, got %s", dest, text)
		}
		if !strings.Contains(text, `"type": "stdio"`) {
			t.Fatalf("%s expected type stdio, got %s", dest, text)
		}
		if !strings.Contains(text, `"env"`) {
			t.Fatalf("%s expected env key, got %s", dest, text)
		}
		if !strings.Contains(text, `"API_KEY"`) {
			t.Fatalf("%s expected API_KEY value, got %s", dest, text)
		}
	}
}

func TestInstallMCPWritesJunieMCPPath(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "npx"
args = ["-y", "pkg"]
`)

	target := Agent{
		ID:             "junie",
		Name:           "Junie",
		GuidelinesFile: ".junie/guidelines.md",
		SkillsDir:      ".junie/skills/",
		MCPConfig:      ".junie/mcp/mcp.json",
		MCPConfigs:     nil,
		MCPFormat:      MCPFormatJSON,
		SkillsSupported: true,
	}

	result := installMCP(target)
	if result.Status != StepStatusOK {
		t.Fatalf("expected Junie MCP result ok, got %+v", result)
	}

	assertFileContains(t, ".junie/mcp/mcp.json", `"mcpServers"`)
}

