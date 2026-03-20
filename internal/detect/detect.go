package detect

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/config"
)

type DetectedGuideline struct {
	Agent   agent.Agent
	Path    string
	Content string
}

type DetectedSkill struct {
	Agent   agent.Agent
	Name    string
	Path    string
	SkillMD string
}

type DetectedMCPServer struct {
	Agent  agent.Agent
	Name   string
	Server config.MCPServer
}

type ProjectDetection struct {
	Guidelines []DetectedGuideline
	Skills     []DetectedSkill
	MCPServers []DetectedMCPServer
}

func (p ProjectDetection) Empty() bool {
	return len(p.Guidelines) == 0 && len(p.Skills) == 0 && len(p.MCPServers) == 0
}

func ScanProject(root string) (ProjectDetection, error) {
	var out ProjectDetection
	agents := agent.All()

	for _, target := range agents {
		guidelines, err := detectGuidelines(root, target)
		if err != nil {
			return ProjectDetection{}, err
		}
		out.Guidelines = append(out.Guidelines, guidelines...)

		skills, err := detectSkills(root, target)
		if err != nil {
			return ProjectDetection{}, err
		}
		out.Skills = append(out.Skills, skills...)

		mcpServers, err := detectMCP(root, target)
		if err != nil {
			return ProjectDetection{}, err
		}
		out.MCPServers = append(out.MCPServers, mcpServers...)
	}

	sharedSkills, err := detectSharedSkills(root)
	if err != nil {
		return ProjectDetection{}, err
	}
	out.Skills = append(out.Skills, sharedSkills...)

	return out, nil
}

func detectGuidelines(root string, target agent.Agent) ([]DetectedGuideline, error) {
	if target.ID == "cursor" {
		return detectCursorGuidelines(root, target)
	}

	path := filepath.Join(root, target.GuidelinesFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	return []DetectedGuideline{
		{
			Agent:   target,
			Path:    path,
			Content: string(data),
		},
	}, nil
}

func detectCursorGuidelines(root string, target agent.Agent) ([]DetectedGuideline, error) {
	glob := filepath.Join(root, ".cursor", "rules", "*.mdc")
	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}

	var out []DetectedGuideline
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		out = append(out, DetectedGuideline{
			Agent:   target,
			Path:    path,
			Content: string(data),
		})
	}

	return out, nil
}

func detectSkills(root string, target agent.Agent) ([]DetectedSkill, error) {
	if !target.SkillsSupported || strings.TrimSpace(target.SkillsDir) == "" {
		return nil, nil
	}

	dir := filepath.Join(root, target.SkillsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	var out []DetectedSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, entry.Name())
		skillMD := filepath.Join(skillDir, "SKILL.md")
		data, err := os.ReadFile(skillMD)

		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}

		out = append(out, DetectedSkill{
			Agent:   target,
			Name:    entry.Name(),
			Path:    skillDir,
			SkillMD: string(data),
		})
	}

	return out, nil
}

func detectLegacySkills(root string) ([]DetectedSkill, error) {
	dir := filepath.Join(root, ".agents", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	legacyAgent := agent.Agent{
		ID:   "legacy-agents",
		Name: ".agents",
	}

	var out []DetectedSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, entry.Name())
		skillMD := filepath.Join(skillDir, "SKILL.md")

		data, err := os.ReadFile(skillMD)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				out = append(out, DetectedSkill{
					Agent:   legacyAgent,
					Name:    entry.Name(),
					Path:    skillDir,
					SkillMD: "",
				})

				continue
			}

			return nil, err
		}

		out = append(out, DetectedSkill{
			Agent:   legacyAgent,
			Name:    entry.Name(),
			Path:    skillDir,
			SkillMD: string(data),
		})
	}

	return out, nil
}

// detectSharedSkills scans the shared .agents/skills directory and returns skills
// tagged with a shared agent label for backward compatibility during migration.
func detectSharedSkills(root string) ([]DetectedSkill, error) {
	dir := filepath.Join(root, ".agents", "skills")

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	sharedAgent := agent.Agent{
		ID:   "agents-shared",
		Name: "Shared (.agents)",
	}

	var out []DetectedSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, entry.Name())
		skillMD := filepath.Join(skillDir, "SKILL.md")

		data, err := os.ReadFile(skillMD)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				out = append(out, DetectedSkill{
					Agent:   sharedAgent,
					Name:    entry.Name(),
					Path:    skillDir,
					SkillMD: "",
				})

				continue
			}

			return nil, err
		}

		out = append(out, DetectedSkill{
			Agent:   sharedAgent,
			Name:    entry.Name(),
			Path:    skillDir,
			SkillMD: string(data),
		})
	}

	return out, nil
}

