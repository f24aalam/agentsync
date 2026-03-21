package agent

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
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

// InstallAgent runs guidelines + MCP for one agent; skillsStep is produced by the runner (shared dirs).
func InstallAgent(target Agent, root string, skillsStep StepResult, plan InstallPlan) InstallResult {
	if root == "" {
		root = "."
	}
	skipG := plan.SkipGuidelines[target.ID]
	skipM := plan.SkipMCP[target.ID]
	return InstallResult{
		Agent: target,
		Steps: []StepResult{
			installGuidelines(target, root, skipG),
			skillsStep,
			installMCP(target, root, skipM),
		},
	}
}

func installGuidelines(target Agent, root string, skip bool) StepResult {
	dest := resolveGuidelinesTarget(target)
	destPath := filepath.Join(root, dest)
	if skip {
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusSkipped}
	}

	files, err := markdownFiles(filepath.Join(root, guidelinesSourceDir))
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
	if err := writeFileWithParents(destPath, []byte(content)); err != nil {
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: err}
	}

	return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusOK}
}

func installSkills(target Agent, root string) StepResult {
	destDir := filepath.Join(root, target.SkillsDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusError, Err: err}
	}

	entries, err := os.ReadDir(filepath.Join(root, skillsSourceDir))
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

		src := filepath.Join(root, skillsSourceDir, entry.Name())
		dst := filepath.Join(destDir, entry.Name())

		if err := copyDir(src, dst); err != nil {
			return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusError, Err: err}
		}
	}

	return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusOK}
}

// copySkillTreesFromAI copies each child directory under root/.ai/skills into root/destSkillsDir.
func copySkillTreesFromAI(root, destSkillsDir string) error {
	srcRoot := filepath.Join(root, skillsSourceDir)
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	destRoot := filepath.Join(root, destSkillsDir)
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		src := filepath.Join(srcRoot, entry.Name())
		dst := filepath.Join(destRoot, entry.Name())
		if err := copyDir(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func installMCP(target Agent, root string, skip bool) StepResult {
	if skip {
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusSkipped}
	}

	mcpPath := filepath.Join(root, mcpSourcePath)
	_, err := os.Stat(mcpPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusSkipped}
		}

		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusError, Err: err}
	}

	cfg, err := config.ReadMCP(mcpPath)
	if err != nil {
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusError, Err: err}
	}

	destPaths, err := resolveMCPDestPaths(target)
	if err != nil {
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusError, Err: err}
	}

	var firstErr error
	for _, dest := range destPaths {
		// Only show the first error in StepResult; still attempt others.
		if firstErr != nil {
			continue
		}

		if strings.TrimSpace(dest) == "" {
			continue
		}

		destWrite := dest
		if !filepath.IsAbs(destWrite) {
			destWrite = filepath.Join(root, destWrite)
		}

		// For now, merge/preserve is implemented for JSON configs.
		// TOML merge preserves only `[mcp_servers]` entries.
		if target.MCPFormat == MCPFormatTOML {
			if err := mergeOrWriteTomlMCP(destWrite, target, cfg); err != nil {
				firstErr = err
			}
			continue
		}

		if err := mergeOrWriteJSONMCP(destWrite, target, cfg); err != nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusError, Err: firstErr}
	}

	return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusOK}
}

func resolveMCPDestPaths(target Agent) ([]string, error) {
	paths := make([]string, 0, 1+len(target.MCPConfigs))
	if strings.TrimSpace(target.MCPConfig) != "" {
		paths = append(paths, target.MCPConfig)
	}
	paths = append(paths, target.MCPConfigs...)

	// Expand ~ in CLI configs.
	for i := range paths {
		p := strings.TrimSpace(paths[i])
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			paths[i] = filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}

	return paths, nil
}

