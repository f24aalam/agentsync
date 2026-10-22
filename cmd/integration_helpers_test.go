package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/f24aalam/agentsync/internal/detect"
	"github.com/spf13/cobra"
)

// integrationEnv manages an isolated test environment with temporary root and HOME directories.
type integrationEnv struct {
	T       *testing.T
	RootDir string
	HomeDir string
	OrigWd  string
}

// newIntegrationEnv initializes an isolated test environment.
// It overrides HOME so user-level configs (e.g. Copilot global MCP) never touch the developer's system.
func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	rootDir := t.TempDir()
	homeDir := t.TempDir()

	t.Setenv("HOME", homeDir)

	if err := os.Chdir(rootDir); err != nil {
		t.Fatalf("failed to chdir to temp root %s: %v", rootDir, err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(origWd)
	})

	return &integrationEnv{
		T:       t,
		RootDir: rootDir,
		HomeDir: homeDir,
		OrigWd:  origWd,
	}
}

// WriteFile writes content to a path relative to the test root directory.
func (e *integrationEnv) WriteFile(relPath, content string) {
	e.T.Helper()
	absPath := filepath.Join(e.RootDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		e.T.Fatalf("failed to mkdir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(absPath, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		e.T.Fatalf("failed to write file %s: %v", relPath, err)
	}
}

// WriteHomeFile writes content to a path relative to the fake HOME directory.
func (e *integrationEnv) WriteHomeFile(relPath, content string) {
	e.T.Helper()
	absPath := filepath.Join(e.HomeDir, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		e.T.Fatalf("failed to mkdir for home file %s: %v", relPath, err)
	}
	if err := os.WriteFile(absPath, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		e.T.Fatalf("failed to write home file %s: %v", relPath, err)
	}
}

// ReadFile reads content from a path relative to the test root directory.
func (e *integrationEnv) ReadFile(relPath string) string {
	e.T.Helper()
	data, err := os.ReadFile(filepath.Join(e.RootDir, relPath))
	if err != nil {
		e.T.Fatalf("failed to read file %s: %v", relPath, err)
	}
	return string(data)
}

// ReadHomeFile reads content from a path relative to the fake HOME directory.
func (e *integrationEnv) ReadHomeFile(relPath string) string {
	e.T.Helper()
	data, err := os.ReadFile(filepath.Join(e.HomeDir, relPath))
	if err != nil {
		e.T.Fatalf("failed to read home file %s: %v", relPath, err)
	}
	return string(data)
}

// FileExists returns true if the file exists relative to the test root.
func (e *integrationEnv) FileExists(relPath string) bool {
	e.T.Helper()
	_, err := os.Stat(filepath.Join(e.RootDir, relPath))
	return err == nil
}

// HomeFileExists returns true if the file exists relative to the fake HOME.
func (e *integrationEnv) HomeFileExists(relPath string) bool {
	e.T.Helper()
	_, err := os.Stat(filepath.Join(e.HomeDir, relPath))
	return err == nil
}

// AssertFileExists asserts that a path exists in the test root.
func (e *integrationEnv) AssertFileExists(relPath string) {
	e.T.Helper()
	if !e.FileExists(relPath) {
		e.T.Fatalf("expected file %s to exist, but it was not found", relPath)
	}
}

// AssertFileNotExists asserts that a path does NOT exist in the test root.
func (e *integrationEnv) AssertFileNotExists(relPath string) {
	e.T.Helper()
	if e.FileExists(relPath) {
		e.T.Fatalf("expected file %s NOT to exist, but it was found", relPath)
	}
}

// AssertFileContains checks if a file's content contains the target string.
func (e *integrationEnv) AssertFileContains(relPath, want string) {
	e.T.Helper()
	content := e.ReadFile(relPath)
	if !strings.Contains(content, want) {
		e.T.Fatalf("file %s does not contain expected substring %q\nFull Content:\n%s", relPath, want, content)
	}
}

// AssertFileNotContains checks if a file's content does NOT contain the target string.
func (e *integrationEnv) AssertFileNotContains(relPath, unwanted string) {
	e.T.Helper()
	content := e.ReadFile(relPath)
	if strings.Contains(content, unwanted) {
		e.T.Fatalf("file %s contains unwanted substring %q\nFull Content:\n%s", relPath, unwanted, content)
	}
}

// RunInstall executes `agentsync install` with the provided arguments.
func (e *integrationEnv) RunInstall(args ...string) (string, string, error) {
	e.T.Helper()
	cmd := newInstallCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// RunList executes `agentsync list`.
func (e *integrationEnv) RunList() (string, string, error) {
	e.T.Helper()
	cmd := newListCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// RunInit executes `agentsync init` with mocked wizard hooks to allow deterministic testing.
func (e *integrationEnv) RunInit(
	overwriteConfirm bool,
	projectName string,
	plan importPlan,
	answers initAnswers,
) (string, string, error) {
	e.T.Helper()

	restoreOverwrite := runOverwriteConfirm
	restoreProject := runProjectNamePrompt
	restoreImport := runImportFlow
	restoreSurvey := runInitSurvey
	defer func() {
		runOverwriteConfirm = restoreOverwrite
		runProjectNamePrompt = restoreProject
		runImportFlow = restoreImport
		runInitSurvey = restoreSurvey
	}()

	runOverwriteConfirm = func(cmd *cobra.Command) (bool, error) {
		return overwriteConfirm, nil
	}

	runProjectNamePrompt = func(cmd *cobra.Command, defaultName string) (string, error) {
		if projectName != "" {
			return projectName, nil
		}
		return defaultName, nil
	}

	runImportFlow = func(cmd *cobra.Command, detection detect.ProjectDetection) (importPlan, error) {
		return plan, nil
	}

	runInitSurvey = func(cmd *cobra.Command, pName string, askG, askS, askM bool) (initAnswers, error) {
		return answers, nil
	}

	cmd := newInitCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// PopulateStandardAI sets up a standard, valid .ai directory with guidelines, skills, MCP, and lockfile.
func (e *integrationEnv) PopulateStandardAI(agents []string) {
	e.T.Helper()

	e.WriteFile(".ai/guidelines/01-core.md", "# Project Core Guidelines\nFollow clean architecture and write unit tests.\n")
	e.WriteFile(".ai/guidelines/02-style.md", "# Coding Style Guidelines\nUse standard formatting and idiomatic constructs.\n")

	e.WriteFile(".ai/skills/code-review/SKILL.md", `---
name: code-review
description: Comprehensive code review checklist
---
# Code Review Skill
Check error handling and edge cases.
`)
	e.WriteFile(".ai/skills/code-review/scripts/review.sh", "#!/bin/bash\necho 'Running review script'\n")

	e.WriteFile(".ai/mcp.toml", `
[servers.postgres]
type = "local"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]
env = { DATABASE_URL = "${DATABASE_URL}" }

[servers.filesystem]
type = "local"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
`)

	var quoted []string
	for _, a := range agents {
		quoted = append(quoted, `"`+a+`"`)
	}
	e.WriteFile(".ai/sync.lock", `agents = [`+strings.Join(quoted, ", ")+`]`+"\n")
}

// PopulatePreExistingConfigs sets up pre-existing, user-owned configuration files across all agents.
// Some contain unrelated user-owned settings that must never be wiped or corrupted.
func (e *integrationEnv) PopulatePreExistingConfigs() {
	e.T.Helper()

	// Codex with custom model configuration outside [mcp_servers]
	e.WriteFile(".codex/config.toml", `# User-owned Codex configuration
model = "gpt-4o"
temperature = 0.2
system_instructions = "Be concise and accurate"

[editor]
tab_size = 4
auto_save = true

[mcp_servers.pre_existing_codex]
command = "existing-codex-tool"
args = ["--local"]
`)

	// OpenCode with custom user properties outside "mcp"
	e.WriteFile("opencode.json", `{
  "$schema": "https://opencode.ai/config.json",
  "theme": "dark",
  "developer_notes": "Important local settings",
  "mcp": {
    "user_local_tool": {
      "type": "local",
      "command": ["custom-cli", "serve"],
      "enabled": true
    }
  }
}`)

	// Claude Code with pre-existing .mcp.json
	e.WriteFile(".mcp.json", `{
  "custom_setting": true,
  "mcpServers": {
    "pre_existing_claude": {
      "command": "custom-claude-mcp",
      "args": ["serve"]
    }
  }
}`)

	// Cursor with pre-existing rules
	e.WriteFile(".cursor/rules/user-rule.mdc", `---
description: Custom user rule
globs: *.go
alwaysApply: true
---
# User Rule
Always write benchmarks.
`)

	// GitHub Copilot project-level MCP
	e.WriteFile(".vscode/mcp.json", `{
  "servers": {
    "vscode_copilot_tool": {
      "type": "stdio",
      "command": "vscode-tool"
    }
  }
}`)

	// GitHub Copilot global MCP in home directory
	e.WriteHomeFile(".copilot/mcp-config.json", `{
  "servers": {
    "global_copilot_tool": {
      "type": "stdio",
      "command": "global-tool",
      "args": ["--daemon"]
    }
  }
}`)
}
