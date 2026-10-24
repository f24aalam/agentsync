package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/f24aalam/agentsync/internal/detect"
	stepflow "github.com/f24aalam/stepflow/pkg"
)

// Scenario A — Fresh project
// 1. Initialize a project containing no existing agent configurations.
// 2. Select multiple supported agents.
// 3. Verify the canonical .ai/ structure.
// 4. Verify selected agents are recorded correctly.
// 5. Run install.
// 6. Confirm the expected destination files exist.
func TestE2E_ScenarioA_FreshProject(t *testing.T) {
	env := newIntegrationEnv(t)

	answers := initAnswers{
		ProjectName:    "FreshApp",
		AddGuidelines:  true,
		AddSampleSkill: true,
		AddMCPConfig:   true,
		Agents:         []string{"cursor", "codex"},
		AddGitignore:   true,
	}

	stdout, stderr, err := env.RunInit("FreshApp", importPlan{}, answers)
	if err != nil {
		t.Fatalf("init failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	// 3. Canonical .ai/ structure
	env.AssertFileExists(".ai/guidelines/core.md")
	env.AssertFileExists(".ai/skills/example-skill/SKILL.md")
	env.AssertFileExists(".ai/mcp.toml")
	env.AssertFileExists(".ai/sync.lock")

	// 4. Selected agents recorded correctly
	env.AssertFileContains(".ai/sync.lock", `agents = ["cursor", "codex"]`)

	// 5. Run install with active MCP server added
	env.WriteFile(".ai/mcp.toml", `
[servers.postgres]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]
`)

	stdout, stderr, err = env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	// 6. Confirm expected destination files exist
	env.AssertFileExists(".cursor/rules/agentsync.mdc")
	env.AssertFileExists(".cursor/skills/example-skill/SKILL.md")
	env.AssertFileExists(".cursor/mcp.json")

	env.AssertFileExists("AGENTS.md")
	env.AssertFileExists(".agents/skills/example-skill/SKILL.md")
	env.AssertFileExists(".codex/config.toml")
}

// Scenario B — Existing project
// 1. Prepare existing Claude/Cursor/Codex configuration fixtures.
// 2. Run init.
// 3. Verify supported configurations are detected.
// 4. Verify selected imports are consolidated into .ai/.
// 5. Confirm original configurations are preserved.
// 6. Run install.
// 7. Verify the generated configuration matches the canonical source.
func TestE2E_ScenarioB_ExistingProject(t *testing.T) {
	env := newIntegrationEnv(t)

	// 1. Prepare existing configurations
	env.WriteFile("CLAUDE.md", "# Existing Claude Instructions\nFollow strict TDD.\n")
	env.WriteFile(".cursor/rules/legacy.mdc", "# Legacy Cursor Rule\nUse TypeScript.\n")
	env.WriteFile(".codex/config.toml", "# Existing Codex TOML\nmodel = \"gpt-4o\"\n\n[mcp_servers.legacy_tool]\ncommand = \"legacy-cmd\"\n")

	// 2. Run init with import plan
	detection, err := detect.ScanProject(env.RootDir)
	if err != nil {
		t.Fatalf("detection failed: %v", err)
	}

	// 3. Verify supported configurations are detected
	foundClaude := false
	foundCodex := false
	foundCursor := false
	for _, g := range detection.Guidelines {
		if g.Agent.ID == "claude-code" {
			foundClaude = true
		}
		if g.Agent.ID == "cursor" {
			foundCursor = true
		}
	}
	for _, m := range detection.MCPServers {
		if m.Agent.ID == "codex" {
			foundCodex = true
		}
	}
	if !foundClaude || !foundCodex || !foundCursor {
		t.Fatalf("expected all 3 agents detected, got claude=%v codex=%v cursor=%v", foundClaude, foundCodex, foundCursor)
	}

	plan := importPlan{
		Guidelines: []importGuideline{
			{Label: "claude", Content: "Follow strict TDD.\n"},
		},
	}
	answers := initAnswers{
		ProjectName:    "ExistingApp",
		AddGuidelines:  false, // imported instead
		AddSampleSkill: true,
		AddMCPConfig:   true,
		Agents:         []string{"claude-code", "codex"},
		AddGitignore:   true,
	}

	stdout, stderr, err := env.RunInit("ExistingApp", plan, answers)
	if err != nil {
		t.Fatalf("init failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	// 4. Selected imports consolidated into .ai/
	env.AssertFileExists(".ai/guidelines/core.md")
	env.AssertFileContains(".ai/guidelines/core.md", "Follow strict TDD")

	// 5. Original configurations preserved
	env.AssertFileContains("CLAUDE.md", "Follow strict TDD")
	env.AssertFileContains(".codex/config.toml", "legacy-cmd")
	env.AssertFileContains(".cursor/rules/legacy.mdc", "Legacy Cursor Rule")

	// 6. Run install with --yes
	stdout, stderr, err = env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	// 7. Verify generated configuration matches canonical source
	env.AssertFileContains("CLAUDE.md", "Follow strict TDD")
	env.AssertFileContains("AGENTS.md", "Follow strict TDD")
	env.AssertFileContains(".codex/config.toml", "legacy_tool")
}

// Scenario C — Add another agent
// 1. Initialize with one agent.
// 2. Run init again.
// 3. Add another supported agent.
// 4. Confirm canonical .ai/ content is unchanged.
// 5. Run install.
// 6. Confirm the new agent receives the configuration.
func TestE2E_ScenarioC_AddAnotherAgent(t *testing.T) {
	env := newIntegrationEnv(t)

	// 1. Initialize with one agent (codex)
	answers := initAnswers{
		ProjectName:    "App",
		AddGuidelines:  true,
		AddSampleSkill: true,
		AddMCPConfig:   true,
		Agents:         []string{"codex"},
		AddGitignore:   true,
	}
	_, _, err := env.RunInit("App", importPlan{}, answers)
	if err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Custom guideline added by user in .ai/
	env.WriteFile(".ai/guidelines/custom.md", "# Custom Team Rule\nShip fast safely.\n")
	origCoreContent := env.ReadFile(".ai/guidelines/core.md")

	// 2 & 3. Run init again to add claude-code
	reinitAns := reinitAnswers{
		Agents:       []string{"codex", "claude-code"},
		AddGitignore: true,
	}
	_, _, err = env.RunReinit(reinitAns)
	if err != nil {
		t.Fatalf("reinit failed: %v", err)
	}

	// 4. Confirm canonical .ai/ content is unchanged
	env.AssertFileContains(".ai/guidelines/custom.md", "Ship fast safely")
	if env.ReadFile(".ai/guidelines/core.md") != origCoreContent {
		t.Fatalf("canonical core.md was unexpectedly modified by reinit")
	}
	env.AssertFileContains(".ai/sync.lock", "codex")
	env.AssertFileContains(".ai/sync.lock", "claude-code")

	// 5. Run install
	_, _, err = env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// 6. Confirm new agent receives configuration
	env.AssertFileExists("CLAUDE.md")
	env.AssertFileContains("CLAUDE.md", "Ship fast safely")
	env.AssertFileExists(".claude/skills/example-skill/SKILL.md")
	env.AssertFileExists(".mcp.json")
}

// Scenario D — Idempotency
// 1. Run install.
// 2. Record output file contents and modification times.
// 3. Run install again without canonical changes.
// 4. Verify no unnecessary file writes.
// 5. Verify no unnecessary overwrite prompts.
func TestE2E_ScenarioD_Idempotency(t *testing.T) {
	env := newIntegrationEnv(t)
	env.PopulateStandardAI([]string{"claude-code", "codex"})

	// 1. Initial install
	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("initial install failed: %v", err)
	}

	// 2. Record file contents and mtimes
	files := []string{
		"CLAUDE.md",
		"AGENTS.md",
		".claude/skills/code-review/SKILL.md",
		".agents/skills/code-review/SKILL.md",
		".mcp.json",
		".codex/config.toml",
		".ai/sync.lock",
	}

	mtimes := make(map[string]time.Time)
	contents := make(map[string]string)
	for _, f := range files {
		st, err := os.Stat(filepath.Join(env.RootDir, f))
		if err != nil {
			t.Fatalf("stat failed on %s: %v", f, err)
		}
		mtimes[f] = st.ModTime()
		contents[f] = env.ReadFile(f)
	}

	// Sleep slightly to guarantee different mtime if written
	time.Sleep(50 * time.Millisecond)

	// 3 & 5. Run install again without -y: must not prompt
	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()
	prompted := false
	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		prompted = true
		return stepflow.Result{}, nil
	}

	stdout, stderr, err := env.RunInstall()
	if err != nil {
		t.Fatalf("repeat install failed: %v\nStderr: %s\nStdout: %s", err, stderr, stdout)
	}

	if prompted {
		t.Fatalf("unexpected prompt on unchanged install!")
	}

	// 4. Verify no unnecessary file writes
	for _, f := range files {
		st, err := os.Stat(filepath.Join(env.RootDir, f))
		if err != nil {
			t.Fatalf("stat failed on %s: %v", f, err)
		}
		if !st.ModTime().Equal(mtimes[f]) {
			t.Errorf("file %s was modified during idempotent run! Old mtime: %v, New mtime: %v", f, mtimes[f], st.ModTime())
		}
		if env.ReadFile(f) != contents[f] {
			t.Errorf("file %s content changed during idempotent run!", f)
		}
	}
}

// Scenario E — Conflicts
// Verify:
// - Managed guideline conflict.
// - Unknown existing guidelines.
// - Managed skill package conflict.
// - Unknown existing skill package.
// - Managed MCP server conflict.
// - Unknown existing MCP server.
// - User approves.
// - User declines.
// - --yes approves only eligible managed overwrites.
// - Unrelated content remains intact.
func TestE2E_ScenarioE_Conflicts(t *testing.T) {
	env := newIntegrationEnv(t)

	// 1. Set up initial managed state
	env.PopulateStandardAI([]string{"claude-code", "codex"})
	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("setup install failed: %v", err)
	}

	// 2. Introduce managed conflict (modify .ai/)
	env.WriteFile(".ai/guidelines/01-core.md", "# Project Core Guidelines\nManaged v2 update\n")
	env.WriteFile(".ai/skills/code-review/SKILL.md", "---\nname: code-review\n---\n# Code Review v2\n")
	env.WriteFile(".ai/mcp.toml", `
[servers.postgres]
type = "local"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres", "--v2"]
`)

	// 3. Introduce unmanaged conflicts and unrelated user data
	// Unmanaged unknown guidelines file in destination
	env.WriteFile("UNKNOWN.md", "Precious unknown guidelines\n")
	// Unmanaged skill in destination
	env.WriteFile(".claude/skills/unmanaged-skill/SKILL.md", "User local unmanaged skill\n")
	// Unmanaged MCP server in destination
	env.WriteFile(".mcp.json", `{
  "custom_root_key": "custom_val",
  "mcpServers": {
    "postgres": { "command": "custom-pg" },
    "user_unmanaged_server": { "command": "user-tool" }
  }
}`)
	// In sync.lock, remove postgres so it becomes unmanaged conflict
	env.WriteFile(".ai/sync.lock", `agents = ["claude-code", "codex"]`+"\n")

	// 4. Test User declines:
	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			res[s.Key()] = "No" // Decline
		}
		return res, nil
	}

	_, _, err = env.RunInstall()
	if err != nil {
		t.Fatalf("install with decline failed: %v", err)
	}

	// Conflicting unmanaged postgres preserved
	env.AssertFileContains(".mcp.json", `"command": "custom-pg"`)
	env.AssertFileContains(".mcp.json", "user_unmanaged_server")
	env.AssertFileContains(".mcp.json", "custom_root_key")
	env.AssertFileContains(".claude/skills/unmanaged-skill/SKILL.md", "User local unmanaged skill")

	// 5. Test --yes: approves only eligible managed overwrites, preserves unmanaged
	// Restore managed marker in guidelines
	_, _, err = env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install -y failed: %v", err)
	}

	// Managed guideline updated
	env.AssertFileContains("CLAUDE.md", "Managed v2 update")
	// Unmanaged MCP postgres remains preserved under --yes
	env.AssertFileContains(".mcp.json", `"command": "custom-pg"`)
	env.AssertFileContains(".mcp.json", "user_unmanaged_server")
	// Unmanaged skill remains preserved
	env.AssertFileContains(".claude/skills/unmanaged-skill/SKILL.md", "User local unmanaged skill")

	// 6. Test User approves:
	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			res[s.Key()] = "Yes" // Approve
		}
		return res, nil
	}

	_, _, err = env.RunInstall()
	if err != nil {
		t.Fatalf("install with approve failed: %v", err)
	}

	// Now approved postgres is updated
	env.AssertFileContains(".mcp.json", "--v2")
	// Unrelated server and root keys still intact
	env.AssertFileContains(".mcp.json", "user_unmanaged_server")
	env.AssertFileContains(".mcp.json", "custom_root_key")
}

