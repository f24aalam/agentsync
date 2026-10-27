package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadLock(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	content := `agents = ["claude-code", "cursor", "gemini-cli"]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write sync.lock: %v", err)
	}

	agents, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock returned error: %v", err)
	}

	want := []string{"claude-code", "cursor", "gemini-cli"}
	if !reflect.DeepEqual(agents, want) {
		t.Fatalf("expected agents %v, got %v", want, agents)
	}
}

func TestWriteLockRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	want := []string{"junie", "codex", "opencode"}
	if err := WriteLock(path, want); err != nil {
		t.Fatalf("WriteLock returned error: %v", err)
	}

	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock returned error: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected round-trip agents %v, got %v", want, got)
	}
}

func TestWriteLockEmptyAgents(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	if err := WriteLock(path, nil); err != nil {
		t.Fatalf("WriteLock returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sync.lock: %v", err)
	}

	if string(data) != "agents = []\n" {
		t.Fatalf("expected empty lockfile to contain agents = [], got %q", string(data))
	}

	got, err := ReadLock(path)
	if err != nil {
		t.Fatalf("ReadLock returned error: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty agent list, got %v", got)
	}
}

func TestReadLockInvalidTOML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	if err := os.WriteFile(path, []byte(`agents = ["claude-code"`), 0o644); err != nil {
		t.Fatalf("write malformed sync.lock: %v", err)
	}

	if _, err := ReadLock(path); err == nil {
		t.Fatalf("expected malformed lockfile to return an error")
	}
}

func TestReadLockMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.lock")
	if _, err := ReadLock(path); err == nil {
		t.Fatalf("expected missing lockfile to return an error")
	}
}

func TestFullLock_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	lock := NewLockFile()
	lock.Agents = []string{"claude-code", "codex", "cursor"}
	lock.RecordSkillPackage(".claude/skills", "code-review")
	lock.RecordSkillPackage(".cursor/skills", "code-review")
	lock.RecordMCP(".mcp.json", "postgres")
	lock.RecordMCP(".mcp.json", "filesystem")
	lock.RecordMCP(".codex/config.toml", "postgres")

	if err := WriteFullLock(path, lock); err != nil {
		t.Fatalf("WriteFullLock failed: %v", err)
	}

	loaded, err := ReadFullLock(path)
	if err != nil {
		t.Fatalf("ReadFullLock failed: %v", err)
	}

	if !reflect.DeepEqual(loaded.Agents, lock.Agents) {
		t.Errorf("agents mismatch:\nWant: %v\nGot: %v", lock.Agents, loaded.Agents)
	}
	if !reflect.DeepEqual(loaded.Skills, lock.Skills) {
		t.Errorf("skills mismatch:\nWant: %v\nGot: %v", lock.Skills, loaded.Skills)
	}
	if !reflect.DeepEqual(loaded.MCP, lock.MCP) {
		t.Errorf("MCP mismatch:\nWant: %v\nGot: %v", lock.MCP, loaded.MCP)
	}

	// Verify query helpers
	if !loaded.IsSkillPackageManaged(".claude/skills", "code-review") {
		t.Errorf("expected code-review to be managed in .claude/skills")
	}
	if loaded.IsSkillPackageManaged(".claude/skills", "unknown") {
		t.Errorf("expected unknown skill not to be managed")
	}
	if !loaded.HasTrackedSkills(".claude/skills") {
		t.Errorf("expected .claude/skills to have tracked skills")
	}
	if loaded.HasTrackedSkills(".unknown/skills") {
		t.Errorf("expected .unknown/skills not to have tracked skills")
	}
	if !loaded.IsMCPManaged(".mcp.json", "postgres") {
		t.Errorf("expected postgres to be managed in .mcp.json")
	}
	if loaded.IsMCPManaged(".mcp.json", "unknown") {
		t.Errorf("expected unknown MCP not to be managed")
	}
}

func TestWriteLock_PreservesExistingMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	initial := NewLockFile()
	initial.Agents = []string{"claude-code"}
	initial.RecordSkillPackage(".claude/skills", "testing")
	initial.RecordMCP(".mcp.json", "postgres")

	if err := WriteFullLock(path, initial); err != nil {
		t.Fatalf("initial WriteFullLock failed: %v", err)
	}

	if err := WriteLock(path, []string{"claude-code", "cursor"}); err != nil {
		t.Fatalf("WriteLock failed: %v", err)
	}

	updated, err := ReadFullLock(path)
	if err != nil {
		t.Fatalf("ReadFullLock failed: %v", err)
	}

	wantAgents := []string{"claude-code", "cursor"}
	if !reflect.DeepEqual(updated.Agents, wantAgents) {
		t.Errorf("agents not updated: got %v, want %v", updated.Agents, wantAgents)
	}
	if !updated.IsSkillPackageManaged(".claude/skills", "testing") {
		t.Errorf("skills metadata lost: %v", updated.Skills)
	}
	if !updated.IsMCPManaged(".mcp.json", "postgres") {
		t.Errorf("mcp metadata lost: %v", updated.MCP)
	}
}
