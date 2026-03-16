package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

type jsonMCPConfig struct {
	MCPServers map[string]MCPServer `json:"mcpServers"`
}

type tomlMCPConfig struct {
	MCPServers map[string]MCPServer `toml:"mcp_servers"`
}

func RenderMCP(cfg MCPConfig, format string) ([]byte, error) {
	switch format {
	case "json":
		payload := jsonMCPConfig{MCPServers: sanitizeServers(cfg.Servers)}
		return json.MarshalIndent(payload, "", "  ")
	case "toml":
		return renderTomlMCP(sanitizeServers(cfg.Servers))
	default:
		return nil, fmt.Errorf("unsupported MCP format: %s", format)
	}
}

func sanitizeServers(servers map[string]MCPServer) map[string]MCPServer {
	if len(servers) == 0 {
		return map[string]MCPServer{}
	}

	sanitized := make(map[string]MCPServer, len(servers))
	for name, server := range servers {
		copyServer := server
		if len(copyServer.Env) == 0 {
			copyServer.Env = nil
		}
		sanitized[name] = copyServer
	}

	return sanitized
}

func renderTomlMCP(servers map[string]MCPServer) ([]byte, error) {
	var buf bytes.Buffer
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		server := servers[name]
		if i > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(fmt.Sprintf("[mcp_servers.%s]\n", name))
		buf.WriteString(fmt.Sprintf("command = %q\n", server.Command))
		if len(server.Args) > 0 {
			buf.WriteString("args = [")
			for idx, arg := range server.Args {
				if idx > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(fmt.Sprintf("%q", arg))
			}
			buf.WriteString("]\n")
		}
		if len(server.Env) > 0 {
			buf.WriteString(fmt.Sprintf("\n[mcp_servers.%s.env]\n", name))
			keys := make([]string, 0, len(server.Env))
			for key := range server.Env {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				buf.WriteString(fmt.Sprintf("%s = %q\n", key, server.Env[key]))
			}
		}
	}

	return buf.Bytes(), nil
}
