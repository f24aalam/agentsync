package scaffold

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/config"
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

func ImportGuidelineToCore(sourceLabel string, content string) (string, error) {
	if err := os.MkdirAll(guidelinesDir, 0o755); err != nil {
		return "", err
	}

	header := fmt.Sprintf("## Imported: %s\n\n", sourceLabel)
	block := header + strings.TrimSpace(content) + "\n\n"

	var buf bytes.Buffer
	if existing, err := os.ReadFile(guidelinesFilePath); err == nil {
		buf.Write(existing)
		if len(existing) > 0 && !bytes.HasSuffix(existing, []byte("\n\n")) {
			if bytes.HasSuffix(existing, []byte("\n")) {
				buf.WriteByte('\n')
			} else {
				buf.WriteString("\n\n")
			}
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	buf.WriteString(block)
	if err := os.WriteFile(guidelinesFilePath, buf.Bytes(), 0o644); err != nil {
		return "", err
	}

	return guidelinesFilePath, nil
}

func ImportSkill(name string, sourceDir string) (string, error) {
	targetDir := filepath.Join(skillsDir, name)
	if err := copyDir(sourceDir, targetDir); err != nil {
		return "", err
	}
	return targetDir + string(os.PathSeparator), nil
}

func ImportMCPServers(servers map[string]config.MCPServer) (string, error) {
	if len(servers) == 0 {
		return "", nil
	}

	var buf bytes.Buffer
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		server := servers[name]
		if i > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(fmt.Sprintf("[servers.%s]\n", name))

		typ := server.Type
		if strings.TrimSpace(typ) == "" {
			typ = "local"
		}
		buf.WriteString(fmt.Sprintf("type = %q\n", typ))

		if typ == "local" {
			if strings.TrimSpace(server.Command) != "" {
				buf.WriteString(fmt.Sprintf("command = %q\n", server.Command))
			}
			if len(server.Args) > 0 {
				buf.WriteString("args = [")
				for idx, arg := range server.Args {
					if idx > 0 {
						buf.WriteString(", ")
					}
					buf.WriteString(fmt.Sprintf("%q", arg))
				}
				buf.WriteString("]\n")
			}
			if len(server.Env) > 0 {
				buf.WriteString(fmt.Sprintf("\n[servers.%s.env]\n", name))
				keys := make([]string, 0, len(server.Env))
				for key := range server.Env {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					buf.WriteString(fmt.Sprintf("%s = %q\n", key, server.Env[key]))
				}
			}
		} else {
			if strings.TrimSpace(server.URL) != "" {
				buf.WriteString(fmt.Sprintf("url = %q\n", server.URL))
			}
			if len(server.Headers) > 0 {
				buf.WriteString(fmt.Sprintf("\n[servers.%s.headers]\n", name))
				keys := make([]string, 0, len(server.Headers))
				for key := range server.Headers {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					buf.WriteString(fmt.Sprintf("%s = %q\n", key, server.Headers[key]))
				}
			}
			if server.OAuth != nil {
				buf.WriteString(fmt.Sprintf("\n[servers.%s.oauth]\n", name))
				if strings.TrimSpace(server.OAuth.ClientID) != "" {
					buf.WriteString(fmt.Sprintf("clientId = %q\n", server.OAuth.ClientID))
				}
				if strings.TrimSpace(server.OAuth.ClientSecret) != "" {
					buf.WriteString(fmt.Sprintf("clientSecret = %q\n", server.OAuth.ClientSecret))
				}
				if strings.TrimSpace(server.OAuth.Scope) != "" {
					buf.WriteString(fmt.Sprintf("scope = %q\n", server.OAuth.Scope))
				}
			}
		}
	}

	if err := os.WriteFile(mcpConfigPath, buf.Bytes(), 0o644); err != nil {
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
