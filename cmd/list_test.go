package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListCommandRequiresAIConfig(t *testing.T) {
	wd := mustGetwdList(t)
	tempDir := t.TempDir()
	mustChdirList(t, tempDir)
	defer mustChdirList(t, wd)

	command := newListCmd()
	var stderr bytes.Buffer
	command.SetErr(&stderr)

	err := command.RunE(command, nil)
	if err == nil {
		t.Fatalf("expected missing .ai error")
	}
	if !strings.Contains(stderr.String(), "Run `agentsync init` first") {
		t.Fatalf("expected init-first message, got %q", stderr.String())
	}
}

func TestListCommandDisplaysDiscoveredConfig(t *testing.T) {
	wd := mustGetwdList(t)
	tempDir := t.TempDir()
	mustChdirList(t, tempDir)
	defer mustChdirList(t, wd)

	mustWriteListFile(t, ".ai/guidelines/custom-rules.md", "# custom")
	mustWriteListFile(t, ".ai/guidelines/core.md", "# core")
	mustWriteListFile(t, ".ai/skills/creating-invoices/SKILL.md", "x")
	mustWriteListFile(t, ".ai/skills/api-conventions/SKILL.md", "x")
	mustWriteListFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "npx"

[servers.filesystem]
command = "npx"
`)
	mustWriteListFile(t, ".ai/sync.lock", `agents = ["claude-code", "cursor"]`)

	command := newListCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("list returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"Guidelines (2)",
		"core.md",
		"custom-rules.md",
		"Skills (2)",
		"api-conventions",
		"creating-invoices",
		"MCP Servers (2)",
		"filesystem",
		"postgres",
		"Agents (sync.lock)",
		"Claude Code",
		"Cursor",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}

func TestListCommandShowsNoneAndNotInitialized(t *testing.T) {
	wd := mustGetwdList(t)
	tempDir := t.TempDir()
	mustChdirList(t, tempDir)
	defer mustChdirList(t, wd)

	if err := os.MkdirAll(".ai", 0o755); err != nil {
		t.Fatalf("mkdir .ai: %v", err)
	}

	command := newListCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("list returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "none") {
		t.Fatalf("expected empty sections to show none, got %q", output)
	}
	if !strings.Contains(output, "not initialized") {
		t.Fatalf("expected agents section to show not initialized, got %q", output)
	}
}

func mustGetwdList(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}

func mustChdirList(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
}

func mustWriteListFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
