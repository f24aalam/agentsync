package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/f24aalam/agentsync/internal/config"
)

// TestSkills_SuccessfulPackageReplacement verifies:
// - Replacement installs canonical package
// - Original package is replaced
// - Temporary backup is removed
// - Ownership is recorded in lock
func TestSkills_SuccessfulPackageReplacement(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, ".ai/skills/my-skill")
	dstDir := filepath.Join(tempDir, ".claude/skills/my-skill")

	mustMkdir(t, srcDir)
	mustWrite(t, filepath.Join(srcDir, "SKILL.md"), "Canonical v2")

	mustMkdir(t, dstDir)
	mustWrite(t, filepath.Join(dstDir, "SKILL.md"), "Original v1")
	mustWrite(t, filepath.Join(dstDir, "old-note.txt"), "stale content")

	lock := config.NewLockFile()
	err := copySkillPackage(srcDir, dstDir, lock, ".claude/skills", "my-skill", tempDir)
	if err != nil {
		t.Fatalf("unexpected copySkillPackage error: %v", err)
	}

	// Verify replacement content
	data, err := os.ReadFile(filepath.Join(dstDir, "SKILL.md"))
	if err != nil || string(data) != "Canonical v2" {
		t.Fatalf("expected replacement content 'Canonical v2', got %q (err: %v)", string(data), err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "old-note.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file 'old-note.txt' was not removed during package replacement")
	}

	// Verify ownership recorded
	if !lock.IsSkillPackageManaged(".claude/skills", "my-skill") {
		t.Fatalf("expected ownership recorded in lock for 'my-skill'")
	}

	// Verify no temporary backup or staging directories remain
	entries, _ := os.ReadDir(filepath.Dir(dstDir))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".my-skill-") {
			t.Fatalf("leaked temporary directory: %s", e.Name())
		}
	}
}

// TestSkills_FailureDuringStagingPreservesOriginal verifies:
// - If staging fails (e.g. invalid symlink), original package is completely untouched
// - Ownership is not recorded
// - Staging directories are cleaned up
func TestSkills_FailureDuringStagingPreservesOriginal(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, ".ai/skills/bad-skill")
	dstDir := filepath.Join(tempDir, ".claude/skills/bad-skill")

	mustMkdir(t, srcDir)
	mustWrite(t, filepath.Join(srcDir, "SKILL.md"), "bad")
	// Create an escaping symlink to trigger validation failure
	outsideDir := t.TempDir()
	_ = os.Symlink(filepath.Join(outsideDir, "escape.txt"), filepath.Join(srcDir, "escape.txt"))

	mustMkdir(t, dstDir)
	mustWrite(t, filepath.Join(dstDir, "SKILL.md"), "Original untouched")

	lock := config.NewLockFile()
	err := copySkillPackage(srcDir, dstDir, lock, ".claude/skills", "bad-skill", tempDir)
	if err == nil {
		t.Fatalf("expected error during staging validation, got nil")
	}

	// Verify original untouched
	data, err := os.ReadFile(filepath.Join(dstDir, "SKILL.md"))
	if err != nil || string(data) != "Original untouched" {
		t.Fatalf("original package was modified! got %q (err: %v)", string(data), err)
	}

	// Verify ownership not recorded
	if lock.IsSkillPackageManaged(".claude/skills", "bad-skill") {
		t.Fatalf("ownership was recorded despite staging failure")
	}

	// Verify no temporary directories leaked
	entries, _ := os.ReadDir(filepath.Dir(dstDir))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".bad-skill-") {
			t.Fatalf("leaked temporary directory: %s", e.Name())
		}
	}
}

