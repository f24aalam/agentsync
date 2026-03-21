package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectInstallConflictsEmptyWhenNothingToWrite(t *testing.T) {
	dir := t.TempDir()
	codex, _ := ByID("codex")
	conflicts, err := DetectInstallConflicts([]Agent{codex}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %+v", conflicts)
	}
}

func TestDetectInstallConflictsGuidelines(t *testing.T) {
	dir := t.TempDir()
	mustWriteConflict(t, dir, ".ai/guidelines/a.md", "a")
	mustWriteConflict(t, dir, "AGENTS.md", "existing")
	codex, _ := ByID("codex")
	conflicts, err := DetectInstallConflicts([]Agent{codex}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].Kind != KindGuidelines || conflicts[0].AgentID != "codex" {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
}

func TestDetectInstallConflictsSkillsDir(t *testing.T) {
	dir := t.TempDir()
	mustWriteConflict(t, dir, ".ai/skills/s/SKILL.md", "s")
	mustWriteConflict(t, dir, ".agents/skills/keep/readme.md", "x")
	codex, _ := ByID("codex")
	conflicts, err := DetectInstallConflicts([]Agent{codex}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].Kind != KindSkillsDir {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
	if !strings.Contains(conflicts[0].StepKey, "install/skills/") {
		t.Fatalf("expected skills step key, got %q", conflicts[0].StepKey)
	}
}

func TestDetectInstallConflictsMCP(t *testing.T) {
	dir := t.TempDir()
	mustWriteConflict(t, dir, ".ai/mcp.toml", `
[servers.pg]
command = "npx"
`)
	mustWriteConflict(t, dir, ".codex/config.toml", "[mcp_servers]\n")
	codex, _ := ByID("codex")
	conflicts, err := DetectInstallConflicts([]Agent{codex}, dir)
	if err != nil {
		t.Fatal(err)
	}
	var mcp []InstallConflict
	for _, c := range conflicts {
		if c.Kind == KindMCP {
			mcp = append(mcp, c)
		}
	}
	if len(mcp) != 1 || mcp[0].AgentID != "codex" {
		t.Fatalf("expected one MCP conflict, got %+v", conflicts)
	}
}

func mustWriteConflict(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}
