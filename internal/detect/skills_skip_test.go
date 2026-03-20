package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/f24aalam/agentsync/internal/agent"
)

func TestDetectSkillsSkipsWhenUnsupported(t *testing.T) {
	tempDir := t.TempDir()

	// Even if the skills directory exists with a SKILL.md, Gemini should be
	// skipped when SkillsSupported=false.
	if err := os.MkdirAll(filepath.Join(tempDir, ".agents", "skills", "s1"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".agents", "skills", "s1", "SKILL.md"), []byte("name: x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	gemini := agent.Agent{
		ID:             "gemini-cli",
		Name:           "Gemini CLI",
		SkillsDir:      ".agents/skills/",
		SkillsSupported: false,
	}

	skills, err := detectSkills(tempDir, gemini)
	if err != nil {
		t.Fatalf("detectSkills error: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(skills))
	}
}

