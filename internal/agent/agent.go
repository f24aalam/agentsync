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
		SkillsDir:      ".claude/skills/",
		MCPConfig:      ".mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "cursor",
		Name:           "Cursor",
		GuidelinesFile: ".cursor/rules/*.mdc",
		SkillsDir:      ".cursor/skills/",
		MCPConfig:      ".cursor/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "codex",
		Name:           "Codex",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".codex/skills/",
		MCPConfig:      ".codex/config.toml",
		MCPFormat:      MCPFormatTOML,
	},
	{
		ID:             "gemini-cli",
		Name:           "Gemini CLI",
		GuidelinesFile: "GEMINI.md",
		SkillsDir:      ".gemini/skills/",
		MCPConfig:      ".gemini/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "github-copilot",
		Name:           "GitHub Copilot",
		GuidelinesFile: ".github/copilot-instructions.md",
		SkillsDir:      ".github/skills/",
		MCPConfig:      ".vscode/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "junie",
		Name:           "Junie",
		GuidelinesFile: ".junie/guidelines.md",
		SkillsDir:      ".junie/skills/",
		MCPConfig:      ".junie/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "opencode",
		Name:           "OpenCode",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".opencode/skills/",
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
