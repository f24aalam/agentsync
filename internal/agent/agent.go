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
	// SkillsDir is where this agent expects skills to live.
	// If SkillsSupported is false, SkillsDir may be ignored by installers/detectors.
	SkillsDir      string
	SkillsSupported bool
	// MCPConfig is the primary MCP config file path used by detectors/handlers.
	MCPConfig      string
	// MCPConfigs is an optional list of additional MCP destinations.
	// Used for agents that need multiple MCP outputs (e.g. Copilot).
	MCPConfigs     []string
	MCPFormat      MCPFormat
}

var registry = []Agent{
	{
		ID:             "claude-code",
		Name:           "Claude Code",
		GuidelinesFile: "CLAUDE.md",
		SkillsDir:      ".claude/skills/",
		SkillsSupported: true,
		MCPConfig:      ".mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "cursor",
		Name:           "Cursor",
		GuidelinesFile: ".cursor/rules/*.mdc",
		SkillsDir:      ".cursor/skills/",
		SkillsSupported: true,
		MCPConfig:      ".cursor/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "codex",
		Name:           "Codex",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".agents/skills/",
		SkillsSupported: true,
		MCPConfig:      ".codex/config.toml",
		MCPFormat:      MCPFormatTOML,
	},
	{
		ID:             "gemini-cli",
		Name:           "Gemini CLI",
		GuidelinesFile: "GEMINI.md",
		SkillsDir:      "",
		SkillsSupported: false,
		MCPConfig:      ".gemini/settings.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "github-copilot",
		Name:           "GitHub Copilot",
		// Per your spec, Copilot only needs AGENTS.md for guidelines.
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".github/skills/",
		SkillsSupported: true,
		// Write/read VS Code MCP config for now; install will also support CLI MCP via MCPConfigs.
		MCPConfig:      ".vscode/mcp.json",
		MCPConfigs:     []string{"~/.copilot/mcp-config.json"},
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "junie",
		Name:           "Junie",
		GuidelinesFile: ".junie/guidelines.md",
		SkillsDir:      ".junie/skills/",
		SkillsSupported: true,
		MCPConfig:      ".junie/mcp/mcp.json",
		MCPFormat:      MCPFormatJSON,
	},
	{
		ID:             "opencode",
		Name:           "OpenCode",
		GuidelinesFile: "AGENTS.md",
		SkillsDir:      ".agents/skills/",
		SkillsSupported: true,
		MCPConfig:      "opencode.json",
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
