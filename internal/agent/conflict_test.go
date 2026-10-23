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

	// Sibling skill 'keep' does NOT trigger conflict for 's' (Rule 1 & 6)
	conflicts, err := DetectInstallConflicts([]Agent{codex}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts when destination skill 's' does not exist, got %+v", conflicts)
	}

	// When skill 's' exists in destination with different content, package conflict is detected (Rule 3)
	mustWriteConflict(t, dir, ".agents/skills/s/SKILL.md", "different content")
	conflicts2, err := DetectInstallConflicts([]Agent{codex}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts2) != 1 || conflicts2[0].Kind != KindSkillPackage || conflicts2[0].SkillName != "s" {
		t.Fatalf("unexpected conflicts: %+v", conflicts2)
	}
	if !strings.Contains(conflicts2[0].StepKey, "install/skills/.agents_skills/s") {
		t.Fatalf("expected skills step key, got %q", conflicts2[0].StepKey)
	}
}

func TestDetectInstallConflictsMCP(t *testing.T) {
	dir := t.TempDir()
	mustWriteConflict(t, dir, ".ai/mcp.toml", `
[servers.pg]
command = "npx"
`)
	// Missing server in destination does NOT trigger conflict
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
	if len(mcp) != 0 {
		t.Fatalf("expected no MCP conflict for missing destination server, got %+v", mcp)
	}

	// Existing destination server with different command DOES trigger conflict
	mustWriteConflict(t, dir, ".codex/config.toml", "[mcp_servers.pg]\ncommand = \"other\"\n")
	conflicts2, err := DetectInstallConflicts([]Agent{codex}, dir)
	if err != nil {
		t.Fatal(err)
	}
	var mcp2 []InstallConflict
	for _, c := range conflicts2 {
		if c.Kind == KindMCP {
			mcp2 = append(mcp2, c)
		}
	}
	if len(mcp2) != 1 || mcp2[0].AgentID != "codex" {
		t.Fatalf("expected one MCP conflict for differing server, got %+v", mcp2)
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