func mergeOrWriteJSONMCP(dest string, target Agent, cfg config.MCPConfig) error {
	// Build managed server entries in the per-agent schema shape.
	managed := make(map[string]any, len(cfg.Servers))
	for name, server := range cfg.Servers {
		managed[name] = renderManagedServerJSON(target, name, server)
	}

	rootKey, renderRoot := mcpJSONRoot(target)

	// Attempt to merge into existing file.
	existing, err := os.ReadFile(dest)
	if err == nil {
		var root map[string]any
		if err := json.Unmarshal(existing, &root); err != nil {
			// If existing JSON can't be parsed, fall back to overwrite.
			root = renderRoot()
		}

		existingServers, ok := root[rootKey].(map[string]any)
		if !ok || existingServers == nil {
			existingServers = map[string]any{}
		}

		for name, v := range managed {
			existingServers[name] = v
		}

		root[rootKey] = existingServers

		data, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return err
		}
		return writeFileWithParents(dest, data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// File doesn't exist: write a fresh root.
	root := renderRoot()
	root[rootKey] = managed
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}

	return writeFileWithParents(dest, data)
}

func renderManagedServerJSON(target Agent, _ string, server config.MCPServer) any {
	// Only local/stdio fields are mapped from canonical MCP right now.
	// Remote/http type fields are handled in a later iteration.
	switch target.ID {
	case "opencode":
		typ := server.Type
		if strings.TrimSpace(typ) == "" {
			typ = "local"
		}

		if typ == "remote" {
			out := map[string]any{
				"type":    "remote",
				"url":     server.URL,
				"enabled": true,
			}
			if len(server.Headers) > 0 {
				out["headers"] = server.Headers
			}
			if server.OAuth != nil {
				out["oauth"] = map[string]any{
					"clientId":     server.OAuth.ClientID,
					"clientSecret": server.OAuth.ClientSecret,
					"scope":        server.OAuth.Scope,
				}
			}
			return out
		}

		command := make([]string, 0, 1+len(server.Args))
		if server.Command != "" {
			command = append(command, server.Command)
		}
		command = append(command, server.Args...)

		out := map[string]any{
			"type":    "local",
			"command": command,
			"enabled": true,
		}
		if len(server.Env) > 0 {
			out["environment"] = server.Env
		}
		return out

	case "github-copilot":
		out := map[string]any{
			"type":    "stdio",
			"command": server.Command,
		}
		if len(server.Args) > 0 {
			out["args"] = server.Args
		}
		if len(server.Env) > 0 {
			out["env"] = server.Env
		}
		return out

	default:
		out := map[string]any{
			"command": server.Command,
		}
		if len(server.Args) > 0 {
			out["args"] = server.Args
		}
		if len(server.Env) > 0 {
			out["env"] = server.Env
		}
		return out
	}
}

func mcpJSONRoot(target Agent) (rootKey string, renderRoot func() map[string]any) {
	switch target.ID {
	case "opencode":
		return "mcp", func() map[string]any {
			return map[string]any{
				"$schema": "https://opencode.ai/config.json",
				"mcp":     map[string]any{},
			}
		}
	case "github-copilot":
		return "servers", func() map[string]any {
			return map[string]any{"servers": map[string]any{}}
		}
	default:
		return "mcpServers", func() map[string]any {
			return map[string]any{
				"mcpServers": map[string]any{},
			}
		}
	}
}

func mergeOrWriteTomlMCP(dest string, target Agent, cfg config.MCPConfig) error {
	// Merge only `[mcp_servers]` entries; other TOML keys are not preserved.
	var payload struct {
		MCPServers map[string]config.MCPServer `toml:"mcp_servers"`
	}

	existing, err := os.ReadFile(dest)
	if err == nil {
		_ = toml.Unmarshal(existing, &payload)
	}

	if payload.MCPServers == nil {
		payload.MCPServers = map[string]config.MCPServer{}
	}

	for name, server := range cfg.Servers {
		payload.MCPServers[name] = server
	}

	data, err := config.RenderMCP(config.MCPConfig{Servers: payload.MCPServers}, "toml")
	if err != nil {
		return err
	}
	return writeFileWithParents(dest, data)
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