// TestSkills_FailedReplacementAttemptsRollbackAndRestoresOriginal verifies:
// - When renaming staged content into destination fails, rollback is attempted
// - Original package is restored to destination
// - Returned error describes original failure and restoration
// - Ownership is not recorded
// - Temporary staging resources are removed
func TestSkills_FailedReplacementAttemptsRollbackAndRestoresOriginal(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, ".ai/skills/my-skill")
	dstDir := filepath.Join(tempDir, ".claude/skills/my-skill")

	mustMkdir(t, srcDir)
	mustWrite(t, filepath.Join(srcDir, "SKILL.md"), "Canonical v2")

	mustMkdir(t, dstDir)
	mustWrite(t, filepath.Join(dstDir, "SKILL.md"), "Original precious data")
	mustWrite(t, filepath.Join(dstDir, "keep.txt"), "do not lose me")

	origRename := osRename
	defer func() { osRename = origRename }()

	// Simulate rename failure when moving tmpDir -> dstDir
	simulatedErr := errors.New("simulated rename failure into dstDir")
	osRename = func(oldpath, newpath string) error {
		if newpath == dstDir && strings.Contains(oldpath, "-tmp-") {
			return simulatedErr
		}
		return origRename(oldpath, newpath)
	}

	lock := config.NewLockFile()
	err := copySkillPackage(srcDir, dstDir, lock, ".claude/skills", "my-skill", tempDir)
	if err == nil {
		t.Fatalf("expected error from failed replacement, got nil")
	}

	if !errors.Is(err, simulatedErr) && !strings.Contains(err.Error(), simulatedErr.Error()) {
		t.Fatalf("error should contain original failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "original package restored") {
		t.Fatalf("error should report that original package was restored, got: %v", err)
	}

	// Original destination package MUST be fully restored
	data, err := os.ReadFile(filepath.Join(dstDir, "SKILL.md"))
	if err != nil || string(data) != "Original precious data" {
		t.Fatalf("expected restored original content, got %q (err: %v)", string(data), err)
	}
	keepData, err := os.ReadFile(filepath.Join(dstDir, "keep.txt"))
	if err != nil || string(keepData) != "do not lose me" {
		t.Fatalf("expected restored keep.txt, got %q (err: %v)", string(keepData), err)
	}

	// Ownership must NOT be recorded
	if lock.IsSkillPackageManaged(".claude/skills", "my-skill") {
		t.Fatalf("ownership was recorded despite replacement failure")
	}

	// Temporary staging resources should be removed
	entries, _ := os.ReadDir(filepath.Dir(dstDir))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".my-skill-") {
			t.Fatalf("leaked temporary directory: %s", e.Name())
		}
	}
}

// TestSkills_FailedRollbackReturnsBothErrorsAndPreservesBackup verifies:
// - When installation AND rollback both fail, both errors are returned
// - The retained backup path is reported in the error
// - The backup directory is preserved on disk with original files intact
// - Ownership is not recorded
func TestSkills_FailedRollbackReturnsBothErrorsAndPreservesBackup(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, ".ai/skills/my-skill")
	dstDir := filepath.Join(tempDir, ".claude/skills/my-skill")

	mustMkdir(t, srcDir)
	mustWrite(t, filepath.Join(srcDir, "SKILL.md"), "Canonical v2")

	mustMkdir(t, dstDir)
	mustWrite(t, filepath.Join(dstDir, "SKILL.md"), "Original precious data")
	mustWrite(t, filepath.Join(dstDir, "keep.txt"), "do not lose me")

	origRename := osRename
	defer func() { osRename = origRename }()

	installErr := errors.New("simulated install replacement failure")
	rollbackErr := errors.New("simulated rollback restoration failure")

	osRename = func(oldpath, newpath string) error {
		// Moving dstDir to bakDir succeeds
		if oldpath == dstDir && strings.Contains(newpath, "-bak-") {
			return origRename(oldpath, newpath)
		}
		// Moving tmpDir to dstDir fails
		if newpath == dstDir && strings.Contains(oldpath, "-tmp-") {
			return installErr
		}
		// Rollback moving bakDir to dstDir fails
		if newpath == dstDir && strings.Contains(oldpath, "-bak-") {
			return rollbackErr
		}
		return origRename(oldpath, newpath)
	}

	lock := config.NewLockFile()
	err := copySkillPackage(srcDir, dstDir, lock, ".claude/skills", "my-skill", tempDir)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	// Verify both errors are returned
	if !errors.Is(err, installErr) && !strings.Contains(err.Error(), installErr.Error()) {
		t.Errorf("error does not contain installErr: %v", err)
	}
	if !errors.Is(err, rollbackErr) && !strings.Contains(err.Error(), rollbackErr.Error()) {
		t.Errorf("error does not contain rollbackErr: %v", err)
	}

	// Verify error reports the retained backup path
	if !strings.Contains(err.Error(), "-bak-") {
		t.Errorf("error does not report backup directory path: %v", err)
	}

	// Verify ownership not recorded
	if lock.IsSkillPackageManaged(".claude/skills", "my-skill") {
		t.Fatalf("ownership was recorded despite double failure")
	}

	// Verify the backup directory still exists on disk with original files
	entries, _ := os.ReadDir(filepath.Dir(dstDir))
	var foundBak string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".my-skill-bak-") {
			foundBak = filepath.Join(filepath.Dir(dstDir), e.Name())
			break
		}
	}
	if foundBak == "" {
		t.Fatalf("backup directory was deleted or lost despite rollback failure!")
	}

	// Backup must contain original files
	bakData, err := os.ReadFile(filepath.Join(foundBak, "SKILL.md"))
	if err != nil || string(bakData) != "Original precious data" {
		t.Fatalf("backup file content corrupted or missing: %v", err)
	}
	bakKeep, err := os.ReadFile(filepath.Join(foundBak, "keep.txt"))
	if err != nil || string(bakKeep) != "do not lose me" {
		t.Fatalf("backup file keep.txt corrupted or missing: %v", err)
	}
}

