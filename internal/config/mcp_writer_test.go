package config

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/f24aalam/agentsync/internal/agent"
)

func TestRenderMCPJSON(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Servers: map[string]MCPServer{
			"postgres": {
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-postgres"},
				Env: map[string]string{
					"DATABASE_URL": "${DATABASE_URL}",
				},
			},
		},
	}

	target, ok := agent.ByID("claude-code")
	if !ok {
		t.Fatalf("expected claude-code agent")
	}

	data, err := RenderMCP(cfg, target)
	if err != nil {
		t.Fatalf("RenderMCP returned error: %v", err)
	}

	var got struct {
		MCPServers map[string]MCPServer `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}

	postgres, ok := got.MCPServers["postgres"]
	if !ok {
		t.Fatalf("expected postgres server in JSON output")
	}

	if postgres.Command != "npx" {
		t.Fatalf("expected command npx, got %q", postgres.Command)
	}

	if postgres.Env["DATABASE_URL"] != "${DATABASE_URL}" {
		t.Fatalf("expected env placeholder to be preserved, got %q", postgres.Env["DATABASE_URL"])
	}
}

func TestRenderMCPJSONOmitsEmptyEnv(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Servers: map[string]MCPServer{
			"filesystem": {
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
			},
		},
	}

	target, ok := agent.ByID("cursor")
	if !ok {
		t.Fatalf("expected cursor agent")
	}

	data, err := RenderMCP(cfg, target)
	if err != nil {
		t.Fatalf("RenderMCP returned error: %v", err)
	}

	if bytes.Contains(data, []byte(`"env"`)) {
		t.Fatalf("expected empty env to be omitted from JSON output: %s", string(data))
	}
}

func TestRenderMCPTOML(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Servers: map[string]MCPServer{
			"postgres": {
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-postgres"},
				Env: map[string]string{
					"DATABASE_URL": "${DATABASE_URL}",
				},
			},
		},
	}

	target, ok := agent.ByID("codex")
	if !ok {
		t.Fatalf("expected codex agent")
	}

	data, err := RenderMCP(cfg, target)
	if err != nil {
		t.Fatalf("RenderMCP returned error: %v", err)
	}

	output := string(data)
	if !strings.Contains(output, "[mcp_servers.postgres]") {
		t.Fatalf("expected TOML server section, got: %s", output)
	}

	if !strings.Contains(output, `[mcp_servers.postgres.env]`) {
		t.Fatalf("expected TOML env section, got: %s", output)
	}

	if !strings.Contains(output, `DATABASE_URL = "${DATABASE_URL}"`) {
		t.Fatalf("expected TOML env entry, got: %s", output)
	}
}

func TestRenderMCPTOMLOmitsEmptyEnv(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{
		Servers: map[string]MCPServer{
			"filesystem": {
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
			},
		},
	}

	target, ok := agent.ByID("codex")
	if !ok {
		t.Fatalf("expected codex agent")
	}

	data, err := RenderMCP(cfg, target)
	if err != nil {
		t.Fatalf("RenderMCP returned error: %v", err)
	}

	if strings.Contains(string(data), ".env]") {
		t.Fatalf("expected empty env section to be omitted, got: %s", string(data))
	}
}

func TestRenderMCPUnsupportedFormat(t *testing.T) {
	t.Parallel()

	cfg := MCPConfig{Servers: map[string]MCPServer{}}
	target := agent.Agent{
		ID:        "custom",
		Name:      "Custom",
		MCPFormat: agent.MCPFormat("yaml"),
	}

	if _, err := RenderMCP(cfg, target); err == nil {
		t.Fatalf("expected unsupported format to return an error")
	}
}
