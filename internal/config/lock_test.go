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
