package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/f24aalam/agentsync/internal/config"
)

// TestMCP_SuccessfulJSONCreationRecordsOwnership verifies fresh JSON MCP write records ownership.
func TestMCP_SuccessfulJSONCreationRecordsOwnership(t *testing.T) {
	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, ".cursor/mcp.json")
	target, _ := ByID("cursor")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"postgres": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres"}},
		},
	}
	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	err := mergeOrWriteJSONMCP(dest, target, cfg, plan, ".cursor/mcp.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil || !strings.Contains(string(data), "postgres") {
		t.Fatalf("destination file not written correctly: %s", string(data))
	}

	if !lock.IsMCPManaged(".cursor/mcp.json", "postgres") {
		t.Fatalf("expected postgres recorded in lock")
	}
}

// TestMCP_SuccessfulTOMLCreationRecordsOwnership verifies fresh TOML MCP write records ownership.
func TestMCP_SuccessfulTOMLCreationRecordsOwnership(t *testing.T) {
	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, ".codex/config.toml")
	target, _ := ByID("codex")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"postgres": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres"}},
		},
	}
	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	err := mergeOrWriteTomlMCP(dest, target, cfg, plan, ".codex/config.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil || !strings.Contains(string(data), "[mcp_servers.postgres]") {
		t.Fatalf("destination file not written correctly: %s", string(data))
	}

	if !lock.IsMCPManaged(".codex/config.toml", "postgres") {
		t.Fatalf("expected postgres recorded in lock")
	}
}

// TestMCP_SuccessfulApprovedReplacementRecordsOwnership verifies replacing a differing server records ownership.
func TestMCP_SuccessfulApprovedReplacementRecordsOwnership(t *testing.T) {
	tempDir := t.TempDir()
	jsonDest := filepath.Join(tempDir, ".cursor/mcp.json")
	mustWrite(t, jsonDest, `{"mcpServers": {"postgres": {"command": "old-pg"}}}`)

	tomlDest := filepath.Join(tempDir, ".codex/config.toml")
	mustWrite(t, tomlDest, "[mcp_servers.postgres]\ncommand = \"old-pg\"\n")

	cursor, _ := ByID("cursor")
	codex, _ := ByID("codex")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"postgres": {Command: "npx", Args: []string{"-y", "new-pg"}},
		},
	}

	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	if err := mergeOrWriteJSONMCP(jsonDest, cursor, cfg, plan, ".cursor/mcp.json"); err != nil {
		t.Fatalf("JSON replacement failed: %v", err)
	}
	if !lock.IsMCPManaged(".cursor/mcp.json", "postgres") {
		t.Fatalf("expected JSON replaced postgres recorded in lock")
	}

	if err := mergeOrWriteTomlMCP(tomlDest, codex, cfg, plan, ".codex/config.toml"); err != nil {
		t.Fatalf("TOML replacement failed: %v", err)
	}
	if !lock.IsMCPManaged(".codex/config.toml", "postgres") {
		t.Fatalf("expected TOML replaced postgres recorded in lock")
	}
}

// TestMCP_FailedJSONWriteLeavesOwnershipUnchanged verifies that when writing JSON fails, ownership is NOT recorded.
func TestMCP_FailedJSONWriteLeavesOwnershipUnchanged(t *testing.T) {
	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, ".cursor/mcp.json")
	target, _ := ByID("cursor")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"postgres": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres"}},
		},
	}
	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	origWrite := osWriteFile
	defer func() { osWriteFile = origWrite }()
	simulatedErr := errors.New("simulated disk write failure")
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		return simulatedErr
	}

	err := mergeOrWriteJSONMCP(dest, target, cfg, plan, ".cursor/mcp.json")
	if err == nil {
		t.Fatalf("expected write error, got nil")
	}

	if lock.IsMCPManaged(".cursor/mcp.json", "postgres") {
		t.Fatalf("OWNERSHIP LEAK: postgres recorded in lock despite failed write!")
	}
}

// TestMCP_FailedTOMLWriteLeavesOwnershipUnchanged verifies that when writing TOML fails, ownership is NOT recorded.
func TestMCP_FailedTOMLWriteLeavesOwnershipUnchanged(t *testing.T) {
	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, ".codex/config.toml")
	target, _ := ByID("codex")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"postgres": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres"}},
		},
	}
	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	origWrite := osWriteFile
	defer func() { osWriteFile = origWrite }()
	simulatedErr := errors.New("simulated disk write failure")
	osWriteFile = func(name string, data []byte, perm os.FileMode) error {
		return simulatedErr
	}

	err := mergeOrWriteTomlMCP(dest, target, cfg, plan, ".codex/config.toml")
	if err == nil {
		t.Fatalf("expected write error, got nil")
	}

	if lock.IsMCPManaged(".codex/config.toml", "postgres") {
		t.Fatalf("OWNERSHIP LEAK: postgres recorded in lock despite failed write!")
	}
}

