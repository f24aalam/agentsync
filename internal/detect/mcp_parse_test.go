package detect

import (
	"testing"

	"github.com/f24aalam/agentsync/internal/config"
)

func TestParseOpenCodeMCPLocal(t *testing.T) {
	data := []byte(`{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "my-local-mcp-server": {
      "type": "local",
      "command": ["godbmcp", "mcp", "--connection-id=postgres-1"],
      "enabled": true,
      "environment": {
        "API_KEY": "${MY_KEY}"
      }
    }
  }
}`)

	servers, err := parseOpenCodeMCP(data)
	if err != nil {
		t.Fatalf("parseOpenCodeMCP error: %v", err)
	}

	s, ok := servers["my-local-mcp-server"]
	if !ok {
		t.Fatalf("expected server key %q", "my-local-mcp-server")
	}

	if s.Command != "godbmcp" {
		t.Fatalf("expected command godbmcp, got %q", s.Command)
	}
	if len(s.Args) != 2 || s.Args[0] != "mcp" {
		t.Fatalf("expected args [mcp --connection-id=...], got %#v", s.Args)
	}
	if s.Env["API_KEY"] != "${MY_KEY}" {
		t.Fatalf("expected env.API_KEY to be preserved, got %q", s.Env["API_KEY"])
	}
}

func TestParseCopilotMCPStdio(t *testing.T) {
	data := []byte(`{
  "servers": {
    "name": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "pkg"],
      "env": {
        "API_KEY": "${input:api-key}"
      },
      "enabled": true
    }
  }
}`)

	servers, err := parseCopilotMCP(data)
	if err != nil {
		t.Fatalf("parseCopilotMCP error: %v", err)
	}

	s, ok := servers["name"]
	if !ok {
		t.Fatalf("expected server key %q", "name")
	}

	if s.Command != "npx" {
		t.Fatalf("expected command npx, got %q", s.Command)
	}
	if len(s.Args) != 2 || s.Args[0] != "-y" {
		t.Fatalf("expected args [-y pkg], got %#v", s.Args)
	}
	if s.Env["API_KEY"] != "${input:api-key}" {
		t.Fatalf("expected env.API_KEY to be preserved, got %q", s.Env["API_KEY"])
	}
}

func TestParseOpenCodeMCPEnvEmpty(t *testing.T) {
	data := []byte(`{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "local": {
      "type": "local",
      "command": ["echo"],
      "enabled": true
    }
  }
}`)

	servers, err := parseOpenCodeMCP(data)
	if err != nil {
		t.Fatalf("parseOpenCodeMCP error: %v", err)
	}

	s, ok := servers["local"]
	if !ok {
		t.Fatalf("expected server key %q", "local")
	}

	if s.Command != "echo" {
		t.Fatalf("expected command echo, got %q", s.Command)
	}
	if len(s.Args) != 0 {
		t.Fatalf("expected no args, got %#v", s.Args)
	}
	if s.Env != nil && len(s.Env) != 0 {
		t.Fatalf("expected empty env map, got %#v", s.Env)
	}
}

func TestParseOpenCodeMCPRemote(t *testing.T) {
	data := []byte(`{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "my-remote-mcp": {
      "type": "remote",
      "url": "https://mcp.example.com",
      "enabled": true,
      "headers": {
        "Authorization": "Bearer MY_API_KEY"
      },
      "oauth": {
        "clientId": "{env:CLIENT_ID}",
        "clientSecret": "{env:CLIENT_SECRET}",
        "scope": "tools:read"
      }
    }
  }
}`)

	servers, err := parseOpenCodeMCP(data)
	if err != nil {
		t.Fatalf("parseOpenCodeMCP error: %v", err)
	}

	s, ok := servers["my-remote-mcp"]
	if !ok {
		t.Fatalf("expected server key %q", "my-remote-mcp")
	}

	if s.Type != "remote" {
		t.Fatalf("expected type remote, got %q", s.Type)
	}
	if s.URL != "https://mcp.example.com" {
		t.Fatalf("expected url, got %q", s.URL)
	}
	if s.Headers["Authorization"] != "Bearer MY_API_KEY" {
		t.Fatalf("expected headers.Authorization preserved, got %q", s.Headers["Authorization"])
	}
	if s.OAuth == nil {
		t.Fatalf("expected oauth to be parsed")
	}
	if s.OAuth.ClientID != "{env:CLIENT_ID}" {
		t.Fatalf("expected oauth.clientId, got %q", s.OAuth.ClientID)
	}
	if s.OAuth.ClientSecret != "{env:CLIENT_SECRET}" {
		t.Fatalf("expected oauth.clientSecret, got %q", s.OAuth.ClientSecret)
	}
	if s.OAuth.Scope != "tools:read" {
		t.Fatalf("expected oauth.scope, got %q", s.OAuth.Scope)
	}
}

// Ensure compile-time import usage of config in this file (helps
// if future refactors remove direct references).
var _ = config.MCPServer{}

