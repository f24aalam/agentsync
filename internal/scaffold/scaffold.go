package scaffold

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/f24aalam/agentsync/internal/agent"
)

const (
	aiRootDir          = ".ai"
	guidelinesDir      = ".ai/guidelines"
	skillsDir          = ".ai/skills"
	guidelinesFilePath = ".ai/guidelines/core.md"
	sampleSkillPath    = ".ai/skills/example-skill/SKILL.md"
	mcpConfigPath      = ".ai/mcp.toml"
)

func EnsureDirs() error {
	dirs := []string{
		aiRootDir,
		guidelinesDir,
		skillsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return nil
}

func CreateGuidelines(projectName string) (string, error) {
	content := fmt.Sprintf(`# %s Guidelines

## Code Style
...

## Architecture
...

## Conventions
...
`, projectName)

	if err := os.WriteFile(guidelinesFilePath, []byte(content), 0o644); err != nil {
		return "", err
	}

	return guidelinesFilePath, nil
}

func CreateSampleSkill() (string, error) {
	if err := os.MkdirAll(filepath.Dir(sampleSkillPath), 0o755); err != nil {
		return "", err
	}

	content := `---
name: example-skill
description: Sample skill scaffolded by agentsync.
---

# Example Skill

## Purpose
Describe the job this skill is responsible for.

## When To Use
Use this skill when a task consistently benefits from repeatable instructions.

## Instructions
1. Gather the local context first.
2. Apply the project-specific workflow.
3. Report the result clearly.
`

	if err := os.WriteFile(sampleSkillPath, []byte(content), 0o644); err != nil {
		return "", err
	}

	return sampleSkillPath, nil
}

func CreateMCPConfig() (string, error) {
	content := `# Example MCP servers.
# Uncomment and adjust the entries below to match your environment.
#
# [servers.postgres]
# command = "npx"
# args = ["-y", "@modelcontextprotocol/server-postgres"]
#
# [servers.postgres.env]
# DATABASE_URL = "${DATABASE_URL}"
#
# [servers.filesystem]
# command = "npx"
# args = ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/project"]
`

	if err := os.WriteFile(mcpConfigPath, []byte(content), 0o644); err != nil {
		return "", err
	}

	return mcpConfigPath, nil
}

func UpdateGitignore(path string, agents []agent.Agent) (bool, error) {
	entries := ignoreEntries(agents)
	if len(entries) == 0 {
		return false, nil
	}

	var existing []byte
	if data, err := os.ReadFile(path); err == nil {
		existing = data
	} else if !os.IsNotExist(err) {
		return false, err
	}

	existingText := string(existing)
	toAppend := make([]string, 0, len(entries))
	for _, entry := range entries {
		if ignoreEntryExists(existingText, entry) {
			continue
		}
		toAppend = append(toAppend, entry)
	}

	if len(toAppend) == 0 {
		return false, nil
	}

	var buf bytes.Buffer
	buf.Write(existing)
	if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n")) {
		buf.WriteByte('\n')
	}
	for _, entry := range toAppend {
		buf.WriteString(entry)
		buf.WriteByte('\n')
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return false, err
	}

	return true, nil
}

func ignoreEntries(agents []agent.Agent) []string {
	entries := make([]string, 0, len(agents)*3)
	for _, target := range agents {
		entries = append(entries, target.GuidelinesFile, target.SkillsDir, target.MCPConfig)
	}

	seen := make(map[string]struct{}, len(entries))
	unique := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, ok := seen[entry]; ok {
			continue
		}
		seen[entry] = struct{}{}
		unique = append(unique, entry)
	}

	slices.Sort(unique)
	return unique
}

func ignoreEntryExists(existing, entry string) bool {
	for _, line := range strings.Split(existing, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}