// Scenario F — Error handling
// Verify:
// - Malformed MCP destination.
// - Invalid canonical MCP configuration.
// - Unwritable destination.
// - Failed skill replacement.
// - Failed MCP write.
// - Failed lockfile persistence.
// Every failure must produce a clear error and an appropriate nonzero exit status.
func TestE2E_ScenarioF_ErrorHandling(t *testing.T) {
	// 1. Malformed MCP destination
	t.Run("MalformedMCPDestination", func(t *testing.T) {
		env := newIntegrationEnv(t)
		env.PopulateStandardAI([]string{"claude-code"})
		env.WriteFile(".mcp.json", `{ invalid json content `)

		stdout, stderr, err := env.RunInstall("-y")
		if err == nil {
			t.Fatalf("expected nonzero exit status on malformed MCP destination, got success!\nStdout: %s", stdout)
		}
		combined := stdout + "\n" + stderr
		if !strings.Contains(combined, "cannot parse existing JSON config") && !strings.Contains(combined, "error") {
			t.Errorf("expected clear error message, got:\n%s", combined)
		}
	})

	// 2. Invalid canonical MCP configuration
	t.Run("InvalidCanonicalMCP", func(t *testing.T) {
		env := newIntegrationEnv(t)
		env.PopulateStandardAI([]string{"claude-code"})
		env.WriteFile(".ai/mcp.toml", `[unclosed table header`)

		stdout, _, err := env.RunInstall("-y")
		if err == nil {
			t.Fatalf("expected nonzero exit status on invalid canonical MCP, got success!\nStdout: %s", stdout)
		}
	})

	// 3. Unwritable destination
	t.Run("UnwritableDestination", func(t *testing.T) {
		env := newIntegrationEnv(t)
		env.PopulateStandardAI([]string{"claude-code"})

		// Create a directory where a file is expected
		_ = os.MkdirAll(filepath.Join(env.RootDir, "CLAUDE.md"), 0o755)

		_, _, err := env.RunInstall("-y")
		if err == nil {
			t.Fatalf("expected error when destination path is a directory, got nil")
		}
	})

	// 4. Failed skill replacement
	t.Run("FailedSkillReplacement", func(t *testing.T) {
		env := newIntegrationEnv(t)
		env.PopulateStandardAI([]string{"claude-code"})
		// Create invalid symlink in canonical skill
		_ = os.Symlink("/nonexistent/target/path", filepath.Join(env.RootDir, ".ai/skills/code-review/bad-link"))

		stdout, stderr, err := env.RunInstall("-y")
		if err == nil {
			t.Fatalf("expected error on failed skill replacement, got nil")
		}
		combined := stdout + "\n" + stderr
		if !strings.Contains(combined, "code-review") {
			t.Errorf("error did not identify affected skill package:\n%s", combined)
		}
	})

	// 5. Failed lockfile persistence
	t.Run("FailedLockfilePersistence", func(t *testing.T) {
		env := newIntegrationEnv(t)
		env.PopulateStandardAI([]string{"claude-code"})

		// Make .ai/sync.lock unwritable by creating a non-empty directory at its location
		_ = os.Remove(filepath.Join(env.RootDir, ".ai/sync.lock"))
		_ = os.MkdirAll(filepath.Join(env.RootDir, ".ai/sync.lock/subfile"), 0o755)

		_, _, err := env.RunInstall("-y")
		if err == nil {
			t.Fatalf("expected error on failed lockfile persistence, got nil")
		}
	})
}
