package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/f24aalam/agentsync/internal/config"
)

func TestInstallGuidelinesMergesAlphabetically(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/guidelines/b.md", "B")
	mustWriteFile(t, ".ai/guidelines/a.md", "A")
	mustWriteFile(t, ".ai/guidelines/ignore.txt", "X")

	target, _ := ByID("claude-code")
	result := installGuidelines(target, ".", false)
	if result.Status != StepStatusOK {
		t.Fatalf("expected guidelines install ok, got %+v", result)
	}

	data, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatalf("read merged guidelines: %v", err)
	}

	want := ManagedSectionStart + "\nA\n\nB\n" + ManagedSectionEnd + "\n"
	if string(data) != want {
		t.Fatalf("expected alphabetical merge in managed block, got %q", string(data))
	}
}

func TestInstallGuidelinesCursorUsesAgentsyncFile(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/guidelines/core.md", "content")

	target, _ := ByID("cursor")
	result := installGuidelines(target, ".", false)
	if result.Target != ".cursor/rules/agentsync.mdc" {
		t.Fatalf("expected cursor target path, got %q", result.Target)
	}

	assertFileContains(t, ".cursor/rules/agentsync.mdc", "content")
}

func TestInstallSkillsCopiesNestedFiles(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/skills/example-skill/SKILL.md", "skill")
	mustWriteFile(t, ".ai/skills/example-skill/assets/data.txt", "nested")

	target, _ := ByID("codex")
	result := installSkills(target, ".")
	if result.Status != StepStatusOK {
		t.Fatalf("expected skills install ok, got %+v", result)
	}

	assertFileContains(t, ".agents/skills/example-skill/SKILL.md", "skill")
	assertFileContains(t, ".agents/skills/example-skill/assets/data.txt", "nested")
}

func TestInstallMCPSkipsWhenMissing(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	target, _ := ByID("claude-code")
	result := installMCP(target, ".", false)
	if result.Status != StepStatusSkipped {
		t.Fatalf("expected skipped MCP result, got %+v", result)
	}
}

func TestInstallMCPWritesJSONAndTOML(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]
`)

	jsonAgent, _ := ByID("claude-code")
	tomlAgent, _ := ByID("codex")

	jsonResult := installMCP(jsonAgent, ".", false)
	if jsonResult.Status != StepStatusOK {
		t.Fatalf("expected json MCP result ok, got %+v", jsonResult)
	}
	tomlResult := installMCP(tomlAgent, ".", false)
	if tomlResult.Status != StepStatusOK {
		t.Fatalf("expected toml MCP result ok, got %+v", tomlResult)
	}

	assertFileContains(t, ".mcp.json", `"mcpServers"`)
	assertFileContains(t, ".codex/config.toml", "[mcp_servers.postgres]")
}

func TestInstallAggregatesStepErrors(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	mustWriteFile(t, ".ai/guidelines/core.md", "content")
	target := Agent{
		ID:             "broken",
		Name:           "Broken",
		GuidelinesFile: ".",
		SkillsDir:      ".skills",
		MCPConfig:      ".mcp.json",
		MCPFormat:      MCPFormatJSON,
	}

	sk := StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusSkipped}
	result := InstallAgent(target, ".", sk, NewInstallPlan())
	if result.Succeeded() {
		t.Fatalf("expected failed install result")
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	return wd
}

func mustChdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimPrefix(content, "\n")), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if !strings.Contains(string(data), want) {
		t.Fatalf("expected %s to contain %q, got %q", path, want, string(data))
	}
}

func TestInjectManagedGuidelines_MalformedMarkers(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "missing end marker",
			content: "# Header\n" + ManagedSectionStart + "\nManaged content without end",
			wantErr: "missing counterpart",
		},
		{
			name:    "missing begin marker",
			content: "# Header\nManaged content without start\n" + ManagedSectionEnd,
			wantErr: "missing counterpart",
		},
		{
			name:    "reversed markers",
			content: "# Header\n" + ManagedSectionEnd + "\nInside\n" + ManagedSectionStart,
			wantErr: "end marker appears before begin marker",
		},
		{
			name:    "duplicate begin markers",
			content: ManagedSectionStart + "\nFirst\n" + ManagedSectionStart + "\nSecond\n" + ManagedSectionEnd,
			wantErr: "duplicate managed markers",
		},
		{
			name:    "duplicate end markers",
			content: ManagedSectionStart + "\nContent\n" + ManagedSectionEnd + "\n" + ManagedSectionEnd,
			wantErr: "duplicate managed markers",
		},
		{
			name:    "nested markers",
			content: ManagedSectionStart + "\n" + ManagedSectionStart + "\nInner\n" + ManagedSectionEnd + "\n" + ManagedSectionEnd,
			wantErr: "duplicate managed markers",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := injectManagedGuidelines(tc.content, "new managed content")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestInjectManagedGuidelines_PreservesExactUserBytes(t *testing.T) {
	// Pre-existing user content with exact whitespace, indentation, and newlines
	userBefore := "# Team Rules\n\n   Indented paragraph with spaces.  \n\n\n"
	userAfter := "\n\n## Footer Notes\n\tTab-indented text\n   Trailing spaces on line   \n"
	initialContent := userBefore + ManagedSectionStart + "\nOld guidelines\n" + ManagedSectionEnd + userAfter

	newManaged := "New managed guidelines v1"
	updated, err := injectManagedGuidelines(initialContent, newManaged)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify user content before markers is preserved byte-for-byte
	if !strings.HasPrefix(updated, userBefore) {
		t.Fatalf("expected exact prefix byte preservation:\nwant: %q\ngot:  %q", userBefore, updated[:len(userBefore)])
	}

	// Verify user content after markers is preserved byte-for-byte
	if !strings.HasSuffix(updated, userAfter) {
		t.Fatalf("expected exact suffix byte preservation:\nwant: %q\ngot:  %q", userAfter, updated[len(updated)-len(userAfter):])
	}

	// Repeated call with same managed content must be byte-for-byte identical
	repeated, err := injectManagedGuidelines(updated, newManaged)
	if err != nil {
		t.Fatalf("unexpected error on repeated inject: %v", err)
	}
	if repeated != updated {
		t.Fatalf("expected idempotent output on repeated run:\nFirst:\n%q\nSecond:\n%q", updated, repeated)
	}
}

func TestUpdateTomlMCPServers_PreservesCommentsAndFormatting(t *testing.T) {
	initialTOML := `# Codex Global Configuration
