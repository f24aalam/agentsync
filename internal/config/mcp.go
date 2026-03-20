package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

type MCPServer struct {
	// Type controls how the MCP connection is established.
	// For backward compatibility, if Type is empty we'll treat it as "local".
	Type string `toml:"type" json:"type,omitempty"`

	// Local/stdio fields.
	Command string   `toml:"command" json:"command,omitempty"`
	Args    []string `toml:"args" json:"args,omitempty"`
	Env     map[string]string `toml:"env" json:"env,omitempty"`

	// Remote/http fields.
	URL     string            `toml:"url" json:"url,omitempty"`
	Headers map[string]string `toml:"headers" json:"headers,omitempty"`
	OAuth   *OAuthConfig      `toml:"oauth" json:"oauth,omitempty"`
}

type OAuthConfig struct {
	ClientID     string `toml:"clientId" json:"clientId,omitempty"`
	ClientSecret string `toml:"clientSecret" json:"clientSecret,omitempty"`
	Scope        string `toml:"scope" json:"scope,omitempty"`
}

type MCPConfig struct {
	Servers map[string]MCPServer `toml:"servers"`
}

func ReadMCP(path string) (MCPConfig, error) {
	var cfg MCPConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return MCPConfig{}, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return MCPConfig{}, err
	}

	if cfg.Servers == nil {
		cfg.Servers = map[string]MCPServer{}
	}

	return cfg, nil
}