// TestMCP_MalformedExistingConfigDoesNotModifyOwnership verifies invalid config returns error without touching lock.
func TestMCP_MalformedExistingConfigDoesNotModifyOwnership(t *testing.T) {
	tempDir := t.TempDir()
	jsonDest := filepath.Join(tempDir, ".cursor/mcp.json")
	mustWrite(t, jsonDest, `{ broken json `)

	tomlDest := filepath.Join(tempDir, ".codex/config.toml")
	mustWrite(t, tomlDest, `[unclosed table`)

	cursor, _ := ByID("cursor")
	codex, _ := ByID("codex")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"postgres": {Command: "npx"},
		},
	}
	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	errJSON := mergeOrWriteJSONMCP(jsonDest, cursor, cfg, plan, ".cursor/mcp.json")
	if errJSON == nil {
		t.Fatalf("expected JSON parse error, got nil")
	}
	if lock.IsMCPManaged(".cursor/mcp.json", "postgres") {
		t.Fatalf("ownership recorded despite malformed JSON")
	}

	errTOML := mergeOrWriteTomlMCP(tomlDest, codex, cfg, plan, ".codex/config.toml")
	if errTOML == nil {
		t.Fatalf("expected TOML parse error, got nil")
	}
	if lock.IsMCPManaged(".codex/config.toml", "postgres") {
		t.Fatalf("ownership recorded despite malformed TOML")
	}
}

// TestMCP_UnsupportedEntriesDoNotReceiveOwnership verifies servers with empty command receive no ownership.
func TestMCP_UnsupportedEntriesDoNotReceiveOwnership(t *testing.T) {
	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, ".cursor/mcp.json")
	target, _ := ByID("cursor")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"empty_server": {Command: "   "},
		},
	}
	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	_ = mergeOrWriteJSONMCP(dest, target, cfg, plan, ".cursor/mcp.json")
	if lock.IsMCPManaged(".cursor/mcp.json", "empty_server") {
		t.Fatalf("unsupported server received ownership in lock")
	}
}

// TestMCP_IdenticalUnmanagedEntriesNotSilentlyAdopted verifies:
// - Destination has pre-existing unmanaged server with identical command/args/env
// - Server is not recorded into sync.lock merely because contents match
func TestMCP_IdenticalUnmanagedEntriesNotSilentlyAdopted(t *testing.T) {
	tempDir := t.TempDir()
	jsonDest := filepath.Join(tempDir, ".cursor/mcp.json")
	mustWrite(t, jsonDest, `{
  "mcpServers": {
    "postgres": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres"]
    }
  }
}
`)

	tomlDest := filepath.Join(tempDir, ".codex/config.toml")
	mustWrite(t, tomlDest, `[mcp_servers.postgres]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]
`)

	cursor, _ := ByID("cursor")
	codex, _ := ByID("codex")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"postgres": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres"}},
		},
	}

	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	// JSON
	if err := mergeOrWriteJSONMCP(jsonDest, cursor, cfg, plan, ".cursor/mcp.json"); err != nil {
		t.Fatalf("unexpected JSON error: %v", err)
	}
	if lock.IsMCPManaged(".cursor/mcp.json", "postgres") {
		t.Fatalf("ADOPTION VIOLATION: identical unmanaged JSON server adopted into lock!")
	}

	// TOML
	if err := mergeOrWriteTomlMCP(tomlDest, codex, cfg, plan, ".codex/config.toml"); err != nil {
		t.Fatalf("unexpected TOML error: %v", err)
	}
	if lock.IsMCPManaged(".codex/config.toml", "postgres") {
		t.Fatalf("ADOPTION VIOLATION: identical unmanaged TOML server adopted into lock!")
	}
}

// TestMCP_RepeatedUnchangedInstallPreservesMTime verifies that repeated runs do not rewrite identical files.
func TestMCP_RepeatedUnchangedInstallPreservesMTime(t *testing.T) {
	tempDir := t.TempDir()
	dest := filepath.Join(tempDir, ".codex/config.toml")
	codex, _ := ByID("codex")

	cfg := config.MCPConfig{
		Servers: map[string]config.MCPServer{
			"postgres": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres"}},
		},
	}
	plan := NewInstallPlan()
	lock := config.NewLockFile()
	plan.Lock = lock

	// First run writes the file
	if err := mergeOrWriteTomlMCP(dest, codex, cfg, plan, ".codex/config.toml"); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Set an older mtime
	oldTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(dest, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes failed: %v", err)
	}
	statBefore, _ := os.Stat(dest)

	// Second run should be a no-op
	if err := mergeOrWriteTomlMCP(dest, codex, cfg, plan, ".codex/config.toml"); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	statAfter, _ := os.Stat(dest)
	if !statBefore.ModTime().Equal(statAfter.ModTime()) {
		t.Fatalf("mtime changed on identical repeated install! Before: %v, After: %v", statBefore.ModTime(), statAfter.ModTime())
	}
}
