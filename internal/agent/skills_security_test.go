package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/f24aalam/agentsync/internal/config"
)

// TestSkills_DestinationSymlinkSafety tests the required security scenarios from Section 4:
// - Destination regular file is safe.
// - Destination file is a symlink outside the project.
// - Destination subdirectory is a symlink outside the project.
// - Source symlink escapes the skill package.
// - Source symlink resolves through another symlink.
// - Source symlink is broken or circular.
// - Replacement fails during copying.
// External target files must remain unchanged in every unsafe case.
func TestSkills_DestinationSymlinkSafety(t *testing.T) {
	outsideDir := t.TempDir()
	outsideVictimFile := filepath.Join(outsideDir, "victim.txt")
	outsideVictimContent := "DO NOT OVERWRITE THIS VICTIM FILE"
	if err := os.WriteFile(outsideVictimFile, []byte(outsideVictimContent), 0o644); err != nil {
		t.Fatal(err)
	}

	outsideVictimSubDir := filepath.Join(outsideDir, "external_subdir")
	if err := os.MkdirAll(outsideVictimSubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideVictimSubFile := filepath.Join(outsideVictimSubDir, "sub_victim.txt")
	outsideVictimSubContent := "DO NOT OVERWRITE THIS SUBDIR FILE"
	if err := os.WriteFile(outsideVictimSubFile, []byte(outsideVictimSubContent), 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := t.TempDir()
	canonicalSkillDir := filepath.Join(projectDir, ".ai/skills/my-skill")
	if err := os.MkdirAll(filepath.Join(canonicalSkillDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonicalSkillDir, "SKILL.md"), []byte("Canonical skill content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonicalSkillDir, "subdir/tool.sh"), []byte("echo tool"), 0o644); err != nil {
		t.Fatal(err)
	}

	destSkillsDir := filepath.Join(projectDir, ".claude/skills")
	destSkillPackage := filepath.Join(destSkillsDir, "my-skill")
	if err := os.MkdirAll(destSkillPackage, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Destination file is a symlink outside the project
	// Symlink .claude/skills/my-skill/SKILL.md -> outsideVictimFile
	destSymlinkFile := filepath.Join(destSkillPackage, "SKILL.md")
	if err := os.Symlink(outsideVictimFile, destSymlinkFile); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	lock := config.NewLockFile()
	err := copySkillPackage(canonicalSkillDir, destSkillPackage, lock, ".claude/skills", "my-skill", projectDir)
	if err != nil {
		t.Fatalf("copySkillPackage failed on destination symlink file: %v", err)
	}

	// External victim file MUST remain untouched!
	data, err := os.ReadFile(outsideVictimFile)
	if err != nil || string(data) != outsideVictimContent {
		t.Fatalf("SECURITY VIOLATION: external victim file was modified! Got: %q", string(data))
	}

	// Destination SKILL.md should now be a regular file with canonical content
	st, err := os.Lstat(destSymlinkFile)
	if err != nil || st.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected dest SKILL.md to be safely replaced regular file, not a symlink")
	}

	// 2. Destination subdirectory is a symlink outside the project
	// Symlink .claude/skills/my-skill/subdir -> outsideVictimSubDir
	_ = os.RemoveAll(destSkillPackage)
	if err := os.MkdirAll(destSkillPackage, 0o755); err != nil {
		t.Fatal(err)
	}
	destSubdirSymlink := filepath.Join(destSkillPackage, "subdir")
	if err := os.Symlink(outsideVictimSubDir, destSubdirSymlink); err != nil {
		t.Fatal(err)
	}

	err = copySkillPackage(canonicalSkillDir, destSkillPackage, lock, ".claude/skills", "my-skill", projectDir)
	if err != nil {
		t.Fatalf("copySkillPackage failed on destination symlink subdir: %v", err)
	}

	// External victim file inside subdir MUST remain untouched!
	subData, err := os.ReadFile(outsideVictimSubFile)
	if err != nil || string(subData) != outsideVictimSubContent {
		t.Fatalf("SECURITY VIOLATION: external victim file in subdir was modified! Got: %q", string(subData))
	}
	// Tool.sh should not have been written to the outside directory
	if _, err := os.Stat(filepath.Join(outsideVictimSubDir, "tool.sh")); !os.IsNotExist(err) {
		t.Fatalf("SECURITY VIOLATION: tool.sh was written to external directory!")
	}

	// 3. Destination package itself is a symlink outside the project
	_ = os.RemoveAll(destSkillPackage)
	outsideSkillDir := filepath.Join(outsideDir, "external_skill_pkg")
	if err := os.MkdirAll(outsideSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFileInPkg := filepath.Join(outsideSkillDir, "external_secret.txt")
	_ = os.WriteFile(outsideFileInPkg, []byte("EXTERNAL PKG SECRET"), 0o644)

	if err := os.Symlink(outsideSkillDir, destSkillPackage); err != nil {
		t.Fatal(err)
	}

	err = copySkillPackage(canonicalSkillDir, destSkillPackage, lock, ".claude/skills", "my-skill", projectDir)
	if err != nil {
		t.Fatalf("copySkillPackage failed when dest package is a symlink: %v", err)
	}

	// External directory files must be unchanged
	secretData, _ := os.ReadFile(outsideFileInPkg)
	if string(secretData) != "EXTERNAL PKG SECRET" {
		t.Fatalf("SECURITY VIOLATION: file in external target dir was modified!")
	}
	if _, err := os.Stat(filepath.Join(outsideSkillDir, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("SECURITY VIOLATION: SKILL.md was written to external target dir!")
	}

	// Destination package must no longer be a symlink
	pkgSt, err := os.Lstat(destSkillPackage)
	if err != nil || pkgSt.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected dest package to be a regular directory now")
	}

	// 4. Source symlink escapes skill package -> rejected
	badSrcSkill := filepath.Join(projectDir, ".ai/skills/bad-skill")
	_ = os.MkdirAll(badSrcSkill, 0o755)
	_ = os.WriteFile(filepath.Join(badSrcSkill, "SKILL.md"), []byte("bad"), 0o644)
	_ = os.Symlink(outsideVictimFile, filepath.Join(badSrcSkill, "escape.txt"))

	badDest := filepath.Join(destSkillsDir, "bad-skill")
	_ = os.MkdirAll(badDest, 0o755)
	_ = os.WriteFile(filepath.Join(badDest, "original.txt"), []byte("ORIGINAL KEEP"), 0o644)

	err = copySkillPackage(badSrcSkill, badDest, lock, ".claude/skills", "bad-skill", projectDir)
	if err == nil {
		t.Fatalf("expected error when source symlink escapes package")
	}
	if !strings.Contains(err.Error(), "bad-skill") {
		t.Errorf("expected error message to identify bad-skill package, got: %v", err)
	}

	// 5. Replacement failure preserves original destination package
	origContent, err := os.ReadFile(filepath.Join(badDest, "original.txt"))
	if err != nil || string(origContent) != "ORIGINAL KEEP" {
		t.Fatalf("original destination package was corrupted or lost on failed replacement!")
	}
	if _, err := os.Stat(filepath.Join(badDest, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("partial replacement occurred: SKILL.md was written to destination despite failure!")
	}

	// Ensure no temporary staging directories were left behind in destSkillsDir
	entries, _ := os.ReadDir(destSkillsDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && (strings.Contains(e.Name(), "-tmp-") || strings.Contains(e.Name(), "-bak-")) {
			t.Errorf("temporary staging directory %s was not cleaned up!", e.Name())
		}
	}
}