# Maintained by DevOps

model = "gpt-4o" # default model
temperature = 0.2

# Custom developer tools section
[custom_section]
key = "important_value"
# internal comment inside custom_section
flag = true

# User unmanaged MCP server
[mcp_servers.my_custom_tool]
command = "my-tool"
args = ["--verbose"]

# Old managed server
[mcp_servers.postgres]
command = "old-pg"

# Trailing footer section
[footer_section]
enabled = false
`

	servers := map[string]config.MCPServer{
		"postgres": {
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-postgres"},
			Env:     map[string]string{"PORT": "5432"},
		},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("updateTomlMCPServers failed: %v", err)
	}

	// 1. Comments preserved
	expectedComments := []string{
		"# Codex Global Configuration",
		"# Maintained by DevOps",
		"# default model",
		"# Custom developer tools section",
		"# internal comment inside custom_section",
		"# User unmanaged MCP server",
		"# Trailing footer section",
	}
	for _, c := range expectedComments {
		if !strings.Contains(updated, c) {
			t.Errorf("expected comment %q preserved in output:\n%s", c, updated)
		}
	}

	// 2. Unmanaged settings and sections preserved
	expectedSections := []string{
		`model = "gpt-4o"`,
		"temperature = 0.2",
		"[custom_section]",
		`key = "important_value"`,
		"flag = true",
		"[mcp_servers.my_custom_tool]",
		`command = "my-tool"`,
		`args = ["--verbose"]`,
		"[footer_section]",
		"enabled = false",
	}
	for _, s := range expectedSections {
		if !strings.Contains(updated, s) {
			t.Errorf("expected section %q preserved in output:\n%s", s, updated)
		}
	}

	// 3. Managed server updated
	if strings.Contains(updated, `command = "old-pg"`) {
		t.Errorf("expected old managed server command replaced")
	}
	if !strings.Contains(updated, "[mcp_servers.postgres]") {
		t.Errorf("expected [mcp_servers.postgres] in output")
	}
	if !strings.Contains(updated, `command = "npx"`) {
		t.Errorf("expected command = \"npx\" in output")
	}
	if !strings.Contains(updated, `[mcp_servers.postgres.env]`) || !strings.Contains(updated, `PORT = "5432"`) {
		t.Errorf("expected postgres env in output:\n%s", updated)
	}

	// 4. Repeated update is byte-for-byte identical
	repeated, err := updateTomlMCPServers(updated, servers)
	if err != nil {
		t.Fatalf("repeated update failed: %v", err)
	}
	if repeated != updated {
		t.Fatalf("expected idempotent repeated update:\nFirst:\n%s\nSecond:\n%s", updated, repeated)
	}
}

func TestUpdateTomlMCPServers_RejectsMalformedTOML(t *testing.T) {
	malformed := `# Corrupted TOML
