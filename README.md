# agentsync

```text
 █████╗  ██████╗ ███████╗███╗   ██╗████████╗███████╗██╗   ██╗███╗   ██╗ ██████╗
██╔══██╗██╔════╝ ██╔════╝████╗  ██║╚══██╔══╝██╔════╝╚██╗ ██╔╝████╗  ██║██╔════╝
███████║██║  ███╗█████╗  ██╔██╗ ██║   ██║   ███████╗ ╚████╔╝ ██╔██╗ ██║██║     
██╔══██║██║   ██║██╔══╝  ██║╚██╗██║   ██║   ╚════██║  ╚██╔╝  ██║╚██╗██║██║     
██║  ██║╚██████╔╝███████╗██║ ╚████║   ██║   ███████║   ██║   ██║ ╚████║╚██████╗
╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚══════╝   ╚═╝   ╚═╝  ╚═══╝ ╚═════╝
                                                                               
```

Sync AI guidelines, skills, and MCP configuration across coding agents from one source of truth.

`agentsync` lets you define project AI configuration once in a local `.ai/` directory, then generate the agent-specific files different tools expect, such as `CLAUDE.md`, `AGENTS.md`, `.cursor/rules/agentsync.mdc`, `.codex/config.toml`, and more.

## Why

Modern agent workflows are fragmented:

- one tool wants `CLAUDE.md`
- another wants `AGENTS.md`
- another wants `.cursor/rules/*.mdc`
- MCP config is JSON in most places, TOML in others
- skills live in different directories depending on the agent

`agentsync` removes that duplication.

You author everything once in `.ai/`, then run:

```bash
agentsync install
```

and the project-specific outputs are generated in the right places.

## What It Manages

Inside your project root:

```text
.ai/
├── guidelines/   # markdown files, merged into agent guideline files
├── skills/       # one folder per skill, copied into each agent's skills dir
└── mcp.toml      # MCP server definitions, rendered to agent-specific formats
```

`agentsync` also writes:

```text
.ai/sync.lock
```

which records the selected target agents for the project.

## Supported Agents

| Agent | ID | Guidelines Output | Skills Output | MCP Output |
| --- | --- | --- | --- | --- |
| Claude Code | `claude-code` | `CLAUDE.md` | `.claude/skills/` | `.mcp.json` |
| Cursor | `cursor` | `.cursor/rules/agentsync.mdc` | `.cursor/skills/` | `.cursor/mcp.json` |
| Codex | `codex` | `AGENTS.md` | `.agents/skills/` | `.codex/config.toml` |
| Gemini CLI | `gemini-cli` | `GEMINI.md` | *(not supported)* | `.gemini/settings.json` |
| GitHub Copilot | `github-copilot` | `AGENTS.md` | `.github/skills/` | `.vscode/mcp.json` |
| Junie | `junie` | `.junie/guidelines.md` | `.junie/skills/` | `.junie/mcp/mcp.json` |
| OpenCode | `opencode` | `AGENTS.md` | `.agents/skills/` | `opencode.json` |

## Installation

### Linux/macOS
```bash
curl -fsSL https://raw.githubusercontent.com/f24aalam/agentsync/main/install.sh | bash
```

### Windows (PowerShell)
```powershell
irm https://raw.githubusercontent.com/f24aalam/agentsync/main/install.ps1 | iex
```

Both scripts install the latest published GitHub release. To build the current `main` branch instead, follow the local build instructions below.

### Build Locally

```bash
git clone --branch main https://github.com/f24aalam/agentsync.git
cd agentsync
go build -o agentsync .
```

### Run Without Installing

```bash
go run . --help
```

## Quick Start

### 1. Initialize `.ai/`

```bash
./agentsync init
```

The interactive TUI will ask for:

- project name
- whether to add a core guidelines file
- whether to add a sample skill
- whether to add MCP config
- which agents to target
- whether generated agent files should be added to `.gitignore`

### 2. Edit Your Source Files

Example:

