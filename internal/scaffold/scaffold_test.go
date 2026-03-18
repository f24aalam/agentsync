package scaffold

import (
	"os"
	"strings"
	"testing"

	"github.com/f24aalam/agentsync/internal/agent"
)

func TestEnsureDirsCreatesExpectedStructure(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs returned error: %v", err)
	}

	for _, path := range []string{aiRootDir, guidelinesDir, skillsDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}

		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", path)
		}
	}
}

func TestCreateGuidelinesIncludesProjectName(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs returned error: %v", err)
	}

	path, err := CreateGuidelines("Demo")
	if err != nil {
		t.Fatalf("CreateGuidelines returned error: %v", err)
	}

	assertFileContains(t, path, "# Demo Guidelines")
	assertFileContains(t, path, "## Code Style")
	assertFileContains(t, path, "## Architecture")
	assertFileContains(t, path, "## Conventions")
}

func TestCreateSampleSkillCreatesValidTemplate(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs returned error: %v", err)
	}

	path, err := CreateSampleSkill()
	if err != nil {
		t.Fatalf("CreateSampleSkill returned error: %v", err)
	}

	assertFileContains(t, path, "---")
	assertFileContains(t, path, "name: example-skill")
	assertFileContains(t, path, "# Example Skill")
}

func TestCreateMCPConfigCreatesCommentedTemplate(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	if err := EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs returned error: %v", err)
	}

	path, err := CreateMCPConfig()
	if err != nil {
		t.Fatalf("CreateMCPConfig returned error: %v", err)
	}

	assertFileContains(t, path, "# Example MCP servers.")
	assertFileContains(t, path, "# [servers.postgres]")
}

func TestUpdateGitignoreCreatesEntriesForSelectedAgents(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	cursor, _ := agent.ByID("cursor")
	codex, _ := agent.ByID("codex")

	updated, err := UpdateGitignore(".gitignore", []agent.Agent{cursor, codex})
	if err != nil {
		t.Fatalf("UpdateGitignore returned error: %v", err)
	}

	if !updated {
		t.Fatalf("expected gitignore to be updated")
	}

	assertFileContains(t, ".gitignore", ".cursor/rules/*.mdc")
	assertFileContains(t, ".gitignore", ".codex/config.toml")
	assertFileContains(t, ".gitignore", "AGENTS.md")
}

func TestUpdateGitignoreAppendsMissingEntriesOnly(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	cursor, _ := agent.ByID("cursor")
	if err := os.WriteFile(".gitignore", []byte(".agents/skills/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	updated, err := UpdateGitignore(".gitignore", []agent.Agent{cursor})
	if err != nil {
		t.Fatalf("UpdateGitignore returned error: %v", err)
	}

	if !updated {
		t.Fatalf("expected gitignore to be updated")
	}

	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}

	if strings.Count(string(data), ".agents/skills/") != 1 {
		t.Fatalf("expected existing ignore entry to remain unique, got %q", string(data))
	}

	assertFileContains(t, ".gitignore", ".cursor/rules/*.mdc")
}

func TestUpdateGitignoreNoEntriesNoop(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	updated, err := UpdateGitignore(".gitignore", nil)
	if err != nil {
		t.Fatalf("UpdateGitignore returned error: %v", err)
	}

	if updated {
		t.Fatalf("expected no update")
	}

	if _, err := os.Stat(".gitignore"); !os.IsNotExist(err) {
		t.Fatalf("expected .gitignore to remain absent, stat err=%v", err)
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
