package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/BurntSushi/toml"
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
		payload := tomlMCPConfig{MCPServers: sanitizeServers(cfg.Servers)}

		var buf bytes.Buffer
		if err := toml.NewEncoder(&buf).Encode(payload); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
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
