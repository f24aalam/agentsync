# Multi-Agent Example

Demonstrates configuration distribution to all seven supported AI coding agents from a single unified `.ai/` directory.

## Purpose

Different AI coding assistants expect instructions, skills, and tools in different locations and formats. This example models a fictional Go service ("Order Management Service") and tests distribution across all seven agents in AgentSync's registry:

1. **Claude Code** (`claude-code`)
2. **Cursor** (`cursor`)
3. **Codex** (`codex`)
4. **Gemini CLI** (`gemini-cli`)
5. **GitHub Copilot** (`github-copilot`)
6. **Junie** (`junie`)
7. **OpenCode** (`opencode`)

## Starting State

The canonical source files are organized inside `.ai/`:

```text
multi-agent/
├── README.md
└── .ai/
    ├── guidelines/
    │   ├── architecture.md
    │   └── coding-style.md
    ├── skills/
    │   ├── code-review/
    │   │   └── SKILL.md
    │   ├── testing/
    │   │   └── SKILL.md
    │   └── documentation/
    │       └── SKILL.md
    ├── mcp.toml
    └── sync.lock
```

The `.ai/sync.lock` file contains all seven agent IDs.

## Agent Capability and Format Matrix

| Agent | Guidelines Destination | Skills Destination | MCP Destination | Format |
|---|---|---|---|---|
| **Claude Code** | `CLAUDE.md` | `.claude/skills/` | `.mcp.json` | JSON (`mcpServers`) |
| **Cursor** | `.cursor/rules/agentsync.mdc` | `.cursor/skills/` | `.cursor/mcp.json` | JSON (`mcpServers`) |
| **Codex** | `AGENTS.md` *(shared)* | `.agents/skills/` *(shared)* | `.codex/config.toml` | TOML (`[mcp_servers]`) |
| **Gemini CLI** | `GEMINI.md` | *(not supported)* | `.gemini/settings.json` | JSON (`mcpServers`) |
| **GitHub Copilot** | `AGENTS.md` *(shared)* | `.github/skills/` | `.vscode/mcp.json` | JSON (`servers`) |
| **Junie** | `.junie/guidelines.md` | `.junie/skills/` | `.junie/mcp/mcp.json` | JSON (`mcpServers`) |
| **OpenCode** | `AGENTS.md` *(shared)* | `.agents/skills/` *(shared)* | `opencode.json` | JSON (`mcp` + `$schema`) |

### Key Architectural Behaviors

- **Shared Guidelines Deduplication:** Codex, GitHub Copilot, and OpenCode all share `AGENTS.md`. AgentSync merges canonical guidelines and writes the file once atomically.
- **Shared Skills Deduplication:** Codex and OpenCode both use `.agents/skills/`. AgentSync detects shared destination paths and avoids redundant copying.
- **Unsupported Features:** Gemini CLI does not support project-level skills (`SkillsSupported: false`); AgentSync skips skill installation for Gemini CLI without error.
- **Format Translation:** Canonical `.ai/mcp.toml` is automatically translated to TOML for Codex and agent-specific JSON schemas for the other six agents.
- **Project Isolation:** All generated configuration is strictly project-local; global configuration files in user home directories (`~/.copilot/`, `~/.claude/`) are never touched.

## Manual Verification Workflow

### 1. Build AgentSync

From the repository root:

```bash
go build -o /tmp/agentsync .
```

### 2. Copy the Example to a Disposable Location

```bash
mkdir -p /tmp/agentsync-manual
rm -rf /tmp/agentsync-manual/multi-agent
cp -R examples/multi-agent /tmp/agentsync-manual/multi-agent
cd /tmp/agentsync-manual/multi-agent
```

### 3. Run Installation

```bash
/tmp/agentsync install
```

### 4. Verify Generated Outputs

Check each expected destination:

```bash
# Guidelines
cat CLAUDE.md
cat .cursor/rules/agentsync.mdc
cat AGENTS.md
cat GEMINI.md
cat .junie/guidelines.md

# Skills
ls .claude/skills/
ls .cursor/skills/
ls .agents/skills/
ls .github/skills/
ls .junie/skills/

# Confirm Gemini CLI received no skills directory:
test ! -d .gemini/skills && echo "Gemini skills correctly omitted"

# MCP Configurations
cat .mcp.json
cat .cursor/mcp.json
cat .codex/config.toml
cat .gemini/settings.json
cat .vscode/mcp.json
cat .junie/mcp/mcp.json
cat opencode.json
```

### 5. Verify Repeated Execution

Run `install` again to verify idempotency:

```bash
/tmp/agentsync install
```

All steps should report success without modifying any files or prompting.

### 6. Reset the Example

```bash
rm -rf /tmp/agentsync-manual/multi-agent
cp -R examples/multi-agent /tmp/agentsync-manual/multi-agent
```

## Limitations and Notes

- The MCP definitions reference `uvx mcp-server-fetch` and `npx -y @modelcontextprotocol/server-filesystem .`. These are standard placeholder servers used to test config distribution without requiring live credentials.
