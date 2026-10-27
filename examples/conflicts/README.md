# Conflicts Example

Demonstrates and verifies AgentSync's non-destructive conflict handling, overwrite approvals, and unmanaged content preservation.

## Purpose

When installing configuration into a project, existing files may already contain developer-authored instructions, local skill tweaks, or custom tool configurations. AgentSync enforces strict safety guarantees:

- **Never silently overwrite user content:** Any pre-existing file or entry that differs from canonical `.ai/` requires an explicit user decision.
- **Preserve unrelated settings:** Unrelated MCP servers, custom JSON properties, and developer guidelines outside managed markers are preserved intact.
- **Unmanaged safety in `--yes`:** Non-interactive `install --yes` automatically updates managed resources, but **refuses to overwrite unknown, unmanaged conflicts**, skipping them safely.

This directory serves as the primary manual regression fixture for conflict handling across guidelines, skills, and MCP configurations.

## Starting State

```text
conflicts/
├── README.md
├── .ai/
│   ├── guidelines/
│   │   └── general.md
│   ├── skills/
│   │   └── code-review/
│   │       └── SKILL.md
│   ├── mcp.toml
│   └── sync.lock
├── AGENTS.md                                    # Conflict A: Pre-existing guidelines
├── .claude/
│   └── skills/
│       └── code-review/
│           ├── SKILL.md                         # Conflict B: Pre-existing conflicting skill
│           └── personal-notes.md                # Conflict B: Extra unmanaged file in skill
└── .cursor/
    └── mcp.json                                 # Conflict C: Conflicting MCP server + custom settings
```

Target agents in `.ai/sync.lock`:
- `codex` (targets `AGENTS.md`)
- `claude-code` (targets `.claude/skills/`)
- `cursor` (targets `.cursor/mcp.json`)

---

## Conflict Scenarios & Expected Behaviors

### Conflict A: Guidelines Conflict (`AGENTS.md`)

- **State:** `AGENTS.md` exists with developer-authored notes (`# Team Local Notes`) and does not yet have AgentSync managed markers (`<!-- BEGIN AGENTSYNC MANAGED -->`).
- **Canonical Source:** `.ai/guidelines/general.md`.
- **Interactive Prompt:**
  ```text
  Codex: guidelines file already exists. Overwrite with merged .ai/guidelines? [y/N]
  ```
- **If Rejected (`No`):**
  - `AGENTS.md` is left completely unchanged.
- **If Approved (`Yes`):**
  - AgentSync adopts the file: it preserves the developer's original `# Team Local Notes` section byte-for-byte and appends the managed section (`<!-- BEGIN AGENTSYNC MANAGED -->` ... `<!-- END AGENTSYNC MANAGED -->`) containing canonical guidelines.
- **Non-Interactive (`install --yes`):**
  - Because `AGENTS.md` does not have managed markers (`IsManaged=false`), AgentSync safely **skips** it to prevent unprompted overwrites of unmanaged user notes.

---

### Conflict B: Skill Conflict (`.claude/skills/code-review/`)

- **State:** The destination directory `.claude/skills/code-review/` already exists with a conflicting `SKILL.md` and an extra file `personal-notes.md` not present in canonical `.ai/skills/code-review/`.
- **Interactive Prompt:**
  ```text
  Skill package "code-review" in .claude/skills/ already exists and differs from .ai/skills/code-review. Overwrite? [y/N]
  ```
- **If Rejected (`No`):**
  - The entire existing `.claude/skills/code-review/` package is preserved untouched, including `personal-notes.md`.
- **If Approved (`Yes`):**
  - The package is replaced by the canonical package. Canonical `SKILL.md` is installed, and obsolete/extra files like `personal-notes.md` are removed.
  - Ownership is recorded in `.ai/sync.lock` for subsequent managed updates.
  - Any unrelated sibling skills in `.claude/skills/` remain untouched.
- **Non-Interactive (`install --yes`):**
  - Because the package is not yet tracked in `.ai/sync.lock` (`IsManaged=false`), AgentSync safely **skips** it.

---

