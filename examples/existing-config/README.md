# Existing Config Example

Demonstrates onboarding an existing codebase with configuration files already scattered across multiple coding agents.

## Purpose

When adopting AgentSync on a project that already uses AI coding assistants, configuration is often fragmented:
- Guidelines exist in `AGENTS.md`, `CLAUDE.md`, and Cursor `.mdc` rules.
- Skills reside in agent-specific paths such as `.claude/skills/`.
- MCP server definitions exist in both `.cursor/mcp.json` and `.mcp.json`.

This example starts **without a `.ai/` directory**. It demonstrates how `agentsync init` scans the workspace, detects existing files, deduplicates identical guidelines, prompts for import choices, and preserves the original files intact while establishing a unified source of truth.

## Starting State

The project starts with only existing agent configurations and **no `.ai/` directory**:

```text
existing-config/
├── README.md
├── AGENTS.md
├── CLAUDE.md
├── .claude/
│   └── skills/
│       └── testing/
│           └── SKILL.md
├── .cursor/
│   ├── rules/
│   │   └── project.mdc
│   └── mcp.json
└── .mcp.json
```

Key aspects of this starting state:
1. **Identical guidelines:** `AGENTS.md` and `CLAUDE.md` contain identical engineering principles. AgentSync's scanner recognizes identical content and groups them together for import.
2. **Differing guidelines:** `.cursor/rules/project.mdc` contains distinct Cursor-specific rules with valid frontmatter. AgentSync presents it as an independent guideline group.
3. **Skill package:** `.claude/skills/testing/SKILL.md` contains a skill definition to be imported into the canonical `.ai/skills/` directory.
4. **Scattered MCP configs:** `.cursor/mcp.json` defines a local `sqlite` server, while `.mcp.json` defines a local `fetch` server. Both are discovered and can be consolidated into canonical `.ai/mcp.toml`.

## Manual Verification Workflow

### 1. Build AgentSync

From the repository root:

```bash
go build -o /tmp/agentsync .
```

### 2. Copy the Example to a Disposable Location

```bash
mkdir -p /tmp/agentsync-manual
rm -rf /tmp/agentsync-manual/existing-config
cp -R examples/existing-config /tmp/agentsync-manual/existing-config
cd /tmp/agentsync-manual/existing-config
```

### 3. Run Initialization Wizard

Launch `agentsync init` interactively:

```bash
/tmp/agentsync init
```

### 4. Interactive Wizard Flow

The interactive wizard guides you through the following steps:

1. **Project Name:** Defaults to directory name (e.g., `existing-config`). Press Enter.
2. **Config Scan:** AgentSync scans for existing configurations and reports:
   - Detected guidelines:
     - Group 0: Shared by `AGENTS.md` and `CLAUDE.md` (Codex + Copilot + OpenCode + Claude Code).
     - Group 1: `project.mdc` from Cursor.
   - Detected skills: `testing` from Claude Code.
   - Detected MCP servers: `sqlite` (Cursor) and `fetch` (Claude Code).
3. **Guidelines Import Selection:** Choose which detected guideline groups to import into `.ai/guidelines/general.md`. Select one or both.
4. **Skills Import Selection:** Select `testing` to import it into `.ai/skills/testing/`.
5. **MCP Import Selection:** Select `sqlite` and `fetch` to consolidate into `.ai/mcp.toml`.
6. **Agent Selection:** Select your desired target agents (e.g., `claude-code`, `cursor`, `codex`).
7. **Gitignore Question:** Choose whether to add generated files to `.gitignore` (Default: No).

### 5. Inspect the Created `.ai/` Directory

Verify that `.ai/` was created with the imported resources:

```bash
ls -la .ai/
```

- `.ai/guidelines/general.md`: Contains the selected guidelines under imported section headers (`## Imported: ...`).
- `.ai/skills/testing/SKILL.md`: Imported from `.claude/skills/testing/`.
- `.ai/mcp.toml`: Contains both `[servers.sqlite]` and `[servers.fetch]`.
- `.ai/sync.lock`: Contains your selected agents.

### 6. Verify Original Agent Files Remain Intact

Check that none of the original files were deleted or overwritten during initialization:

```bash
cat AGENTS.md
cat CLAUDE.md
cat .cursor/rules/project.mdc
cat .cursor/mcp.json
cat .mcp.json
```

All original files retain their exact starting content.

### 7. Run Installation

Distribute the newly canonicalized configuration to all selected agents:

```bash
/tmp/agentsync install
```

Review the updated agent destinations:
- Guidelines files receive the managed sections (`<!-- BEGIN AGENTSYNC MANAGED -->` ... `<!-- END AGENTSYNC MANAGED -->`).
- Destination MCP files contain the canonical servers.
- Destination skills directories receive the imported skills.

### 8. Reset the Example

To reset your testing environment:

```bash
rm -rf /tmp/agentsync-manual/existing-config
cp -R examples/existing-config /tmp/agentsync-manual/existing-config
```

## Limitations and Notes

- `agentsync init` is designed as an interactive onboarding wizard. If running in automated tests without a TUI, wizard mock hooks are used to simulate responses.
- The pre-existing agent files remain on disk after initialization. This allows developers to review the imported canonical source before deciding whether to clean up legacy files.
