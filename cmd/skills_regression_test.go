package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	stepflow "github.com/f24aalam/stepflow/pkg"
)

// Scenario 1: Identical unmanaged package is not adopted.
func TestRegression_Skills_IdenticalUnmanagedPackageNotAdopted(t *testing.T) {
	env := newIntegrationEnv(t)

	// Pre-create canonical skill
	env.WriteFile(".ai/skills/my-tool/SKILL.md", "Tool content\n")
	env.WriteFile(".ai/sync.lock", `agents = ["codex"]`+"\n")

	// Pre-create destination skill with identical content, but UNMANAGED (not in sync.lock)
	env.WriteFile(".agents/skills/my-tool/SKILL.md", "Tool content\n")

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	lockContent := env.ReadFile(".ai/sync.lock")
	if strings.Contains(lockContent, "my-tool") {
		t.Fatalf("ADOPTION VIOLATION: identical pre-existing skill package was adopted into sync.lock!\n%s", lockContent)
	}
}

// Scenario 2: New package is created and recorded in lock.
func TestRegression_Skills_NewPackageCreatedAndRecorded(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".ai/skills/brand-new/SKILL.md", "Brand new skill\n")
	env.WriteFile(".ai/sync.lock", `agents = ["claude-code"]`+"\n")

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	env.AssertFileExists(".claude/skills/brand-new/SKILL.md")
	lockContent := env.ReadFile(".ai/sync.lock")
	if !strings.Contains(lockContent, "brand-new") {
		t.Fatalf("expected new package 'brand-new' to be recorded in sync.lock:\n%s", lockContent)
	}
}

// Scenario 3: Approved existing package is replaced and recorded.
// Scenario 4: Rejected package remains unchanged and not recorded.
func TestRegression_Skills_ApprovedVsRejectedDecisions(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".ai/skills/approved-tool/SKILL.md", "Canonical approved v2\n")
	env.WriteFile(".ai/skills/rejected-tool/SKILL.md", "Canonical rejected v2\n")
	env.WriteFile(".ai/sync.lock", `agents = ["claude-code"]`+"\n")

	// Pre-create conflicting unmanaged destination packages
	env.WriteFile(".claude/skills/approved-tool/SKILL.md", "User local version 1\n")
	env.WriteFile(".claude/skills/approved-tool/stale-notes.txt", "Clean me up\n")

	env.WriteFile(".claude/skills/rejected-tool/SKILL.md", "User custom version\n")
	env.WriteFile(".claude/skills/rejected-tool/keep-notes.txt", "Keep this safe\n")

	restoreStepflow := runInstallStepflow
	defer func() { runInstallStepflow = restoreStepflow }()

	runInstallStepflow = func(steps []stepflow.Step) (stepflow.Result, error) {
		res := make(stepflow.Result)
		for _, s := range steps {
			if strings.Contains(s.Key(), "approved-tool") {
				res[s.Key()] = "Yes"
			} else if strings.Contains(s.Key(), "rejected-tool") {
				res[s.Key()] = "No"
			}
		}
		return res, nil
	}

	_, _, err := env.RunInstall()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Approved package: replaced, stale file removed, recorded in lock
	env.AssertFileContains(".claude/skills/approved-tool/SKILL.md", "Canonical approved v2")
	env.AssertFileNotExists(".claude/skills/approved-tool/stale-notes.txt")

	// Rejected package: completely untouched, not in lock
	env.AssertFileContains(".claude/skills/rejected-tool/SKILL.md", "User custom version")
	env.AssertFileExists(".claude/skills/rejected-tool/keep-notes.txt")

	lockContent := env.ReadFile(".ai/sync.lock")
	if !strings.Contains(lockContent, "approved-tool") {
		t.Errorf("expected approved-tool in sync.lock:\n%s", lockContent)
	}
	if strings.Contains(lockContent, "rejected-tool") {
		t.Errorf("rejected-tool must NOT be in sync.lock:\n%s", lockContent)
	}
}

