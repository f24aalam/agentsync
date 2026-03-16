package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentpkg "github.com/f24aalam/agentsync/internal/agent"
)

func TestInstallCommandRequiresLockfile(t *testing.T) {
	wd := mustGetwdInstall(t)
	tempDir := t.TempDir()
	mustChdirInstall(t, tempDir)
	defer mustChdirInstall(t, wd)

	command := newInstallCmd()
	var stderr bytes.Buffer
	command.SetErr(&stderr)

	err := command.RunE(command, nil)
	if err == nil {
		t.Fatalf("expected missing lockfile error")
	}
	if !strings.Contains(stderr.String(), "Run `agentsync init` first") {
		t.Fatalf("expected init-first message, got %q", stderr.String())
	}
}

func TestInstallCommandContinuesOnUnknownAgent(t *testing.T) {
	wd := mustGetwdInstall(t)
	tempDir := t.TempDir()
	mustChdirInstall(t, tempDir)
	defer mustChdirInstall(t, wd)

	mustWriteInstallFile(t, ".ai/sync.lock", `agents = ["unknown", "codex"]`)

	restore := runAgentRunner
	t.Cleanup(func() { runAgentRunner = restore })
	runAgentRunner = func(targets []agentpkg.Agent, mode string) agentpkg.RunSummary {
		results := make([]agentpkg.InstallResult, 0, len(targets))
		for _, target := range targets {
			results = append(results, agentpkg.InstallResult{
				Agent: target,
				Steps: []agentpkg.StepResult{
					{Name: "Guidelines", Target: target.GuidelinesFile, Status: agentpkg.StepStatusOK},
					{Name: "Skills", Target: target.SkillsDir, Status: agentpkg.StepStatusOK},
					{Name: "MCP", Target: target.MCPConfig, Status: agentpkg.StepStatusOK},
				},
			})
		}
		return agentpkg.RunSummary{Mode: mode, Results: results, ConfiguredCount: len(results)}
	}

	command := newInstallCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("install returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "unknown") {
		t.Fatalf("expected unknown agent to be reported, got %q", output)
	}
	if !strings.Contains(output, "Done! 1 agents configured.") {
		t.Fatalf("expected one successful agent, got %q", output)
	}
}

func TestInstallCommandReportsStepErrorsAndContinues(t *testing.T) {
	wd := mustGetwdInstall(t)
	tempDir := t.TempDir()
	mustChdirInstall(t, tempDir)
	defer mustChdirInstall(t, wd)

	mustWriteInstallFile(t, ".ai/sync.lock", `agents = ["claude-code", "cursor"]`)

	restore := runAgentRunner
	t.Cleanup(func() { runAgentRunner = restore })
	runAgentRunner = func(targets []agentpkg.Agent, mode string) agentpkg.RunSummary {
		results := make([]agentpkg.InstallResult, 0, len(targets))
		configured := 0
		for _, target := range targets {
			if target.ID == "claude-code" {
				results = append(results, agentpkg.InstallResult{
					Agent: target,
					Steps: []agentpkg.StepResult{
						{Name: "Guidelines", Target: target.GuidelinesFile, Status: agentpkg.StepStatusError, Err: errors.New("boom")},
						{Name: "Skills", Target: target.SkillsDir, Status: agentpkg.StepStatusOK},
						{Name: "MCP", Target: target.MCPConfig, Status: agentpkg.StepStatusSkipped},
					},
				})
				continue
			}
			results = append(results, agentpkg.InstallResult{
				Agent: target,
				Steps: []agentpkg.StepResult{
					{Name: "Guidelines", Target: ".cursor/rules/agentsync.mdc", Status: agentpkg.StepStatusOK},
					{Name: "Skills", Target: target.SkillsDir, Status: agentpkg.StepStatusOK},
					{Name: "MCP", Target: target.MCPConfig, Status: agentpkg.StepStatusOK},
				},
			})
			configured++
		}
		return agentpkg.RunSummary{Mode: mode, Results: results, ConfiguredCount: configured}
	}

	command := newInstallCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("install returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "boom") {
		t.Fatalf("expected step error to be reported, got %q", output)
	}
	if !strings.Contains(output, ".cursor/rules/agentsync.mdc") {
		t.Fatalf("expected cursor resolved path in summary, got %q", output)
	}
	if !strings.Contains(output, "Done! 1 agents configured.") {
		t.Fatalf("expected one successful agent, got %q", output)
	}
}

func TestUpdateCommandUsesUpdatingIntro(t *testing.T) {
	wd := mustGetwdInstall(t)
	tempDir := t.TempDir()
	mustChdirInstall(t, tempDir)
	defer mustChdirInstall(t, wd)

	mustWriteInstallFile(t, ".ai/sync.lock", `agents = ["codex", "cursor"]`)

	restore := runAgentRunner
	t.Cleanup(func() { runAgentRunner = restore })
	runAgentRunner = func(targets []agentpkg.Agent, mode string) agentpkg.RunSummary {
		results := make([]agentpkg.InstallResult, 0, len(targets))
		for _, target := range targets {
			results = append(results, agentpkg.InstallResult{
				Agent: target,
				Steps: []agentpkg.StepResult{
					{Name: "Guidelines", Target: target.GuidelinesFile, Status: agentpkg.StepStatusOK},
					{Name: "Skills", Target: target.SkillsDir, Status: agentpkg.StepStatusOK},
					{Name: "MCP", Target: target.MCPConfig, Status: agentpkg.StepStatusOK},
				},
			})
		}
		return agentpkg.RunSummary{Mode: mode, Results: results, ConfiguredCount: len(results)}
	}

	command := newUpdateCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("update returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Updating 2 agents...") {
		t.Fatalf("expected updating intro, got %q", stdout.String())
	}
}

func mustGetwdInstall(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func mustChdirInstall(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
}

func mustWriteInstallFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
