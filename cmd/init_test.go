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

func TestInitAbortsWithoutWritesWhenOverwriteDeclined(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	if err := os.MkdirAll(".ai", 0o755); err != nil {
		t.Fatalf("mkdir .ai: %v", err)
	}

	if err := os.WriteFile(".ai/sync.lock", []byte(`agents = ["codex"]`), 0o644); err != nil {
		t.Fatalf("write existing lockfile: %v", err)
	}

	restoreOverwrite := runOverwriteConfirm
	restoreProject := runProjectNamePrompt
	restoreImport := runImportFlow
	restoreSurvey := runInitSurvey
	t.Cleanup(func() {
		runOverwriteConfirm = restoreOverwrite
		runProjectNamePrompt = restoreProject
		runImportFlow = restoreImport
		runInitSurvey = restoreSurvey
	})

	runOverwriteConfirm = func(cmd *cobra.Command) (bool, error) {
		return false, nil
	}

	runProjectNamePrompt = func(cmd *cobra.Command, defaultName string) (string, error) {
		t.Fatalf("project name prompt should not run when overwrite is declined")
		return "", nil
	}

	runImportFlow = func(cmd *cobra.Command, detection detect.ProjectDetection) (importPlan, error) {
		t.Fatalf("import flow should not run when overwrite is declined")
		return importPlan{}, nil
	}

	runInitSurvey = func(cmd *cobra.Command, projectName string, askGuidelines bool, askSampleSkill bool, askMCP bool) (initAnswers, error) {
		t.Fatalf("survey should not run when overwrite is declined")
		return initAnswers{}, nil
	}

	command := newInitCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("init command returned error: %v", err)
	}

	if _, err := os.Stat(".ai/guidelines/core.md"); !os.IsNotExist(err) {
		t.Fatalf("expected no scaffold writes, stat error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Initialization cancelled") {
		t.Fatalf("expected cancellation message, got %q", stdout.String())
	}
}

func TestInitCreatesScaffoldAndGitignore(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	restoreOverwrite := runOverwriteConfirm
	restoreProject := runProjectNamePrompt
	restoreImport := runImportFlow
	restoreSurvey := runInitSurvey

	t.Cleanup(func() {
		runOverwriteConfirm = restoreOverwrite
		runProjectNamePrompt = restoreProject
		runImportFlow = restoreImport
		runInitSurvey = restoreSurvey
	})

	runOverwriteConfirm = func(cmd *cobra.Command) (bool, error) {
		return true, nil
	}

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

	restoreOverwrite := runOverwriteConfirm
	restoreProject := runProjectNamePrompt
	restoreImport := runImportFlow
	restoreSurvey := runInitSurvey

	t.Cleanup(func() {
		runOverwriteConfirm = restoreOverwrite
		runProjectNamePrompt = restoreProject
		runImportFlow = restoreImport
		runInitSurvey = restoreSurvey
	})

	runOverwriteConfirm = func(cmd *cobra.Command) (bool, error) {
		return true, nil
	}

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