// Scenario 5: --yes updates previously managed packages.
// Scenario 6: --yes preserves unmanaged conflicting packages.
func TestRegression_Skills_YesUpdatesManagedPreservesUnmanaged(t *testing.T) {
	env := newIntegrationEnv(t)

	// sync.lock tracks 'managed-tool', but NOT 'unmanaged-tool'
	env.WriteFile(".ai/sync.lock", `
agents = ["claude-code"]

[skills]
".claude/skills" = ["managed-tool"]
`)

	env.WriteFile(".ai/skills/managed-tool/SKILL.md", "Managed tool v2\n")
	env.WriteFile(".ai/skills/unmanaged-tool/SKILL.md", "Canonical unmanaged tool v2\n")

	env.WriteFile(".claude/skills/managed-tool/SKILL.md", "Managed tool v1\n")
	env.WriteFile(".claude/skills/unmanaged-tool/SKILL.md", "User precious unmanaged skill\n")

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install -y failed: %v", err)
	}

	// Managed tool must be updated
	env.AssertFileContains(".claude/skills/managed-tool/SKILL.md", "Managed tool v2")

	// Unmanaged conflicting tool must be preserved untouched
	env.AssertFileContains(".claude/skills/unmanaged-tool/SKILL.md", "User precious unmanaged skill")
}

// Scenario 11: Unrelated sibling skills remain untouched.
func TestRegression_Skills_UnrelatedSiblingSkillsUntouched(t *testing.T) {
	env := newIntegrationEnv(t)

	env.WriteFile(".ai/skills/synced-skill/SKILL.md", "Synced content\n")
	env.WriteFile(".ai/sync.lock", `agents = ["claude-code"]`+"\n")

	// Pre-create unrelated sibling skill in destination
	env.WriteFile(".claude/skills/custom-sibling/SKILL.md", "Custom sibling content\n")
	env.WriteFile(".claude/skills/custom-sibling/data/extra.json", `{"key":"val"}`)

	_, _, err := env.RunInstall("-y")
	if err != nil {
		t.Fatalf("install -y failed: %v", err)
	}

	env.AssertFileExists(".claude/skills/synced-skill/SKILL.md")
	env.AssertFileExists(".claude/skills/custom-sibling/SKILL.md")
	env.AssertFileExists(".claude/skills/custom-sibling/data/extra.json")
	env.AssertFileContains(".claude/skills/custom-sibling/SKILL.md", "Custom sibling content")
}

// Error-handling tests:
// - Failed writes produce a nonzero exit status.
// - Package replacement failures are reported with skill package name.
// - Ownership isn't updated after a failed replacement.
// - Temporary directories are cleaned up.
// - No operation falsely reports success.
func TestRegression_Skills_FailureHandlingAndCleanRollback(t *testing.T) {
	env := newIntegrationEnv(t)

	// Canonical skill has an invalid/broken symlink
	env.WriteFile(".ai/skills/broken-skill/SKILL.md", "Content\n")
	linkPath := filepath.Join(env.RootDir, ".ai/skills/broken-skill/broken.txt")
	if err := os.Symlink("nonexistent-file.txt", linkPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// broken-skill was previously managed, so --yes attempts to update it
	env.WriteFile(".ai/sync.lock", `
agents = ["claude-code"]

[skills]
".claude/skills" = ["broken-skill"]
`+"\n")

	// Pre-existing destination package
	env.WriteFile(".claude/skills/broken-skill/SKILL.md", "Original pre-existing content\n")

	stdout, stderr, err := env.RunInstall("-y")
	if err == nil {
		t.Fatalf("expected non-zero exit status on failure, got success!\nStdout:\n%s", stdout)
	}

	// Must report failure and identify affected package
	combined := stdout + "\n" + stderr
	if !strings.Contains(combined, "broken-skill") {
		t.Errorf("error output did not identify affected skill package 'broken-skill':\n%s", combined)
	}
	if strings.Contains(stdout, "Done!") {
		t.Errorf("falsely reported unconditional success 'Done!':\n%s", stdout)
	}

	// Original destination package MUST be preserved untouched
	env.AssertFileContains(".claude/skills/broken-skill/SKILL.md", "Original pre-existing content")
	env.AssertFileNotExists(".claude/skills/broken-skill/broken.txt")

	// No temporary staging directories left in .claude/skills
	entries, _ := os.ReadDir(filepath.Join(env.RootDir, ".claude/skills"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && (strings.Contains(e.Name(), "-tmp-") || strings.Contains(e.Name(), "-bak-")) {
			t.Errorf("temporary staging directory %s was leaked on failure!", e.Name())
		}
	}
}
