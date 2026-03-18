package agent

type MCPFormat string

const (
	MCPFormatJSON MCPFormat = "json"
	MCPFormatTOML MCPFormat = "toml"
)

type Agent struct {
	ID             string
	Name           string
	GuidelinesFile string
	SkillsDir      string
	MCPConfig      string
	MCPFormat      MCPFormat
}

var registry = []Agent{
	{
		ID:             "claude-code",
		Name:           "Claude Code",
		GuidelinesFile: "CLAUDE.md",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      ".mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "cursor",
		Name:           "Cursor",
		GuidelinesFile: ".cursor/rules/*.mdc",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      ".cursor/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "codex",
		Name:           "Codex",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      ".codex/config.toml",
		MCPFormat:      MCPFormatTOML,
	},
	{
		ID:             "gemini-cli",
		Name:           "Gemini CLI",
		GuidelinesFile: "GEMINI.md",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      ".gemini/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "github-copilot",
		Name:           "GitHub Copilot",
		GuidelinesFile: ".github/copilot-instructions.md",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      ".vscode/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "junie",
		Name:           "Junie",
		GuidelinesFile: ".junie/guidelines.md",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      ".junie/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "opencode",
		Name:           "OpenCode",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".agents/skills/",
		MCPConfig:      ".opencode/opencode.json",
		MCPFormat:      MCPFormatJSON,
	},
}

func All() []Agent {
	agents := make([]Agent, len(registry))
	copy(agents, registry)

	return agents
}

func ByID(id string) (Agent, bool) {
	for _, agent := range registry {
		if agent.ID == id {
			return agent, true
		}
	}

	return Agent{}, false
}

// UniqueSkillsDirs returns a deduplicated slice of skill directories from the given agents.
// Since multiple agents may share the same SkillsDir, this prevents duplicate copying.
func UniqueSkillsDirs(agents []Agent) []string {
	seen := make(map[string]bool)
	var dirs []string
	for _, agent := range agents {
		if !seen[agent.SkillsDir] {
			seen[agent.SkillsDir] = true
			dirs = append(dirs, agent.SkillsDir)
		}
	}

	return dirs
}
