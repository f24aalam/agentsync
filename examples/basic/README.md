# Basic Example

Demonstrates the simplest successful installation from a pre-configured canonical `.ai/` directory.

## Purpose

This example demonstrates how AgentSync reads a minimal `.ai/` setup—one guideline file, one skill with supporting reference material, one local MCP server configuration, and two selected agents—and distributes them to agent-specific destinations.

## Starting State

The repository contains only the canonical source files inside `.ai/`:

```text
basic/
├── README.md
└── .ai/
    ├── guidelines/
    │   └── general.md
    ├── skills/
    │   └── code-review/
    │       ├── SKILL.md
    │       └── references/
    │           └── checklist.md
    ├── mcp.toml
    └── sync.lock
```

Target agents configured in `.ai/sync.lock`:
- `claude-code` (Claude Code)
- `cursor` (Cursor)

No agent output files exist yet.

## Manual Verification Workflow

### 1. Build AgentSync

From the repository root:

```bash
go build -o /tmp/agentsync .
```

### 2. Copy the Example to a Disposable Location

Always test against a copy so the committed fixture remains clean:

```bash
mkdir -p /tmp/agentsync-manual
rm -rf /tmp/agentsync-manual/basic
cp -R examples/basic /tmp/agentsync-manual/basic
cd /tmp/agentsync-manual/basic
```

### 3. Run Installation

Execute `install` in non-interactive mode or interactive mode:

```bash
/tmp/agentsync install
```

### 4. Verify Generated Files

Confirm that the expected agent configuration files were created:

- **Claude Code:**
  - `CLAUDE.md`: Contains the managed section delimiters `<!-- BEGIN AGENTSYNC MANAGED -->` and `<!-- END AGENTSYNC MANAGED -->` wrapping the contents of `.ai/guidelines/general.md`.
  - `.claude/skills/code-review/SKILL.md`: Installed from `.ai/skills/code-review/SKILL.md`.
  - `.claude/skills/code-review/references/checklist.md`: Supporting reference file preserved in the nested package.
  - `.mcp.json`: Contains the `fetch` server under the `mcpServers` JSON key.
- **Cursor:**
  - `.cursor/rules/agentsync.mdc`: Contains Cursor frontmatter (`description`, `globs: *`, `alwaysApply: true`) followed by the managed guidelines block.
  - `.cursor/skills/code-review/SKILL.md`: Installed from `.ai/skills/code-review/SKILL.md`.
  - `.cursor/skills/code-review/references/checklist.md`: Supporting reference file preserved.
  - `.cursor/mcp.json`: Contains the `fetch` server under the `mcpServers` JSON key.
- **Lockfile update:**
  - `.ai/sync.lock` now records the managed skills and MCP servers for future conflict and idempotency tracking.

### 5. Verify Idempotency

Run `install` a second time:

```bash
/tmp/agentsync install
```

**Expected behavior:**
- AgentSync recognizes that existing outputs match canonical sources.
- No files are rewritten and no interactive prompts appear.

### 6. Verify Content Updates

Modify `.ai/guidelines/general.md` in the disposable copy:

```bash
echo -e "\n## Security\n- Validate all inputs.\n" >> .ai/guidelines/general.md
/tmp/agentsync install
```

**Expected behavior:**
- Because the guideline target already has managed markers (`IsManaged=true`), AgentSync will prompt to update the managed section (or automatically update it if running with `/tmp/agentsync install --yes`).
- Surrounding file content (if any had been added outside the markers) is preserved, and the updated guideline content appears between the managed markers.

### 7. Reset the Example

To reset your testing environment, remove the disposable directory and recopy:

```bash
rm -rf /tmp/agentsync-manual/basic
cp -R examples/basic /tmp/agentsync-manual/basic
```

## Limitations and Notes

- The MCP configuration uses `uvx mcp-server-fetch`, a harmless placeholder. AgentSync generates and manages configuration files; it does not execute external MCP servers during installation.
