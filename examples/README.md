# AgentSync Examples & Manual Testing Suite

This directory contains self-contained example projects that demonstrate and manually test the major workflows of AgentSync v1.

These examples serve two purposes:
1. **Manual Testing & Regression Verification:** Verify `agentsync init`, `agentsync install`, idempotency, and non-destructive conflict handling.
2. **User Reference Documentation:** Show real-world configurations across all supported AI coding agents.

---

## Example Index

| Example | Main Purpose | Initial State | Key Agents Tested |
|---|---|---|---|
| [`basic/`](./basic/) | First installation from canonical `.ai/` | Canonical `.ai/` directory | Claude Code, Cursor |
| [`existing-config/`](./existing-config/) | Onboarding & importing scattered agent configs | No `.ai/` initially (existing agent files) | Claude Code, Cursor, Codex |
| [`multi-agent/`](./multi-agent/) | Configuration distribution to all supported agents | Full canonical `.ai/` with Go service model | All 7 registry agents |
| [`conflicts/`](./conflicts/) | Overwrite approvals, `--yes` safety, and preservation | Canonical `.ai/` + conflicting agent files | Codex, Claude Code, Cursor |
| [`skills/`](./skills/) | Complete skill package synchronization | Canonical skills with scripts & references | Claude Code, Cursor |
| [`mcp/`](./mcp/) | MCP configuration translation (JSON & TOML) | Canonical `.ai/mcp.toml` | Claude Code, Codex, OpenCode |
| [`reinit/`](./reinit/) | Adding agents to an already-initialized project | Canonical `.ai/` with single target agent | Claude Code → Cursor, Codex |

---

## Quick Start: Building AgentSync

From the root of the repository, compile the `agentsync` binary:

```bash
# From repository root
go build -o agentsync .
```

To run commands from within any example directory, reference the binary via relative path:

```bash
../../agentsync install
```

---

## Safe Testing Workflow: Using Disposable Copies

AgentSync writes generated files (such as `CLAUDE.md`, `AGENTS.md`, `.cursor/`, and `.codex/`) directly into the current working directory. 

To keep the committed example fixtures clean and reproducible, **always copy an example to a disposable directory** before running `init` or `install`.

### Step-by-Step Testing Workflow

```bash
# 1. Build the binary from repo root
go build -o /tmp/agentsync .

# 2. Prepare a disposable directory
mkdir -p /tmp/agentsync-manual

# 3. Copy the desired example (e.g., basic)
rm -rf /tmp/agentsync-manual/basic
cp -R examples/basic /tmp/agentsync-manual/basic

# 4. Navigate to the disposable copy
cd /tmp/agentsync-manual/basic

# 5. Run AgentSync
/tmp/agentsync install

# 6. Inspect generated agent files
ls -la
```

### Resetting a Test Scenario

To reset a scenario back to its initial state, delete the disposable test directory and re-copy the fixture:

```bash
rm -rf /tmp/agentsync-manual/basic
cp -R /path/to/agentsync/examples/basic /tmp/agentsync-manual/basic
```

> [!NOTE]
> Do not use destructive Git commands (like `git clean -fdx`) inside the repository to reset test output. Using isolated directories in `/tmp` guarantees that repository fixtures and unstaged development files remain intact.

---

## Supported Coding Agents Reference

AgentSync synchronizes configuration to seven supported agents:

| Agent | Registry ID | Guidelines Output | Skills Output | MCP Output |
|---|---|---|---|---|
| **Claude Code** | `claude-code` | `CLAUDE.md` | `.claude/skills/` | `.mcp.json` |
| **Cursor** | `cursor` | `.cursor/rules/agentsync.mdc` | `.cursor/skills/` | `.cursor/mcp.json` |
| **Codex** | `codex` | `AGENTS.md` *(shared)* | `.agents/skills/` *(shared)* | `.codex/config.toml` |
| **Gemini CLI** | `gemini-cli` | `GEMINI.md` | *(not supported)* | `.gemini/settings.json` |
| **GitHub Copilot** | `github-copilot` | `AGENTS.md` *(shared)* | `.github/skills/` | `.vscode/mcp.json` |
| **Junie** | `junie` | `.junie/guidelines.md` | `.junie/skills/` | `.junie/mcp/mcp.json` |
| **OpenCode** | `opencode` | `AGENTS.md` *(shared)* | `.agents/skills/` *(shared)* | `opencode.json` |

---

## Directory Descriptions

### 1. `basic/`
A minimal, complete project setup demonstrating the core sync loop. Contains one general guidelines document, one skill (`code-review`) with supporting checklist references, and one local MCP server (`fetch`). Configured for Claude Code and Cursor.

### 2. `existing-config/`
Demonstrates onboarding a project that already has agent files (`AGENTS.md`, `CLAUDE.md`, `.cursor/rules/project.mdc`, `.claude/skills/testing/`, `.cursor/mcp.json`, and `.mcp.json`). Shows how `agentsync init` discovers existing files, deduplicates shared guideline text, prompts for selective imports, and leaves original files intact while creating `.ai/`.

### 3. `multi-agent/`
Exercises configuration distribution across all seven supported agents simultaneously. Simulates an "Order Management Service" written in Go with Clean Architecture guidelines, three skills (`code-review`, `testing`, `documentation`), and multiple MCP servers (`fetch`, `filesystem`). Tests shared guideline writing (`AGENTS.md`), shared skill directories (`.agents/skills/`), and destination format differences (JSON vs. TOML).

### 4. `conflicts/`
Tests AgentSync's safety rules when destination files exist and differ from `.ai/`:
- **Guidelines Conflict:** Unmarked `AGENTS.md` requires adoption approval; with `--yes`, unmanaged content is preserved and skipped.
- **Skill Conflict:** Modified skill package with extra files (`personal-notes.md`) requires overwrite approval.
- **MCP Conflict:** Conflicting server (`filesystem`) prompts for resolution while unrelated servers (`unrelated-db`) and custom settings (`theme: dark`) are preserved.

### 5. `skills/`
Demonstrates that AgentSync copies complete skill packages, preserving nested files, reference checklists (`references/checklist.md`), shell scripts (`scripts/check.sh`), and code examples (`examples/sample-test.md`). Verifies that unmodified packages are skipped, canonical changes propagate, and unmanaged sibling skills are preserved.

### 6. `mcp/`
Tests translation of canonical `.ai/mcp.toml` into multiple agent destination formats, including standard JSON (`mcpServers`), VS Code JSON (`servers`), OpenCode schema JSON (`mcp`), and Codex TOML (`[mcp_servers]`). Verifies environment variable mapping, idempotency, and adding new servers.

### 7. `reinit/`
Tests repeat execution of `agentsync init` on an existing project. Verifies that canonical `.ai/` files are never overwritten or duplicated, currently configured agents are pre-selected in the interactive wizard, and new agents can be added cleanly.
