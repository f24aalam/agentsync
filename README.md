# agentsync

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
| Claude Code | `claude-code` | `CLAUDE.md` | `.agents/skills/` | `.mcp.json` |
| Cursor | `cursor` | `.cursor/rules/agentsync.mdc` | `.agents/skills/` | `.cursor/mcp.json` |
| Codex | `codex` | `AGENTS.md` | `.agents/skills/` | `.codex/config.toml` |
| Gemini CLI | `gemini-cli` | `GEMINI.md` | `.agents/skills/` | `.gemini/mcp.json` |
| GitHub Copilot | `github-copilot` | `.github/copilot-instructions.md` | `.agents/skills/` | `.vscode/mcp.json` |
| Junie | `junie` | `.junie/guidelines.md` | `.agents/skills/` | `.junie/mcp.json` |
| OpenCode | `opencode` | `AGENTS.md` | `.agents/skills/` | `.opencode/opencode.json` |

## Installation

### Linux/macOS
```bash
curl -fsSL https://raw.githubusercontent.com/f24aalam/agentsync/master/install.sh | bash
```

### Windows (PowerShell)
```powershell
irm https://raw.githubusercontent.com/f24aalam/agentsync/master/install.ps1 | iex
```

### Build Locally

```bash
git clone <your-repo-url>
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

### 5. Inspect What `agentsync` Sees

```bash
./agentsync list
```

## Commands

### `agentsync init`

Scaffolds `.ai/` and writes `.ai/sync.lock`.

Features:

- interactive TUI powered by [stepflow](https://github.com/f24aalam/stepflow) (Bubble Tea)
- safe overwrite confirmation if `.ai/` already exists
- optional `.gitignore` updates for generated agent files
- project name detection from the current directory

### `agentsync install`

Reads `.ai/sync.lock` and generates agent outputs for every selected agent.

Behavior:

- merges all `.md` files in `.ai/guidelines/` alphabetically
- copies each skill directory from `.ai/skills/`
- parses `.ai/mcp.toml` and renders JSON or TOML depending on the target agent
- continues installing remaining agents even if one agent fails

When a target file or directory **already has content** and `agentsync` would write there, you get an interactive prompt (via [stepflow](https://github.com/f24aalam/stepflow)) per category:

- **Guidelines** — per agent (e.g. existing `AGENTS.md`, Cursor rules file)
- **Skills** — once per shared output directory (e.g. `.agents/skills/` used by several agents), listing which agent IDs share it
- **MCP** — per agent if any of that agent’s MCP config paths already exist

Answering **No** (default) **skips** that step; **Yes** overwrites or merges as usual. If there is nothing to generate from `.ai/` for a category, no prompt is shown for that category.

Non-interactive / CI:

```bash
agentsync install --yes
# or
agentsync install -y
```

This applies all writes without prompts.

### `agentsync list`

Displays a clean summary of what exists in `.ai/`:

- guideline files
- skill folders
- MCP server names
- selected agents from `sync.lock`

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
.gemini/mcp.json
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

`go.mod` contains `replace github.com/f24aalam/stepflow => ../stepflow` so agentsync builds against the sibling **stepflow** repo in this workspace. That version adds `WithAltScreen(false)`, which keeps the pre-TUI banner visible (the published module uses the alternate screen by default, which hides prior stderr output until exit). If you clone **only** `agentsync`, either check out **stepflow** beside it or remove the `replace` after a published stepflow release includes `WithAltScreen`.

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
- `list`

The current implementation focuses on a clean local-project workflow with deterministic generated outputs and test coverage for the core sync pipeline.
