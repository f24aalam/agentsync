# Re-initialization Example

Demonstrates re-running `agentsync init` on an already-initialized project to add or update target agents without overwriting or re-importing existing canonical files in `.ai/`.

## Purpose

Projects evolve over time. A team might start by targeting only Claude Code, but later decide to support Cursor and Codex as new team members join. 

When `agentsync init` is executed in a repository where `.ai/` or `.ai/sync.lock` already exists, AgentSync:
1. **Detects existing initialization:** Safely bypasses the initial scaffolding and import flows.
2. **Preserves canonical assets:** Leaves existing guidelines, skills, and MCP configurations in `.ai/` completely untouched.
3. **Presents current selections:** Launches a streamlined re-initialization survey with the currently configured agents pre-selected.
4. **Updates target agents:** Updates `.ai/sync.lock` with the new agent selections while preserving any previously recorded skill or MCP ownership metadata.

## Starting State

```text
reinit/
├── README.md
└── .ai/
    ├── guidelines/
    │   └── general.md
    └── sync.lock
```

- Initial target agent in `.ai/sync.lock`:
  ```toml
  agents = ["claude-code"]
  ```
- No generated agent files (`CLAUDE.md`, `AGENTS.md`, `.cursor/`, etc.) exist yet.

## Manual Verification Workflow

### 1. Build AgentSync

From the repository root:

```bash
go build -o /tmp/agentsync .
```

### 2. Copy the Example to a Disposable Location

```bash
mkdir -p /tmp/agentsync-manual
rm -rf /tmp/agentsync-manual/reinit
cp -R examples/reinit /tmp/agentsync-manual/reinit
cd /tmp/agentsync-manual/reinit
```

### 3. Run Re-initialization

Run `agentsync init`:

```bash
/tmp/agentsync init
```

### 4. Interactive Flow and Prompts

Because `.ai/` already exists, AgentSync skips project scaffolding and enters reinit mode:

1. **Agent Selection:** A multi-select list is displayed with `claude-code` pre-selected.
   - Use the arrow keys and spacebar to select additional agents, for example: `cursor` and `codex`.
   - Press Enter to confirm.
2. **Gitignore Prompt:** Choose whether to update `.gitignore` (Default: No).
3. **Summary Output:**
   ```text
   AgentSync already initialized. Updating configuration:
     ✓ .ai/sync.lock

   Done! Run agentsync install to configure your agents.
   ```

### 5. Verify Canonical Preservation

Inspect `.ai/guidelines/general.md`:

```bash
cat .ai/guidelines/general.md
```

**Expected behavior:** The contents of `.ai/guidelines/general.md` remain completely unchanged. No duplicate headers, reimport markers, or modifications exist.

### 6. Verify Updated Lockfile

Inspect `.ai/sync.lock`:

```bash
cat .ai/sync.lock
```

**Expected behavior:** The `agents` array now contains the updated set of agents:
```toml
agents = ["claude-code", "cursor", "codex"]
```

### 7. Run Installation

Distribute configuration to the newly selected agents:

```bash
/tmp/agentsync install
```

**Expected behavior:**
- Configuration is installed for all three agents:
  - `CLAUDE.md` (Claude Code)
  - `.cursor/rules/agentsync.mdc` (Cursor)
  - `AGENTS.md` (Codex)

### 8. Reset the Example

```bash
rm -rf /tmp/agentsync-manual/reinit
cp -R examples/reinit /tmp/agentsync-manual/reinit
```

## Limitations and Notes

- `agentsync init` in reinit mode only modifies `.ai/sync.lock` and optionally `.gitignore`. It does not delete or overwrite files in `.ai/guidelines/`, `.ai/skills/`, or `.ai/mcp.toml`.
