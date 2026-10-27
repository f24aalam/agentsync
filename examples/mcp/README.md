# MCP Configuration Example

Demonstrates translation and distribution of canonical Model Context Protocol (MCP) server configurations into agent-specific JSON and TOML formats.

## Purpose

Different AI coding agents expect MCP server definitions in disparate configuration file formats:
- **JSON format (`mcpServers`):** Claude Code (`.mcp.json`), Cursor (`.cursor/mcp.json`), Gemini CLI (`.gemini/settings.json`), Junie (`.junie/mcp/mcp.json`).
- **TOML format (`[mcp_servers]`):** Codex (`.codex/config.toml`).
- **OpenCode schema (`mcp`):** OpenCode (`opencode.json`).
- **VS Code format (`servers`):** GitHub Copilot (`.vscode/mcp.json`).

Authoring these definitions manually in multiple files leads to drift and syntax discrepancies. AgentSync allows teams to maintain one canonical `.ai/mcp.toml` and automatically renders the appropriate format for each target agent.

## Starting State

```text
mcp/
├── README.md
└── .ai/
    ├── mcp.toml
    └── sync.lock
```

Target agents in `.ai/sync.lock`:
- `claude-code` (tests standard JSON `.mcp.json`)
- `codex` (tests TOML `.codex/config.toml`)
- `opencode` (tests OpenCode schema `opencode.json`)

Canonical servers in `.ai/mcp.toml`:
1. `fetch`: lightweight command using `uvx mcp-server-fetch`.
2. `filesystem`: local stdio server using `npx @modelcontextprotocol/server-filesystem` with custom environment variables (`WORKSPACE_ROOT`, `LOG_LEVEL`).

## Manual Verification Workflow

### 1. Build AgentSync

From the repository root:

```bash
go build -o /tmp/agentsync .
```

### 2. Copy the Example to a Disposable Location

```bash
mkdir -p /tmp/agentsync-manual
rm -rf /tmp/agentsync-manual/mcp
cp -R examples/mcp /tmp/agentsync-manual/mcp
cd /tmp/agentsync-manual/mcp
```

### 3. Run Initial Installation

```bash
/tmp/agentsync install
```

### 4. Inspect Generated MCP Configurations

Verify that each agent destination received the correct format:

#### Claude Code (`.mcp.json` - JSON):
```bash
cat .mcp.json
```
Expected output:
```json
{
  "mcpServers": {
    "fetch": {
      "args": [
        "mcp-server-fetch"
      ],
      "command": "uvx"
    },
    "filesystem": {
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "."
      ],
      "command": "npx",
      "env": {
        "LOG_LEVEL": "info",
        "WORKSPACE_ROOT": "."
      }
    }
  }
}
```

#### Codex (`.codex/config.toml` - TOML):
```bash
cat .codex/config.toml
```
Expected output:
```toml
[mcp_servers.fetch]
command = "uvx"
args = ["mcp-server-fetch"]

[mcp_servers.filesystem]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "."]

[mcp_servers.filesystem.env]
LOG_LEVEL = "info"
WORKSPACE_ROOT = "."
```

#### OpenCode (`opencode.json` - OpenCode Schema):
```bash
cat opencode.json
```
Expected output includes `"$schema": "https://opencode.ai/config.json"`, `"type": "local"`, and an `"environment"` map.

### 5. Verify Idempotency

Re-run installation:

```bash
/tmp/agentsync install
```

**Expected behavior:** No configuration changes are made and no prompts are shown.

### 6. Verify Adding a New Server

Add a third server to `.ai/mcp.toml`:

```toml
[servers.sqlite]
type = "local"
command = "uvx"
args = ["mcp-server-sqlite", "--db-path", "app.db"]
```

Re-run installation:

```bash
/tmp/agentsync install --yes
```

**Expected behavior:** All three agent files (`.mcp.json`, `.codex/config.toml`, `opencode.json`) are updated to include the new `sqlite` server alongside the existing `fetch` and `filesystem` servers.

### 7. Verify Preservation of Unmanaged Existing Settings

Simulate user-owned custom configuration in `.codex/config.toml`:

```toml
# Developer local configuration
model = "gpt-4o"
temperature = 0.2
```

Prepend this to `.codex/config.toml` and re-run:

```bash
/tmp/agentsync install --yes
```

**Expected behavior:** The user-owned `model` and `temperature` settings outside `[mcp_servers]` are preserved intact.

### 8. Reset the Example

```bash
rm -rf /tmp/agentsync-manual/mcp
cp -R examples/mcp /tmp/agentsync-manual/mcp
```

## Limitations and Notes

- AgentSync only translates and installs configuration files. It does not spin up or manage the background lifecycle of MCP servers.
- The example commands use standard placeholder binaries (`uvx`, `npx`). No external network connection or running daemon is required for installation.
