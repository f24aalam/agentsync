package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMCP(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.toml")

	content := `
[servers.postgres]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]

[servers.postgres.env]
DATABASE_URL = "${DATABASE_URL}"

[servers.filesystem]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/home/user/projects"]

[servers.my-custom]
command = "go"
args = ["run", "./cmd/mcp"]
`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write mcp.toml: %v", err)
	}

	cfg, err := ReadMCP(path)
	if err != nil {
		t.Fatalf("ReadMCP returned error: %v", err)
	}

	if len(cfg.Servers) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(cfg.Servers))
	}

	postgres, ok := cfg.Servers["postgres"]
	if !ok {
		t.Fatalf("expected postgres server to be present")
	}

	if postgres.Command != "npx" {
		t.Fatalf("expected postgres command npx, got %q", postgres.Command)
	}

	if len(postgres.Args) != 2 {
		t.Fatalf("expected postgres args to have 2 elements, got %d", len(postgres.Args))
	}

	if postgres.Env["DATABASE_URL"] != "${DATABASE_URL}" {
		t.Fatalf("expected DATABASE_URL placeholder to be preserved, got %q", postgres.Env["DATABASE_URL"])
	}

	filesystem, ok := cfg.Servers["filesystem"]
	if !ok {
		t.Fatalf("expected filesystem server to be present")
	}

	if filesystem.Env != nil {
		t.Fatalf("expected filesystem env to be nil when omitted, got %#v", filesystem.Env)
	}
}

func TestReadMCPEmptyServers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.toml")

	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("write empty mcp.toml: %v", err)
	}

	cfg, err := ReadMCP(path)
	if err != nil {
		t.Fatalf("ReadMCP returned error: %v", err)
	}

	if cfg.Servers == nil {
		t.Fatalf("expected servers map to be initialized")
	}

	if len(cfg.Servers) != 0 {
		t.Fatalf("expected no servers, got %d", len(cfg.Servers))
	}
}
