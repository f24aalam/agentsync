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

func TestReadFullLock_LegacyFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	// Old lock format: contains only agents list
	legacyContent := "agents = [\"claude-code\", \"cursor\"]\n"
	if err := os.WriteFile(path, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("write legacy sync.lock: %v", err)
	}

	lock, err := ReadFullLock(path)
	if err != nil {
		t.Fatalf("ReadFullLock on legacy format returned error: %v", err)
	}

	if len(lock.Agents) != 2 || lock.Agents[0] != "claude-code" || lock.Agents[1] != "cursor" {
		t.Fatalf("unexpected agents: %v", lock.Agents)
	}
	// Conservative behavior: must NOT assume ownership of any existing resources
	if len(lock.Guidelines) != 0 {
		t.Fatalf("expected empty guidelines ownership for legacy lockfile, got %v", lock.Guidelines)
	}
	if len(lock.Skills) != 0 {
		t.Fatalf("expected empty skills ownership for legacy lockfile, got %v", lock.Skills)
	}
	if len(lock.MCP) != 0 {
		t.Fatalf("expected empty MCP ownership for legacy lockfile, got %v", lock.MCP)
	}
}

func TestFullLock_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	lock := NewLockFile()
	lock.Agents = []string{"claude-code", "codex"}
	lock.Guidelines["CLAUDE.md"] = ResourceLock{Hash: HashString("guideline content")}
	lock.Guidelines["AGENTS.md"] = ResourceLock{Hash: HashString("agents content")}
	lock.Skills[".claude/skills/code-review/SKILL.md"] = ResourceLock{Hash: HashString("skill content")}
	lock.MCP[".mcp.json"] = map[string]ResourceLock{
		"postgres":   {Hash: HashString("postgres-json")},
		"filesystem": {Hash: HashString("filesystem-json")},
	}
	lock.MCP[".codex/config.toml"] = map[string]ResourceLock{
		"postgres": {Hash: HashString("postgres-toml")},
	}

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
	if !reflect.DeepEqual(loaded.Guidelines, lock.Guidelines) {
		t.Errorf("guidelines mismatch:\nWant: %v\nGot: %v", lock.Guidelines, loaded.Guidelines)
	}
	if !reflect.DeepEqual(loaded.Skills, lock.Skills) {
		t.Errorf("skills mismatch:\nWant: %v\nGot: %v", lock.Skills, loaded.Skills)
	}
	if !reflect.DeepEqual(loaded.MCP, lock.MCP) {
		t.Errorf("MCP mismatch:\nWant: %v\nGot: %v", lock.MCP, loaded.MCP)
	}
}

func TestWriteLock_PreservesExistingMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sync.lock")

	// Set up full lock with existing ownership metadata
	initial := NewLockFile()
	initial.Agents = []string{"claude-code"}
	initial.Guidelines["CLAUDE.md"] = ResourceLock{Hash: "hash123"}
	initial.Skills[".claude/skills/review.sh"] = ResourceLock{Hash: "hash456"}
	initial.MCP[".mcp.json"] = map[string]ResourceLock{"postgres": {Hash: "hash789"}}

	if err := WriteFullLock(path, initial); err != nil {
		t.Fatalf("initial WriteFullLock failed: %v", err)
	}

	// Calling legacy WriteLock (e.g. from init) to update selected agents
	if err := WriteLock(path, []string{"claude-code", "cursor"}); err != nil {
		t.Fatalf("WriteLock failed: %v", err)
	}

	// Verify agents updated and existing ownership metadata preserved
	updated, err := ReadFullLock(path)
	if err != nil {
		t.Fatalf("ReadFullLock failed: %v", err)
	}

	wantAgents := []string{"claude-code", "cursor"}
	if !reflect.DeepEqual(updated.Agents, wantAgents) {
		t.Errorf("agents not updated: got %v, want %v", updated.Agents, wantAgents)
	}
	if updated.Guidelines["CLAUDE.md"].Hash != "hash123" {
		t.Errorf("guidelines metadata lost: %v", updated.Guidelines)
	}
	if updated.Skills[".claude/skills/review.sh"].Hash != "hash456" {
		t.Errorf("skills metadata lost: %v", updated.Skills)
	}
	if updated.MCP[".mcp.json"]["postgres"].Hash != "hash789" {
		t.Errorf("mcp metadata lost: %v", updated.MCP)
	}
}

func TestHashHelpers(t *testing.T) {
	t.Parallel()

	h1 := HashString("hello world")
	h2 := HashBytes([]byte("hello world"))
	if h1 != h2 {
		t.Errorf("HashString and HashBytes mismatch: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char sha256 hex string, got len %d: %s", len(h1), h1)
	}
}
