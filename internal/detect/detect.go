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
	path := filepath.Join(root, target.MCPConfig)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	var servers map[string]config.MCPServer
	switch target.MCPFormat {
	case agent.MCPFormatJSON:
		var payload struct {
			MCPServers map[string]config.MCPServer `json:"mcpServers"`
		}

		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}

		servers = payload.MCPServers
	case agent.MCPFormatTOML:
		var payload struct {
			MCPServers map[string]config.MCPServer `toml:"mcp_servers"`
		}

		if err := toml.Unmarshal(data, &payload); err != nil {
			return nil, err
		}

		servers = payload.MCPServers
	default:
		return nil, nil
	}

	var out []DetectedMCPServer
	for name, server := range servers {
		out = append(out, DetectedMCPServer{
			Agent:  target,
			Name:   name,
			Server: normalizeServer(server),
		})
	}

	return out, nil
}

func normalizeServer(server config.MCPServer) config.MCPServer {
	server.Command = strings.TrimSpace(server.Command)

	return server
}
