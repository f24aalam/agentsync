package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentpkg "github.com/f24aalam/agentsync/internal/agent"
	stepflow "github.com/f24aalam/stepflow/pkg"
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
	runAgentRunner = func(targets []agentpkg.Agent, plan agentpkg.InstallPlan, root string) agentpkg.RunSummary {
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
		return agentpkg.RunSummary{Results: results, ConfiguredCount: len(results)}
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
	runAgentRunner = func(targets []agentpkg.Agent, plan agentpkg.InstallPlan, root string) agentpkg.RunSummary {
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
		return agentpkg.RunSummary{Results: results, ConfiguredCount: configured}
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

func TestInstallYesFlagSkipsConflictPrompts(t *testing.T) {
	wd := mustGetwdInstall(t)
	tempDir := t.TempDir()
	mustChdirInstall(t, tempDir)
	defer mustChdirInstall(t, wd)

	mustWriteInstallFile(t, ".ai/sync.lock", `agents = ["codex"]`)
	mustWriteInstallFile(t, ".ai/guidelines/core.md", "g")
	mustWriteInstallFile(t, "AGENTS.md", "existing")

	restoreSF := runInstallStepflow
	t.Cleanup(func() { runInstallStepflow = restoreSF })
	runInstallStepflow = func([]stepflow.Step) (stepflow.Result, error) {
		return nil, fmt.Errorf("stepflow should not run when --yes")
	}

	restore := runAgentRunner
	t.Cleanup(func() { runAgentRunner = restore })
	runAgentRunner = func(targets []agentpkg.Agent, plan agentpkg.InstallPlan, root string) agentpkg.RunSummary {
		if plan.SkipGuidelines["codex"] {
			t.Fatalf("expected no skip with --yes, got plan %+v", plan.SkipGuidelines)
		}
		return agentpkg.RunSummary{ConfiguredCount: 1}
	}

	command := newInstallCmd()
	_ = command.Flags().Set("yes", "true")
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
}

func TestInstallConflictNoAppliesSkipToPlan(t *testing.T) {
	wd := mustGetwdInstall(t)
	tempDir := t.TempDir()
	mustChdirInstall(t, tempDir)
	defer mustChdirInstall(t, wd)

	mustWriteInstallFile(t, ".ai/sync.lock", `agents = ["codex"]`)
	mustWriteInstallFile(t, ".ai/guidelines/core.md", "g")
	mustWriteInstallFile(t, "AGENTS.md", "existing")

	key := "install/codex/guidelines"
	restoreSF := runInstallStepflow
	t.Cleanup(func() { runInstallStepflow = restoreSF })
	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		if len(steps) != 1 {
			t.Fatalf("expected 1 conflict step, got %d", len(steps))
		}
		return stepflow.Result{key: "No"}, nil
	}

	restore := runAgentRunner
	t.Cleanup(func() { runAgentRunner = restore })
	runAgentRunner = func(targets []agentpkg.Agent, plan agentpkg.InstallPlan, root string) agentpkg.RunSummary {
		if !plan.SkipGuidelines["codex"] {
			t.Fatalf("expected guidelines skip for codex, plan=%+v", plan.SkipGuidelines)
		}
		return agentpkg.RunSummary{ConfiguredCount: 1}
	}

	command := newInstallCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(stdout.String(), "Installing for 1 agents...") {
		t.Fatalf("expected installing intro, got %q", stdout.String())
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
