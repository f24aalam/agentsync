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

func TestInitAbortsWithoutWritesWhenReinitCancelled(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	if err := os.MkdirAll(".ai/guidelines", 0o755); err != nil {
		t.Fatalf("mkdir .ai/guidelines: %v", err)
	}
	if err := os.WriteFile(".ai/guidelines/custom.md", []byte("# Custom Guidelines"), 0o644); err != nil {
		t.Fatalf("write custom guideline: %v", err)
	}
	if err := os.WriteFile(".ai/sync.lock", []byte(`agents = ["codex"]`+"\n"), 0o644); err != nil {
		t.Fatalf("write existing lockfile: %v", err)
	}

	restoreReinit := runReinitSurvey
	t.Cleanup(func() {
		runReinitSurvey = restoreReinit
	})

	runReinitSurvey = func(cmd *cobra.Command, currentAgents []string) (reinitAnswers, error) {
		return reinitAnswers{}, ErrUserAborted
	}

	command := newInitCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("init command returned error: %v", err)
	}

	// Canonical guidelines must not be modified or deleted
	assertFileContains(t, ".ai/guidelines/custom.md", "# Custom Guidelines")
	assertFileContains(t, ".ai/sync.lock", `agents = ["codex"]`)

	if !strings.Contains(stdout.String(), "Initialization cancelled") {
		t.Fatalf("expected cancellation message, got %q", stdout.String())
	}
}

func TestInitExistingProjectUpdatesAgentsAndPreservesCanonicalContent(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	if err := os.MkdirAll(".ai/guidelines", 0o755); err != nil {
		t.Fatalf("mkdir .ai/guidelines: %v", err)
	}
	if err := os.WriteFile(".ai/guidelines/custom.md", []byte("# Custom Guidelines"), 0o644); err != nil {
		t.Fatalf("write custom guideline: %v", err)
	}
	if err := os.WriteFile(".ai/sync.lock", []byte(`agents = ["codex"]`+"\n"), 0o644); err != nil {
		t.Fatalf("write existing lockfile: %v", err)
	}

	restoreReinit := runReinitSurvey
	t.Cleanup(func() {
		runReinitSurvey = restoreReinit
	})

	runReinitSurvey = func(cmd *cobra.Command, currentAgents []string) (reinitAnswers, error) {
		if len(currentAgents) != 1 || currentAgents[0] != "codex" {
			t.Fatalf("expected currentAgents to contain codex, got: %v", currentAgents)
		}
		return reinitAnswers{
			Agents:       []string{"codex", "claude-code"},
			AddGitignore: true,
		}, nil
	}

	command := newInitCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("init command returned error: %v", err)
	}

	// Canonical guidelines must be preserved untouched
	assertFileContains(t, ".ai/guidelines/custom.md", "# Custom Guidelines")
	// Lockfile must be updated with new agents
	assertFileContains(t, ".ai/sync.lock", "codex")
	assertFileContains(t, ".ai/sync.lock", "claude-code")
	// Gitignore updated
	assertFileContains(t, ".gitignore", "CLAUDE.md")
	assertFileContains(t, ".gitignore", "AGENTS.md")

	if !strings.Contains(stdout.String(), "AgentSync already initialized") {
		t.Fatalf("expected initialized header in stdout, got %q", stdout.String())
	}
}

func TestInitCreatesScaffoldAndGitignore(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	restoreProject := runProjectNamePrompt
	restoreImport := runImportFlow
	restoreSurvey := runInitSurvey

	t.Cleanup(func() {
		runProjectNamePrompt = restoreProject
		runImportFlow = restoreImport
		runInitSurvey = restoreSurvey
	})

	runProjectNamePrompt = func(cmd *cobra.Command, defaultName string) (string, error) {
		if defaultName != filepath.Base(tempDir) {
			t.Fatalf("expected default name %q, got %q", filepath.Base(tempDir), defaultName)
		}

		return "AgentSync", nil
	}

	runImportFlow = func(cmd *cobra.Command, detection detect.ProjectDetection) (importPlan, error) {
		return importPlan{}, nil
	}

	runInitSurvey = func(cmd *cobra.Command, projectName string, askGuidelines bool, askSampleSkill bool, askMCP bool) (initAnswers, error) {
		return initAnswers{
			ProjectName:    "AgentSync",
			AddGuidelines:  true,
			AddSampleSkill: true,
			AddMCPConfig:   true,
			Agents:         []string{"cursor", "codex"},
			AddGitignore:   true,
		}, nil
	}

	command := newInitCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("init command returned error: %v", err)
	}

	assertFileContains(t, ".ai/guidelines/core.md", "# AgentSync Guidelines")
	assertFileContains(t, ".ai/skills/example-skill/SKILL.md", "name: example-skill")
	assertFileContains(t, ".ai/mcp.toml", "# Example MCP servers.")
	assertFileContains(t, ".ai/sync.lock", `agents = ["cursor", "codex"]`)
	assertFileContains(t, ".gitignore", ".cursor/rules/*.mdc")
	assertFileContains(t, ".gitignore", ".agents/skills/")
	assertFileContains(t, ".gitignore", "AGENTS.md")

	output := stdout.String()
	if !strings.Contains(output, "Creating .ai/") {
		t.Fatalf("expected summary header, got %q", output)
	}

	if !strings.Contains(output, ".gitignore") {
		t.Fatalf("expected gitignore to be included in summary, got %q", output)
	}
}

func TestInitAllowsEmptyAgentSelection(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	restoreProject := runProjectNamePrompt
	restoreImport := runImportFlow
	restoreSurvey := runInitSurvey

	t.Cleanup(func() {
		runProjectNamePrompt = restoreProject
		runImportFlow = restoreImport
		runInitSurvey = restoreSurvey
	})

	runProjectNamePrompt = func(cmd *cobra.Command, defaultName string) (string, error) {
		return "AgentSync", nil
	}

	runImportFlow = func(cmd *cobra.Command, detection detect.ProjectDetection) (importPlan, error) {
		return importPlan{}, nil
	}

	runInitSurvey = func(cmd *cobra.Command, projectName string, askGuidelines bool, askSampleSkill bool, askMCP bool) (initAnswers, error) {
		return initAnswers{
			ProjectName:  "AgentSync",
			AddGitignore: true,
			Agents:       nil,
		}, nil
	}

	command := newInitCmd()
	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("init command returned error: %v", err)
	}

	assertFileContains(t, ".ai/sync.lock", "agents = []")
	if _, err := os.Stat(".gitignore"); !os.IsNotExist(err) {
		t.Fatalf("expected no .gitignore updates when no agent entries exist, stat err=%v", err)
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
