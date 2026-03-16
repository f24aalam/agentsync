package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

type MCPServer struct {
	Command string            `toml:"command" json:"command"`
	Args    []string          `toml:"args" json:"args,omitempty"`
	Env     map[string]string `toml:"env" json:"env,omitempty"`
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
