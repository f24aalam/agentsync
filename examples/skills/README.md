# Skills Example

Demonstrates that AgentSync synchronizes entire skill packages—including nested scripts, subdirectories, and reference documentation—rather than just isolated `SKILL.md` files.

## Purpose

Skills in modern coding agents often require supporting assets such as validation scripts, checklists, templates, and example files. This example demonstrates how AgentSync treats each subdirectory under `.ai/skills/` as a complete, self-contained package and synchronizes its full directory structure into agent-specific skill destinations.

## Starting State

```text
skills/
├── README.md
└── .ai/
    ├── guidelines/
    │   └── general.md
    ├── skills/
    │   ├── code-review/
    │   │   ├── SKILL.md
    │   │   ├── scripts/
    │   │   │   └── check.sh
    │   │   └── references/
    │   │       └── checklist.md
    │   └── testing/
    │       ├── SKILL.md
    │       └── examples/
    │           └── sample-test.md
    └── sync.lock
```

Target agents in `.ai/sync.lock`:
- `claude-code` (dest: `.claude/skills/`)
- `cursor` (dest: `.cursor/skills/`)

## Manual Verification Workflow

### 1. Build AgentSync

From the repository root:

```bash
go build -o /tmp/agentsync .
```

### 2. Copy the Example to a Disposable Location

```bash
mkdir -p /tmp/agentsync-manual
rm -rf /tmp/agentsync-manual/skills
cp -R examples/skills /tmp/agentsync-manual/skills
cd /tmp/agentsync-manual/skills
```

### 3. Run Installation

```bash
/tmp/agentsync install
```

### 4. Verify Complete Skill Package Installation

Check that all nested files and subdirectories were mirrored to both agent destinations:

```bash
# Verify Claude Code skills
find .claude/skills/ -type f | sort

# Expected output:
# .claude/skills/code-review/SKILL.md
# .claude/skills/code-review/references/checklist.md
# .claude/skills/code-review/scripts/check.sh
# .claude/skills/testing/SKILL.md
# .claude/skills/testing/examples/sample-test.md

# Verify Cursor skills
find .cursor/skills/ -type f | sort

# Expected output:
# .cursor/skills/code-review/SKILL.md
# .cursor/skills/code-review/references/checklist.md
# .cursor/skills/code-review/scripts/check.sh
# .cursor/skills/testing/SKILL.md
# .cursor/skills/testing/examples/sample-test.md
```

Confirm that the nested directory hierarchies (`scripts/`, `references/`, `examples/`) are fully preserved.

### 5. Verify Idempotency

Run `install` a second time:

```bash
/tmp/agentsync install
```

**Expected behavior:** Both skill packages match canonical sources byte-for-byte; no files are modified, and no prompts appear.

### 6. Verify Package Propagation

Modify a supporting file inside the canonical `code-review` skill:

```bash
echo -e "\n- [ ] Dependencies have zero known security vulnerabilities.\n" >> .ai/skills/code-review/references/checklist.md
/tmp/agentsync install --yes
```

**Expected behavior:**
- AgentSync detects the change in `code-review`.
- Because `code-review` was previously installed and recorded in `.ai/sync.lock`, the managed package is cleanly updated.
- The `testing` package remains untouched.

### 7. Verify Unrelated Sibling Skills Are Preserved

Simulate a user-created, unmanaged local skill in `.claude/skills/`:

```bash
mkdir -p .claude/skills/local-notes
echo "# Local Notes" > .claude/skills/local-notes/SKILL.md

/tmp/agentsync install
```

**Expected behavior:**
- AgentSync updates or skips canonical skills (`code-review`, `testing`).
- The unmanaged sibling skill `.claude/skills/local-notes/` is left completely intact.

### 8. Verify Non-Destructive Behavior on Canonical Deletion

Remove a canonical skill from `.ai/skills/`:

```bash
rm -rf .ai/skills/testing
/tmp/agentsync install
```

**Expected behavior:**
- AgentSync does not delete existing destination directories automatically when removed from `.ai/skills/`.
- `.claude/skills/testing/` remains in place to prevent accidental data loss.

### 9. Reset the Example

```bash
rm -rf /tmp/agentsync-manual/skills
cp -R examples/skills /tmp/agentsync-manual/skills
```

## Limitations and Notes

- `check.sh` is provided as an example of a supporting utility script; AgentSync copies it as a file asset and does not execute it during installation.