### Conflict C: MCP Conflict (`.cursor/mcp.json`)

- **State:** Destination `.cursor/mcp.json` contains:
  1. A conflicting `filesystem` server (`command: "node"`, `args: ["./local-filesystem-wrapper.js"]` vs canonical `npx @modelcontextprotocol/server-filesystem .`).
  2. An unrelated server `unrelated-db`.
  3. An unrelated top-level setting `"theme": "dark"`.
  4. Missing server `fetch` (present in `.ai/mcp.toml`).
- **Interactive Prompt:**
  ```text
  Cursor MCP configuration differs:
  .cursor/mcp.json

  Conflicting servers:
  - filesystem

  Replace these entries using .ai/mcp.toml? [y/N]
  ```
- **If Rejected (`No`):**
  - Conflicting entry `filesystem` is preserved as `node ./local-filesystem-wrapper.js`.
  - Missing server `fetch` is safely added.
  - Unrelated server `unrelated-db` and setting `"theme": "dark"` are preserved intact.
- **If Approved (`Yes`):**
  - Conflicting entry `filesystem` is replaced by the canonical definition from `.ai/mcp.toml`.
  - Missing server `fetch` is added.
  - Unrelated server `unrelated-db` and setting `"theme": "dark"` are preserved intact.
- **Non-Interactive (`install --yes`):**
  - Because `filesystem` is not yet tracked in `.ai/sync.lock` (`IsManaged=false`), AgentSync preserves the conflicting `filesystem` entry and safely adds the missing `fetch` server.

---

## Manual Verification Workflow

### 1. Build AgentSync

From the repository root:

```bash
go build -o /tmp/agentsync .
```

### 2. Copy the Example to a Disposable Location

```bash
mkdir -p /tmp/agentsync-manual
rm -rf /tmp/agentsync-manual/conflicts
cp -R examples/conflicts /tmp/agentsync-manual/conflicts
cd /tmp/agentsync-manual/conflicts
```

### 3. Test Non-Interactive Safe Mode (`--yes`)

Run `install --yes` on the fresh fixture:

```bash
/tmp/agentsync install --yes
```

**Verify Safety:**
1. Check `AGENTS.md`: Notice it still contains only the original `# Team Local Notes` (unmanaged file skipped).
2. Check `.claude/skills/code-review/personal-notes.md`: Still exists (unmanaged skill package skipped).
3. Check `.cursor/mcp.json`:
   - `filesystem` still points to `node ./local-filesystem-wrapper.js`.
   - `unrelated-db` and `"theme": "dark"` are preserved.
   - Missing `fetch` server was added.

### 4. Test Interactive Rejection

Reset the directory:

```bash
cd /tmp/agentsync-manual
rm -rf conflicts && cp -R /path/to/repo/examples/conflicts ./
cd conflicts
```

Run interactive install and decline all prompts:

```bash
/tmp/agentsync install
```

When prompted, answer **No** to all prompts.
Confirm all original files remain completely identical to the initial fixture state.

### 5. Test Interactive Approval

Reset the directory:

```bash
cd /tmp/agentsync-manual
rm -rf conflicts && cp -R /path/to/repo/examples/conflicts ./
cd conflicts
```

Run interactive install and accept all prompts:

```bash
/tmp/agentsync install
```

When prompted, answer **Yes** to all prompts.
**Confirm:**
1. `AGENTS.md` contains both `# Team Local Notes` and the newly injected `<!-- BEGIN AGENTSYNC MANAGED -->` section.
2. `.claude/skills/code-review/SKILL.md` contains the canonical content, and `personal-notes.md` has been removed.
3. `.cursor/mcp.json` contains canonical `filesystem`, canonical `fetch`, and preserved `unrelated-db` and `"theme": "dark"`.

### 6. Reset the Example

```bash
rm -rf /tmp/agentsync-manual/conflicts
cp -R examples/conflicts /tmp/agentsync-manual/conflicts
```

## Limitations and Notes

- Do not use symlinks to test conflicts in cross-platform examples, as symlink support varies between operating systems.