model = "unclosed string without closing quote
[mcp_servers]
`
	servers := map[string]config.MCPServer{
		"postgres": {Command: "npx"},
	}

	_, err := updateTomlMCPServers(malformed, servers)
	if err == nil {
		t.Fatalf("expected error on malformed TOML, got nil")
	}
	if !strings.Contains(err.Error(), "cannot parse existing TOML config") {
		t.Fatalf("expected 'cannot parse existing TOML config' in error, got: %v", err)
	}
}

func TestInjectManagedGuidelines_LineEndingsAndDrift(t *testing.T) {
	// 1. CRLF line endings preservation
	userCRLF := "# Header\r\n\r\nCustom notes.\r\n"
	managed := "Managed paragraph 1\nManaged paragraph 2"
	withCRLF, err := injectManagedGuidelines(userCRLF, managed)
	if err != nil {
		t.Fatalf("inject with CRLF failed: %v", err)
	}
	if !strings.Contains(withCRLF, "\r\n") {
		t.Errorf("expected CRLF line endings preserved")
	}
	if !strings.HasPrefix(withCRLF, userCRLF) {
		t.Errorf("expected exact CRLF prefix preserved:\nWant: %q\nGot: %q", userCRLF, withCRLF[:len(userCRLF)])
	}

	// Repeated inject with CRLF must be byte-for-byte identical
	repeatedCRLF, err := injectManagedGuidelines(withCRLF, managed)
	if err != nil {
		t.Fatalf("repeated inject failed: %v", err)
	}
	if repeatedCRLF != withCRLF {
		t.Errorf("drift detected on repeated CRLF inject:\nFirst:\n%q\nSecond:\n%q", withCRLF, repeatedCRLF)
	}

	// 2. Missing final newline in existing file
	userNoFinalNewline := "# No final newline"
	withNoFinal, err := injectManagedGuidelines(userNoFinalNewline, "Managed")
	if err != nil {
		t.Fatalf("inject without final newline failed: %v", err)
	}
	if !strings.HasPrefix(withNoFinal, userNoFinalNewline) {
		t.Errorf("expected prefix preserved")
	}

	// 3. User content before and after managed section with arbitrary spacing
	userBefore := "# Top Note\n\n"
	userAfter := "\n\n# Bottom Note\n"
	existing := userBefore + ManagedSectionStart + "\nOld\n" + ManagedSectionEnd + userAfter
	updated, err := injectManagedGuidelines(existing, "New Content")
	if err != nil {
		t.Fatalf("inject failed: %v", err)
	}
	if !strings.HasPrefix(updated, userBefore) {
		t.Errorf("expected exact userBefore preserved")
	}
	if !strings.HasSuffix(updated, userAfter) {
		t.Errorf("expected exact userAfter preserved")
	}

	// Idempotency check across 3 iterations
	it2, _ := injectManagedGuidelines(updated, "New Content")
	it3, _ := injectManagedGuidelines(it2, "New Content")
	if it2 != updated || it3 != updated {
		t.Errorf("formatting drift detected across repeated iterations!")
	}
}

func TestInstallGuidelines_LeavesUntouchedWhenEmptied(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	// Setup initial guideline
	mustWriteFile(t, ".ai/guidelines/a.md", "content A")
	target, _ := ByID("claude-code")
	lock := config.NewLockFile()
	plan := NewInstallPlan()
	plan.Lock = lock

	res := installGuidelinesWithLock(target, ".", false, lock)
	if res.Status != StepStatusOK {
		t.Fatalf("expected ok, got %+v", res)
	}

	// Remove guidelines from .ai/guidelines
	_ = os.Remove(".ai/guidelines/a.md")

	// In v1, install skips when canonical source is empty and leaves previously distributed output untouched
	res2 := installGuidelinesWithLock(target, ".", false, lock)
	if res2.Status != StepStatusSkipped {
		t.Fatalf("expected skipped status when canonical guidelines are empty, got %+v", res2)
	}

	// CLAUDE.md must remain untouched
	if _, err := os.Stat("CLAUDE.md"); err != nil {
		t.Fatalf("expected CLAUDE.md to remain untouched, got err: %v", err)
	}
	data, _ := os.ReadFile("CLAUDE.md")
	if !strings.Contains(string(data), "content A") {
		t.Fatalf("expected CLAUDE.md content to be preserved, found: %q", string(data))
	}
}

func TestSkills_PackageSyncAndNoOrphanPruning(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	// 1. Initial skill setup with 2 files
	mustWriteFile(t, ".ai/skills/code-review/SKILL.md", "Skill v1")
	mustWriteFile(t, ".ai/skills/code-review/scripts/review.sh", "echo review")

	lock := config.NewLockFile()
	destDir := ".claude/skills/"

	// First sync
	if err := syncSkillTrees(".", destDir, lock); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	skillKey := filepath.Join(destDir, "code-review/SKILL.md")
	scriptKey := filepath.Join(destDir, "code-review/scripts/review.sh")

	// Verify target files exist
	assertFileContains(t, skillKey, "Skill v1")
	assertFileContains(t, scriptKey, "echo review")

	// 2. Add an unrelated sibling skill directly in destination (must NEVER be touched)
	siblingSkill := filepath.Join(destDir, "user-custom-skill/SKILL.md")
	mustWriteFile(t, siblingSkill, "User standalone skill")

	// 3. In .ai/skills, delete review.sh and update SKILL.md
	_ = os.Remove(".ai/skills/code-review/scripts/review.sh")
	mustWriteFile(t, ".ai/skills/code-review/SKILL.md", "Skill v2")

	// Second sync
	if err := syncSkillTrees(".", destDir, lock); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}

	// SKILL.md should be updated
	assertFileContains(t, skillKey, "Skill v2")

	// Stale file inside the updated package is removed so package matches canonical source
	if _, err := os.Stat(scriptKey); !os.IsNotExist(err) {
		t.Errorf("expected stale file %s inside managed package to be cleaned up", scriptKey)
	}

	// Unrelated sibling skill must NOT be touched
	assertFileContains(t, siblingSkill, "User standalone skill")

	// 4. In v1: when a canonical skill is removed from .ai/skills, previously distributed output is NOT deleted
	mustWriteFile(t, ".ai/skills/temp-skill/SKILL.md", "Temporary skill")
	if err := syncSkillTrees(".", destDir, lock); err != nil {
		t.Fatalf("sync temp skill failed: %v", err)
	}
	tempKey := filepath.Join(destDir, "temp-skill/SKILL.md")
	assertFileContains(t, tempKey, "Temporary skill")

	// Remove temp-skill from .ai/skills
	_ = os.RemoveAll(".ai/skills/temp-skill")

	if err := syncSkillTrees(".", destDir, lock); err != nil {
		t.Fatalf("sync after temp skill removed failed: %v", err)
	}

	// In v1: previously distributed output remains untouched
	assertFileContains(t, tempKey, "Temporary skill")

	// Sibling skill still preserved
	assertFileContains(t, siblingSkill, "User standalone skill")

	// Root skills dir must NEVER be deleted
	if _, err := os.Stat(destDir); os.IsNotExist(err) {
		t.Errorf("SAFETY VIOLATION: root skills dir %s was deleted!", destDir)
	}
}

func TestSkills_PackageConflictDecisions_ApproveVsDeclineAndYes(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	// Canonical skill: testing
	mustWriteFile(t, ".ai/skills/testing/SKILL.md", "Canonical testing skill v1")
	mustWriteFile(t, ".ai/skills/testing/scripts/test.sh", "canonical test")

	destDir := ".claude/skills"
	pkgPath := filepath.Join(destDir, "testing")
	pkgKey := SkillDirKey(destDir) + "/testing"

	// 1. Pre-existing destination skill with extra developer file
	mustWriteFile(t, filepath.Join(pkgPath, "SKILL.md"), "Pre-existing different skill")
	mustWriteFile(t, filepath.Join(pkgPath, "personal-notes.md"), "Do not lose my notes")

	// Pre-existing package is UNKNOWN (not in lock)
	lock := config.NewLockFile()
	if lock.IsSkillPackageManaged(destDir, "testing") {
		t.Fatalf("expected package to be unmanaged initially")
	}

	// Case A: User declines overwrite (plan.SkipSkillPackages[pkgKey] = true)
	planDecline := NewInstallPlan()
	planDecline.Lock = lock
	planDecline.SkipSkillPackages[pkgKey] = true

	if err := syncSkillTreesWithPlan(".", destDir, planDecline); err != nil {
		t.Fatalf("sync with decline failed: %v", err)
	}

	// Verify complete package is UNCHANGED (Rule 5 & 7)
	assertFileContains(t, filepath.Join(pkgPath, "SKILL.md"), "Pre-existing different skill")
	assertFileContains(t, filepath.Join(pkgPath, "personal-notes.md"), "Do not lose my notes")
	if _, err := os.Stat(filepath.Join(pkgPath, "scripts/test.sh")); !os.IsNotExist(err) {
		t.Fatalf("scripts/test.sh should not have been copied after user declined")
	}

	// Case B: User approves overwrite
	planApprove := NewInstallPlan()
	planApprove.Lock = lock

	if err := syncSkillTreesWithPlan(".", destDir, planApprove); err != nil {
		t.Fatalf("sync with approval failed: %v", err)
	}

	// Verify package matches canonical exactly (Rule 4: extra files removed after approval)
	assertFileContains(t, filepath.Join(pkgPath, "SKILL.md"), "Canonical testing skill v1")
	assertFileContains(t, filepath.Join(pkgPath, "scripts/test.sh"), "canonical test")
	if _, err := os.Stat(filepath.Join(pkgPath, "personal-notes.md")); !os.IsNotExist(err) {
		t.Fatalf("personal-notes.md should have been cleaned up after explicit package approval")
	}
	if !lock.IsSkillPackageManaged(destDir, "testing") {
		t.Fatalf("expected package to be recorded as managed after approval")
	}

	// Case C: Canonical update to v2, destination modified -> with managed lock, --yes approves update (Rule 8)
	mustWriteFile(t, ".ai/skills/testing/SKILL.md", "Canonical testing skill v2")
	mustWriteFile(t, filepath.Join(pkgPath, "SKILL.md"), "User modified v1.5")
	mustWriteFile(t, filepath.Join(pkgPath, "notes2.md"), "Temporary file")

	planYes := NewInstallPlan()
	planYes.Lock = lock

	if err := syncSkillTreesWithPlan(".", destDir, planYes); err != nil {
		t.Fatalf("sync with yes failed: %v", err)
	}

	assertFileContains(t, filepath.Join(pkgPath, "SKILL.md"), "Canonical testing skill v2")
	if _, err := os.Stat(filepath.Join(pkgPath, "notes2.md")); !os.IsNotExist(err) {
		t.Fatalf("notes2.md should have been cleaned up in approved package update")
	}
}

func TestMCP_SyncAndSafePruning_JSONAndTOML(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	// 1. JSON test (.mcp.json for Claude Code)
	claude, _ := ByID("claude-code")
	lock := config.NewLockFile()

	// Initial configuration with 2 servers: postgres and redis
	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "pg-cmd"
args = ["--port", "5432"]

[servers.redis]
command = "redis-cmd"
`)

	res := installMCPWithLock(claude, ".", false, lock)
	if res.Status != StepStatusOK {
		t.Fatalf("initial json install failed: %+v", res)
	}

	assertFileContains(t, ".mcp.json", `"postgres"`)
	assertFileContains(t, ".mcp.json", `"redis"`)

	// User adds an unmanaged server directly in .mcp.json
	userJSON := `{
  "mcpServers": {
    "postgres": { "command": "pg-cmd", "args": ["--port", "5432"] },
    "redis": { "command": "redis-cmd" },
    "my_user_tool": { "command": "my-tool" }
  }
}`
	mustWriteFile(t, ".mcp.json", userJSON)

	// Remove redis from .ai/mcp.toml
	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "pg-cmd"