func detectMCP(root string, target agent.Agent) ([]DetectedMCPServer, error) {
	paths := make([]string, 0, 1+len(target.MCPConfigs))
	if strings.TrimSpace(target.MCPConfig) != "" {
		paths = append(paths, target.MCPConfig)
	}
	paths = append(paths, target.MCPConfigs...)

	var out []DetectedMCPServer
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}

		realPath := p
		if strings.HasPrefix(p, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			realPath = filepath.Join(home, strings.TrimPrefix(p, "~/"))
		} else if !filepath.IsAbs(p) {
			realPath = filepath.Join(root, p)
		}

		data, err := os.ReadFile(realPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}

		servers, err := parseMCPForAgent(target, data)
		if err != nil {
			return nil, err
		}

		for name, server := range servers {
			// We only support local/stdio servers in the canonical MCP model for now.
			if strings.TrimSpace(server.Command) == "" {
				continue
			}

			out = append(out, DetectedMCPServer{
				Agent:  target,
				Name:   name,
				Server: normalizeServer(server),
			})
		}
	}

	return out, nil
}

func normalizeServer(server config.MCPServer) config.MCPServer {
	server.Command = strings.TrimSpace(server.Command)

	return server
}

func parseMCPForAgent(target agent.Agent, data []byte) (map[string]config.MCPServer, error) {
	switch target.ID {
	case "opencode":
		return parseOpenCodeMCP(data)
	case "github-copilot":
		return parseCopilotMCP(data)
	}

	switch target.MCPFormat {
	case agent.MCPFormatJSON:
		// Default JSON shape for Claude/Cursor/Gemini/Junie.
		var payload struct {
			MCPServers map[string]config.MCPServer `json:"mcpServers"`
			Servers    map[string]config.MCPServer `json:"servers"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		if payload.MCPServers != nil && len(payload.MCPServers) > 0 {
			return payload.MCPServers, nil
		}
		if payload.Servers != nil && len(payload.Servers) > 0 {
			return payload.Servers, nil
		}
		return map[string]config.MCPServer{}, nil

	case agent.MCPFormatTOML:
		var payload struct {
			MCPServers map[string]config.MCPServer `toml:"mcp_servers"`
		}
		if err := toml.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		if payload.MCPServers == nil {
			return map[string]config.MCPServer{}, nil
		}
		return payload.MCPServers, nil
	}

	return map[string]config.MCPServer{}, nil
}

type openCodeMCPServer struct {
	Type    string   `json:"type"`
	Enabled bool     `json:"enabled"`

	// local
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment"`

	// remote
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	OAuth   *openCodeOAuth    `json:"oauth,omitempty"`
}

type openCodeOAuth struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Scope        string `json:"scope"`
}

func parseOpenCodeMCP(data []byte) (map[string]config.MCPServer, error) {
	var payload struct {
		MCP map[string]openCodeMCPServer `json:"mcp"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	out := make(map[string]config.MCPServer, len(payload.MCP))
	for name, srv := range payload.MCP {
		// Enabled is optional in schema; if it's false, treat as disabled.
		if srv.Enabled == false {
			continue
		}

		switch srv.Type {
		case "local":
			if len(srv.Command) == 0 {
				continue
			}

			command := srv.Command[0]
			args := []string{}
			if len(srv.Command) > 1 {
				args = append(args, srv.Command[1:]...)
			}

			var env map[string]string
			if len(srv.Environment) > 0 {
				env = srv.Environment
			}

			out[name] = config.MCPServer{
				Type:    "local",
				Command: command,
				Args:    args,
				Env:     env,
			}

		case "remote":
			if strings.TrimSpace(srv.URL) == "" {
				continue
			}

			var headers map[string]string
			if len(srv.Headers) > 0 {
				headers = srv.Headers
			}

			var oauth *config.OAuthConfig
			if srv.OAuth != nil {
				oauth = &config.OAuthConfig{
					ClientID:     srv.OAuth.ClientID,
					ClientSecret: srv.OAuth.ClientSecret,
					Scope:        srv.OAuth.Scope,
				}
			}

			out[name] = config.MCPServer{
				Type:    "remote",
				URL:     srv.URL,
				Headers: headers,
				OAuth:   oauth,
			}
		default:
			// Unknown type: ignore.
			continue
		}
	}

	return out, nil
}

type copilotMCPServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Enabled bool              `json:"enabled"`
}

func parseCopilotMCP(data []byte) (map[string]config.MCPServer, error) {
	var payload struct {
		Servers map[string]copilotMCPServer `json:"servers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	out := make(map[string]config.MCPServer, len(payload.Servers))
	for name, srv := range payload.Servers {
		// Skip remote entries (we only map stdio-local command/args/env right now).
		if srv.Command == "" {
			continue
		}

		var env map[string]string
		if len(srv.Env) > 0 {
			env = srv.Env
		}

		out[name] = config.MCPServer{
			Command: srv.Command,
			Args:    srv.Args,
			Env:     env,
		}
	}

	return out, nil
}
