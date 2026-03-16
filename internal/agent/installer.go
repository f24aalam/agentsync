package agent

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/f24aalam/agentsync/internal/config"
)

const (
	guidelinesSourceDir = ".ai/guidelines"
	skillsSourceDir     = ".ai/skills"
	mcpSourcePath       = ".ai/mcp.toml"
	cursorGuidelinesOut = ".cursor/rules/agentsync.mdc"
)

type StepStatus string

const (
	StepStatusOK      StepStatus = "ok"
	StepStatusSkipped StepStatus = "skipped"
	StepStatusError   StepStatus = "error"
)

type StepResult struct {
	Name   string
	Target string
	Status StepStatus
	Err    error
}

type InstallResult struct {
	Agent Agent
	Steps []StepResult
}

func (r InstallResult) Succeeded() bool {
	for _, step := range r.Steps {
		if step.Status == StepStatusError {
			return false
		}
	}
	return true
}

func Install(target Agent) InstallResult {
	return InstallResult{
		Agent: target,
		Steps: []StepResult{
			installGuidelines(target),
			installSkills(target),
			installMCP(target),
		},
	}
}

func installGuidelines(target Agent) StepResult {
	dest := resolveGuidelinesTarget(target)
	files, err := markdownFiles(guidelinesSourceDir)
	if err != nil {
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: err}
	}
	if len(files) == 0 {
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusSkipped}
	}

	var parts []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: err}
		}
		parts = append(parts, string(data))
	}

	content := strings.Join(parts, "\n")
	if err := writeFileWithParents(dest, []byte(content)); err != nil {
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: err}
	}

	return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusOK}
}

func installSkills(target Agent) StepResult {
	if err := os.MkdirAll(target.SkillsDir, 0o755); err != nil {
		return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusError, Err: err}
	}

	entries, err := os.ReadDir(skillsSourceDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusOK}
		}
		return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusError, Err: err}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		src := filepath.Join(skillsSourceDir, entry.Name())
		dst := filepath.Join(target.SkillsDir, entry.Name())
		if err := copyDir(src, dst); err != nil {
			return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusError, Err: err}
		}
	}

	return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusOK}
}

func installMCP(target Agent) StepResult {
	_, err := os.Stat(mcpSourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusSkipped}
		}
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusError, Err: err}
	}

	cfg, err := config.ReadMCP(mcpSourcePath)
	if err != nil {
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusError, Err: err}
	}

	data, err := config.RenderMCP(cfg, string(target.MCPFormat))
	if err != nil {
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusError, Err: err}
	}

	if err := writeFileWithParents(target.MCPConfig, data); err != nil {
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusError, Err: err}
	}

	return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusOK}
}

func resolveGuidelinesTarget(target Agent) string {
	if target.ID == "cursor" {
		return cursorGuidelinesOut
	}
	return target.GuidelinesFile
}

func markdownFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	slices.Sort(files)
	return files, nil
}

func writeFileWithParents(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := out.Chmod(info.Mode()); err != nil {
		return err
	}

	return out.Close()
}