args = ["--port", "5432"]
`)

	res2 := installMCPWithLock(claude, ".", false, lock)
	if res2.Status != StepStatusOK {
		t.Fatalf("second json install failed: %+v", res2)
	}

	// postgres should remain
	assertFileContains(t, ".mcp.json", `"postgres"`)
	// unmanaged user tool must remain
	assertFileContains(t, ".mcp.json", `"my_user_tool"`)
	// in v1, deleted redis is not pruned (no automatic orphan cleanup)
	assertFileContains(t, ".mcp.json", `"redis"`)

	// 2. TOML test (.codex/config.toml for Codex)
	codex, _ := ByID("codex")
	codexLock := config.NewLockFile()

	initialCodexTOML := `# Developer configuration
model = "gpt-4o"

[mcp_servers.developer_tool]
command = "dev-tool"

[mcp_servers.postgres]
command = "pg-cmd"
`
	mustWriteFile(t, ".codex/config.toml", initialCodexTOML)

	// In .ai/mcp.toml: postgres and search
	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "pg-cmd"

[servers.search]
command = "search-cmd"
`)

	resTOML := installMCPWithLock(codex, ".", false, codexLock)
	if resTOML.Status != StepStatusOK {
		t.Fatalf("initial toml install failed: %+v", resTOML)
	}

	assertFileContains(t, ".codex/config.toml", `model = "gpt-4o"`)
	assertFileContains(t, ".codex/config.toml", "[mcp_servers.developer_tool]")
	assertFileContains(t, ".codex/config.toml", "[mcp_servers.postgres]")
	assertFileContains(t, ".codex/config.toml", "[mcp_servers.search]")

	// Remove search from .ai/mcp.toml
	mustWriteFile(t, ".ai/mcp.toml", `
[servers.postgres]
command = "pg-cmd"
`)

	resTOML2 := installMCPWithLock(codex, ".", false, codexLock)
	if resTOML2.Status != StepStatusOK {
		t.Fatalf("second toml install failed: %+v", resTOML2)
	}

	codexContent := readFile(t, ".codex/config.toml")
	if !strings.Contains(codexContent, `model = "gpt-4o"`) {
		t.Errorf("model setting lost from .codex/config.toml")
	}
	if !strings.Contains(codexContent, "[mcp_servers.developer_tool]") {
		t.Errorf("unmanaged developer tool lost from .codex/config.toml")
	}
	if !strings.Contains(codexContent, "[mcp_servers.postgres]") {
		t.Errorf("postgres lost from .codex/config.toml")
	}
	// in v1, deleted search is not pruned (no automatic orphan cleanup)
	if !strings.Contains(codexContent, "[mcp_servers.search]") {
		t.Errorf("expected deleted managed server 'search' to remain in .codex/config.toml in v1!\nGot:\n%s", codexContent)
	}
}