```text
.ai/
├── guidelines/
│   ├── core.md
│   └── api-style.md
├── skills/
│   └── writing-migrations/
│       ├── SKILL.md
│       └── examples/
└── mcp.toml
```

### 3. Generate Agent Files

```bash
./agentsync install
```

### 4. Re-sync After Changes

```bash
./agentsync install
```

## Commands

AgentSync v1 intentionally exposes only two commands:

### `agentsync init`

Scaffolds `.ai/` and configures target agents in `.ai/sync.lock`.

**Fresh project:**
- Detects existing AI coding agent configurations, skills, guidelines, and MCP servers.
- Deduplicates identical content and lets you choose what to import.
- Lets you select target AI coding agents and saves them to `.ai/sync.lock`.
- Original configuration files are preserved intact.

**Existing project (`.ai/` already initialized):**
- Safely detects existing initialization.
- Preserves all canonical guidelines, skills, and MCP configurations without reimporting or deleting.
- Prompts with your currently configured agents pre-selected, allowing you to update or add agents cleanly.

### `agentsync install`

Reads `.ai/sync.lock` and distributes configurations from `.ai/` to every selected coding agent.

Behavior:
- **Guidelines**: Merges all `.md` files in `.ai/guidelines/` deterministically into managed sections (`<!-- BEGIN AGENTSYNC MANAGED -->` ... `<!-- END AGENTSYNC MANAGED -->`), preserving any surrounding user instructions. Shared destinations (like `AGENTS.md`) are deduplicated and resolved as a single atomic target.
- **Skills**: Copies each skill package directory from `.ai/skills/` preserving supporting files and relative symlinks.
- **MCP**: Translates `.ai/mcp.toml` into agent-specific configurations (JSON or TOML), preserving existing unmanaged servers.
- **Conflict Handling**: Prompts before overwriting modified or unmanaged content. Destinations matching desired output are skipped automatically.

Non-interactive / CI:

```bash
agentsync install --yes
# or
agentsync install -y
```

`--yes` approves ordinary managed replacements without interactive prompts, while still refusing to corrupt or delete unmanaged content.

## Example Workflow

### Source of Truth

```toml
# .ai/mcp.toml
[servers.postgres]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]

[servers.postgres.env]
DATABASE_URL = "${DATABASE_URL}"
```

```md
# .ai/guidelines/core.md
# Project Guidelines
```

### Generated Output

After `agentsync install`, you may get:

```text
CLAUDE.md
.cursor/rules/agentsync.mdc
.codex/config.toml
.gemini/settings.json
.agents/skills/example-skill/
```

## MCP Format Handling

`agentsync` automatically renders MCP config in the correct format:

- JSON for Claude Code, Cursor, Gemini CLI, GitHub Copilot, Junie, and OpenCode
- TOML for Codex

You keep one canonical `.ai/mcp.toml`; `agentsync` handles the translation.

## Gitignore Behavior

During `agentsync init`, you can choose to add generated agent files to `.gitignore`.

This keeps the repository focused on the source configuration in `.ai/`, while leaving generated files like these untracked:

- `.cursor/`
- `.codex/`
- `.junie/`
- `.opencode/`
- `AGENTS.md`
- `CLAUDE.md`

The exact entries depend on the agents selected during initialization.

## Current Model

Today, `agentsync` writes agent outputs as project-relative files inside the repository.

That means it generates files like:

- `AGENTS.md`
- `.cursor/rules/agentsync.mdc`
- `.codex/config.toml`

directly in the project tree, rather than installing into user home directories.

## Development

Useful commands:

```bash
go test ./...
go build ./...
go vet ./...
```

## Tech Stack

- Go
- `spf13/cobra`
- `charmbracelet/lipgloss`
- `f24aalam/stepflow` (init wizard and install prompts; Bubble Tea)
- `BurntSushi/toml`

## Status

The CLI is functional end to end:

- `init`
- `install`

The current implementation focuses on a clean local-project workflow with deterministic generated outputs and comprehensive test coverage for the core sync pipeline.