// TestSkills_BackupCleanupFailureAfterSuccessfulReplacement verifies:
// - Replacement succeeds and ownership is recorded
// - Failure to remove the backup directory is clearly distinguished and reported
// - The backup directory remains for manual recovery
func TestSkills_BackupCleanupFailureAfterSuccessfulReplacement(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, ".ai/skills/my-skill")
	dstDir := filepath.Join(tempDir, ".claude/skills/my-skill")

	mustMkdir(t, srcDir)
	mustWrite(t, filepath.Join(srcDir, "SKILL.md"), "Canonical v2")

	mustMkdir(t, dstDir)
	mustWrite(t, filepath.Join(dstDir, "SKILL.md"), "Original v1")

	origRemoveAll := osRemoveAll
	defer func() { osRemoveAll = origRemoveAll }()

	cleanupErr := errors.New("simulated cleanup failure for bakDir")
	osRemoveAll = func(path string) error {
		if strings.Contains(path, "-bak-") {
			return cleanupErr
		}
		return origRemoveAll(path)
	}

	lock := config.NewLockFile()
	err := copySkillPackage(srcDir, dstDir, lock, ".claude/skills", "my-skill", tempDir)
	if err == nil {
		t.Fatalf("expected error reporting cleanup failure, got nil")
	}

	// Error distinguishes successful replacement from cleanup failure
	if !strings.Contains(err.Error(), "installed successfully, but failed to remove temporary backup at") {
		t.Errorf("error did not clearly distinguish successful install with cleanup failure: %v", err)
	}
	if !strings.Contains(err.Error(), "-bak-") {
		t.Errorf("error did not include backup path: %v", err)
	}

	// Destination package WAS replaced successfully
	dstData, err := os.ReadFile(filepath.Join(dstDir, "SKILL.md"))
	if err != nil || string(dstData) != "Canonical v2" {
		t.Fatalf("destination was not updated: %v", err)
	}

	// Ownership WAS recorded
	if !lock.IsSkillPackageManaged(".claude/skills", "my-skill") {
		t.Fatalf("expected ownership recorded in lock for successfully replaced package")
	}
}

// TestSkills_UnrelatedSiblingPackageUntouched verifies:
// - Sibling packages in the same skills directory are untouched during replacement
// - Sibling packages remain untouched even during failed replacement / rollback
func TestSkills_UnrelatedSiblingPackageUntouched(t *testing.T) {
	tempDir := t.TempDir()
	skillsDir := filepath.Join(tempDir, ".claude/skills")
	srcDir := filepath.Join(tempDir, ".ai/skills/skill-a")
	dstDir := filepath.Join(skillsDir, "skill-a")
	siblingDir := filepath.Join(skillsDir, "sibling-skill")

	mustMkdir(t, srcDir)
	mustWrite(t, filepath.Join(srcDir, "SKILL.md"), "Canonical skill-a")

	mustMkdir(t, dstDir)
	mustWrite(t, filepath.Join(dstDir, "SKILL.md"), "Original skill-a")

	mustMkdir(t, siblingDir)
	mustWrite(t, filepath.Join(siblingDir, "SKILL.md"), "Precious sibling")
	mustWrite(t, filepath.Join(siblingDir, "sibling-data.json"), `{"intact": true}`)

	lock := config.NewLockFile()
	err := copySkillPackage(srcDir, dstDir, lock, ".claude/skills", "skill-a", tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check sibling is completely intact
	sibData, err := os.ReadFile(filepath.Join(siblingDir, "SKILL.md"))
	if err != nil || string(sibData) != "Precious sibling" {
		t.Fatalf("sibling SKILL.md was modified or lost: %v", err)
	}
	sibJSON, err := os.ReadFile(filepath.Join(siblingDir, "sibling-data.json"))
	if err != nil || string(sibJSON) != `{"intact": true}` {
		t.Fatalf("sibling data was modified or lost: %v", err)
	}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