func TestUpdateTomlMCPServers_MultilineStringsContainingFakeTableHeaders(t *testing.T) {
	initialTOML := `# System prompt configuration
model = "gpt-4o"

prompt = """
Example configuration below:
[mcp_servers.fake]
command = "echo fake"
args = ["test"]
"""

literal_prompt = '''
Another literal example:
[mcp_servers.fake_literal]
command = "echo literal"
'''

[mcp_servers.existing]
command = "real-tool"
`
	servers := map[string]config.MCPServer{
		"new_tool": {Command: "new-cmd"},
	}

	updated, err := updateTomlMCPServers(initialTOML, servers)
	if err != nil {
		t.Fatalf("updateTomlMCPServers failed: %v", err)
	}

	// 1. Both multiline strings must be preserved completely intact
	if !strings.Contains(updated, "[mcp_servers.fake]") {
		t.Errorf("multiline basic string was corrupted! Missing [mcp_servers.fake]:\n%s", updated)
	}
	if !strings.Contains(updated, `command = "echo fake"`) {
		t.Errorf("multiline basic string content lost:\n%s", updated)
	}
	if !strings.Contains(updated, "[mcp_servers.fake_literal]") {
		t.Errorf("multiline literal string was corrupted! Missing [mcp_servers.fake_literal]:\n%s", updated)
	}
	if !strings.Contains(updated, `command = "echo literal"`) {
		t.Errorf("multiline literal string content lost:\n%s", updated)
	}

	// 2. Unmanaged real server preserved
	if !strings.Contains(updated, "[mcp_servers.existing]") {
		t.Errorf("existing unmanaged server was lost:\n%s", updated)
	}

	// 3. New managed server added
	if !strings.Contains(updated, "[mcp_servers.new_tool]") {
		t.Errorf("new managed server was not added:\n%s", updated)
	}

	// 4. Repeated update is 100% byte-for-byte identical
	repeated, err := updateTomlMCPServers(updated, servers)
	if err != nil {
		t.Fatalf("repeated update failed: %v", err)
	}
	if repeated != updated {
		t.Errorf("drift detected across repeated runs:\nFirst:\n%s\nSecond:\n%s", updated, repeated)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestSymlinks_ValidationAndSafetyCases(t *testing.T) {
	tempDir := t.TempDir()

	pkgRoot := filepath.Join(tempDir, "source_pkg")
	dstRoot := filepath.Join(tempDir, "dest_pkg")
	if err := os.MkdirAll(pkgRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	// Outside file that should NEVER be accessed or linked
	outsideFile := filepath.Join(tempDir, "secret_outside.txt")
	if err := os.WriteFile(outsideFile, []byte("TOP SECRET DO NOT TOUCH"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Valid relative symlink within the same directory
	if err := os.WriteFile(filepath.Join(pkgRoot, "real.txt"), []byte("real content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(pkgRoot, "valid_same_dir.txt")); err != nil {
		t.Skipf("symlinks not supported on filesystem: %v", err)
	}

	err := CopyFileWithinPackage(filepath.Join(pkgRoot, "valid_same_dir.txt"), filepath.Join(dstRoot, "valid_same_dir.txt"), pkgRoot)
	if err != nil {
		t.Fatalf("expected valid internal relative symlink to succeed, got: %v", err)
	}
	fi, err := os.Lstat(filepath.Join(dstRoot, "valid_same_dir.txt"))
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected dest to be a symlink")
	}

	// 2. Valid relative symlink using .. that still resolves within the package
	if err := os.MkdirAll(filepath.Join(pkgRoot, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../real.txt", filepath.Join(pkgRoot, "sub/parent_rel.txt")); err != nil {
		t.Fatal(err)
	}

	err = CopyFileWithinPackage(filepath.Join(pkgRoot, "sub/parent_rel.txt"), filepath.Join(dstRoot, "sub/parent_rel.txt"), pkgRoot)
	if err != nil {
		t.Fatalf("expected valid parent-relative symlink within package to succeed, got: %v", err)
	}

	// 3. Reject relative symlink that escapes the package
	if err := os.Symlink("../../secret_outside.txt", filepath.Join(pkgRoot, "sub/escape.txt")); err != nil {
		t.Fatal(err)
	}
	err = CopyFileWithinPackage(filepath.Join(pkgRoot, "sub/escape.txt"), filepath.Join(dstRoot, "sub/escape.txt"), pkgRoot)
	if err == nil {
		t.Fatalf("expected escaping relative symlink to be rejected")
	}
	if !strings.Contains(err.Error(), "escapes skill package") {
		t.Errorf("expected escaping error message, got: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dstRoot, "sub/escape.txt")); !os.IsNotExist(err) {
		t.Errorf("SECURITY VIOLATION: escaping symlink was created at destination!")
	}

	// 4. Reject absolute symlink escaping package
	if err := os.Symlink(outsideFile, filepath.Join(pkgRoot, "absolute.txt")); err != nil {
		t.Fatal(err)
	}
	err = CopyFileWithinPackage(filepath.Join(pkgRoot, "absolute.txt"), filepath.Join(dstRoot, "absolute.txt"), pkgRoot)
	if err == nil {
		t.Fatalf("expected absolute symlink to be rejected")
	}
	if !strings.Contains(err.Error(), "escapes skill package") {
		t.Errorf("expected absolute error message, got: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dstRoot, "absolute.txt")); !os.IsNotExist(err) {
		t.Errorf("SECURITY VIOLATION: absolute symlink was created at destination!")
	}

	// 5. Reject broken symlink
	if err := os.Symlink("nonexistent.txt", filepath.Join(pkgRoot, "broken.txt")); err != nil {
		t.Fatal(err)
	}
	err = CopyFileWithinPackage(filepath.Join(pkgRoot, "broken.txt"), filepath.Join(dstRoot, "broken.txt"), pkgRoot)
	if err == nil {
		t.Fatalf("expected broken symlink to be rejected")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected broken symlink error, got: %v", err)
	}

	// 6. Reject circular symlink
	if err := os.Symlink("circ_b.txt", filepath.Join(pkgRoot, "circ_a.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("circ_a.txt", filepath.Join(pkgRoot, "circ_b.txt")); err != nil {
		t.Fatal(err)
	}
	err = CopyFileWithinPackage(filepath.Join(pkgRoot, "circ_a.txt"), filepath.Join(dstRoot, "circ_a.txt"), pkgRoot)
	if err == nil {
		t.Fatalf("expected circular symlink to be rejected")
	}

	// 7. Reject symlink pointing through intermediate symlink escaping package
	if err := os.Symlink("../secret_outside.txt", filepath.Join(pkgRoot, "hop.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("hop.txt", filepath.Join(pkgRoot, "indirect.txt")); err != nil {
		t.Fatal(err)
	}
	err = CopyFileWithinPackage(filepath.Join(pkgRoot, "indirect.txt"), filepath.Join(dstRoot, "indirect.txt"), pkgRoot)
	if err == nil {
		t.Fatalf("expected indirect escaping symlink to be rejected")
	}
}

func TestMCP_DestinationAwareConflictsAndDecisions(t *testing.T) {
	wd := mustGetwd(t)
	tempDir := t.TempDir()
	mustChdir(t, tempDir)
	defer mustChdir(t, wd)

	// Canonical config
	mustWriteFile(t, ".ai/mcp.toml", `
[servers.fetch]
type = "local"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-fetch"]

[servers.postgres]
type = "local"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-postgres"]
`)

	// Existing Codex config with an unmanaged "fetch" (different cmd), an unmanaged "user-tool", and custom setting
	existingCodex := `# Codex preferences
model = "o3-mini"

[mcp_servers.fetch]
command = "old-fetch"

[mcp_servers.user_tool]
command = "my-custom-tool"
`
	mustWriteFile(t, ".codex/config.toml", existingCodex)

	codex, _ := ByID("codex")
	conflicts, err := DetectInstallConflicts([]Agent{codex}, ".")
	if err != nil {
		t.Fatalf("unexpected error detecting conflicts: %v", err)
	}

	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(conflicts), conflicts)
	}
	c := conflicts[0]
	if c.Kind != KindMCP {
		t.Fatalf("expected KindMCP, got %v", c.Kind)
	}
	if !slices.Equal(c.ConflictingServers, []string{"fetch", "postgres"}) {
		t.Errorf("expected conflicting servers [fetch postgres], got %v", c.ConflictingServers)
	}
	expectedPromptSnippet := "Codex MCP configuration differs:\n.codex/config.toml\n\nConflicting servers:\n- fetch\n- postgres\n\nReplace these entries using .ai/mcp.toml?"
	if c.Question != expectedPromptSnippet {
		t.Errorf("prompt mismatch.\nWant:\n%s\nGot:\n%s", expectedPromptSnippet, c.Question)
	}
	if c.IsManaged {
		t.Errorf("expected IsManaged to be false since fetch is unmanaged in destination")
	}

	// 1. Decline overwrite: destination must remain untouched
	planDecline := NewInstallPlan()
	planDecline.SkipMCPDest[".codex/config.toml"] = true
	resDecline := installMCPWithPlan(codex, ".", planDecline)
	if resDecline.Status != StepStatusSkipped {
		t.Fatalf("expected skipped status, got %+v", resDecline)
	}
	contentAfterDecline, _ := os.ReadFile(".codex/config.toml")
	if string(contentAfterDecline) != existingCodex {
		t.Fatalf("declined install modified destination file!")
	}

	// 2. Approve overwrite: managed servers updated/inserted, unmanaged and settings preserved
	planApprove := NewInstallPlan()
	resApprove := installMCPWithPlan(codex, ".", planApprove)
	if resApprove.Status != StepStatusOK {
		t.Fatalf("expected ok, got %+v", resApprove)
	}
	contentApproved, _ := os.ReadFile(".codex/config.toml")
	approvedStr := string(contentApproved)

	if !strings.Contains(approvedStr, `model = "o3-mini"`) {
		t.Errorf("unrelated settings lost after approve! Content:\n%s", approvedStr)
	}
	if !strings.Contains(approvedStr, "user_tool") || !strings.Contains(approvedStr, "my-custom-tool") {
		t.Errorf("unmanaged server lost after approve! Content:\n%s", approvedStr)
	}
	if !strings.Contains(approvedStr, "@modelcontextprotocol/server-fetch") {
		t.Errorf("managed server fetch not updated! Content:\n%s", approvedStr)
	}
	if !strings.Contains(approvedStr, "@modelcontextprotocol/server-postgres") {
		t.Errorf("managed server postgres not inserted! Content:\n%s", approvedStr)
	}

	// 3. Repeating install when already identical must be skipped (no conflict)
	conflictsRepeat, err := DetectInstallConflicts([]Agent{codex}, ".")
	if err != nil {
		t.Fatalf("repeat conflict check failed: %v", err)
	}
	for _, cr := range conflictsRepeat {
		if cr.Kind == KindMCP {
			t.Errorf("expected no MCP conflict when already identical, got: %+v", cr)
		}
	}
}
