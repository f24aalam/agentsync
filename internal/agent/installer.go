package agent

import (
	"encoding/json"
	"errors"
	"fmt"
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

	ManagedSectionStart = "<!-- BEGIN AGENTSYNC MANAGED -->"
	ManagedSectionEnd   = "<!-- END AGENTSYNC MANAGED -->"
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
	dest := resolveGuidelinesTarget(target)
	skipG := plan.SkipGuidelines[target.ID] || plan.SkipGuidelinesDest[dest]
	skipM := plan.SkipMCP[target.ID]
	return InstallResult{
		Agent: target,
		Steps: []StepResult{
			installGuidelinesWithLock(target, root, skipG, plan.Lock),
			skillsStep,
			installMCP(target, root, skipM),
		},
	}
}

func installGuidelines(target Agent, root string, skip bool) StepResult {
	return installGuidelinesWithLock(target, root, skip, nil)
}

func installGuidelinesWithLock(target Agent, root string, skip bool, lock *config.LockFile) StepResult {
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
		if lock != nil && lock.Guidelines != nil {
			if stored, ok := lock.Guidelines[dest]; ok && stored.Hash != "" {
				if existingData, err := os.ReadFile(destPath); err == nil {
					currentManaged, hasMarkers := ExtractManagedSection(string(existingData))
					if hasMarkers && config.HashString(currentManaged) == stored.Hash {
						cleaned := removeManagedSection(string(existingData))
						delete(lock.Guidelines, dest)
						if strings.TrimSpace(cleaned) == "" {
							_ = os.Remove(destPath)
						} else {
							_ = writeFileWithParents(destPath, []byte(cleaned))
						}
					} else if hasMarkers {
						return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: errors.New("managed guidelines modified by user, cannot safely remove")}
					}
				}
			}
		}
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusSkipped}
	}

	var parts []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: err}
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	if len(parts) == 0 {
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusSkipped}
	}

	mergedContent := strings.Join(parts, "\n\n")

	var existingContent string
	existingData, err := os.ReadFile(destPath)
	if err == nil {
		existingContent = string(existingData)
	} else if !errors.Is(err, os.ErrNotExist) {
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: err}
	}

	finalContent, err := injectManagedGuidelines(existingContent, mergedContent)
	if err != nil {
		return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: err}
	}

	if existingContent != finalContent {
		if err := writeFileWithParents(destPath, []byte(finalContent)); err != nil {
			return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusError, Err: err}
		}
	}

	if lock != nil {
		if lock.Guidelines == nil {
			lock.Guidelines = make(map[string]config.ResourceLock)
		}
		if inner, ok := ExtractManagedSection(finalContent); ok {
			lock.Guidelines[dest] = config.ResourceLock{Hash: config.HashString(inner)}
		}
	}

	return StepResult{Name: "Guidelines", Target: dest, Status: StepStatusOK}
}

func detectLineEnding(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func normalizeLineEndings(s, eol string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if eol == "\n" {
		return s
	}
	return strings.ReplaceAll(s, "\n", eol)
}

func ExtractManagedSection(content string) (string, bool) {
	startIdx := strings.Index(content, ManagedSectionStart)
	endIdx := strings.Index(content, ManagedSectionEnd)
	if startIdx == -1 || endIdx == -1 || endIdx < startIdx {
		return "", false
	}
	inner := content[startIdx+len(ManagedSectionStart) : endIdx]
	return strings.TrimSpace(inner), true
}

func removeManagedSection(content string) string {
	startIdx := strings.Index(content, ManagedSectionStart)
	endIdx := strings.Index(content, ManagedSectionEnd)
	if startIdx == -1 || endIdx == -1 || endIdx < startIdx {
		return content
	}
	before := content[:startIdx]
	afterStart := endIdx + len(ManagedSectionEnd)
	if afterStart+1 < len(content) && content[afterStart:afterStart+2] == "\r\n" {
		afterStart += 2
	} else if afterStart < len(content) && content[afterStart] == '\n' {
		afterStart++
	}
	after := content[afterStart:]
	return strings.TrimRight(before, "\r\n") + after
}

func injectManagedGuidelines(existingContent, managedContent string) (string, error) {
	eol := detectLineEnding(existingContent)
	if eol == "\n" && strings.Contains(managedContent, "\r\n") {
		eol = "\r\n"
	}
	normalizedManaged := normalizeLineEndings(managedContent, eol)
	managedBlock := ManagedSectionStart + eol + normalizedManaged + eol + ManagedSectionEnd

	countBegin := strings.Count(existingContent, ManagedSectionStart)
	countEnd := strings.Count(existingContent, ManagedSectionEnd)

	// Detect malformed markers:
	if countBegin > 1 || countEnd > 1 {
		return "", fmt.Errorf("malformed guidelines: duplicate managed markers found (begin=%d, end=%d)", countBegin, countEnd)
	}
	if (countBegin == 1 && countEnd == 0) || (countBegin == 0 && countEnd == 1) {
		return "", fmt.Errorf("malformed guidelines: missing counterpart managed marker (begin=%d, end=%d)", countBegin, countEnd)
	}

	if countBegin == 1 && countEnd == 1 {
		startIdx := strings.Index(existingContent, ManagedSectionStart)
		endIdx := strings.Index(existingContent, ManagedSectionEnd)

		if endIdx < startIdx {
			return "", fmt.Errorf("malformed guidelines: end marker appears before begin marker")
		}

		// Exact byte preservation of user content outside markers.
		before := existingContent[:startIdx]
		afterStart := endIdx + len(ManagedSectionEnd)

		markerHadNewline := false
		if afterStart+1 < len(existingContent) && existingContent[afterStart:afterStart+2] == "\r\n" {
			afterStart += 2
			markerHadNewline = true
		} else if afterStart < len(existingContent) && existingContent[afterStart] == '\n' {
			afterStart++
			markerHadNewline = true
		}

		after := existingContent[afterStart:]

		var b strings.Builder
		b.WriteString(before)
		b.WriteString(managedBlock)
		if markerHadNewline || len(after) > 0 {
			b.WriteString(eol)
		}
		b.WriteString(after)
		return b.String(), nil
	}

	// No markers present:
	if len(existingContent) == 0 {
		return managedBlock + eol, nil
	}

	// Adoption of existing file without markers:
	var b strings.Builder
	b.WriteString(existingContent)
	if !strings.HasSuffix(existingContent, "\n") {
		b.WriteString(eol)
	}
	if !strings.HasSuffix(existingContent, "\n\n") && !strings.HasSuffix(existingContent, "\r\n\r\n") {
		b.WriteString(eol)
	}
	b.WriteString(managedBlock)
	b.WriteString(eol)
	return b.String(), nil
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
			return fmt.Errorf("cannot parse existing JSON config at %s: %w", dest, err)
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
	existing, err := os.ReadFile(dest)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		rendered := renderTomlMCPServers(cfg.Servers)
		if !strings.HasSuffix(rendered, "\n") {
			rendered += "\n"
		}
		return writeFileWithParents(dest, []byte(rendered))
	}

	updated, err := updateTomlMCPServers(string(existing), cfg.Servers)
	if err != nil {
		return fmt.Errorf("cannot parse existing TOML config at %s: %w", dest, err)
	}

	return writeFileWithParents(dest, []byte(updated))
}

func updateTomlMCPServers(existingContent string, servers map[string]config.MCPServer) (string, error) {
	// 1. Validate that the existing content is syntactically valid TOML
	var check map[string]any
	if _, err := toml.Decode(existingContent, &check); err != nil {
		return "", fmt.Errorf("cannot parse existing TOML config: %w", err)
	}

	// 2. Parse existing content into structural sections
	rawLines := strings.Split(strings.ReplaceAll(existingContent, "\r\n", "\n"), "\n")
	type tomlSection struct {
		header string
		lines  []string
	}

	var sections []tomlSection
	current := tomlSection{header: ""}

	for _, line := range rawLines {
		if header, isTable := parseTOMLTableHeader(line); isTable {
			// Find trailing comments and blank lines in current.lines that precede this header
			splitIdx := len(current.lines)
			for splitIdx > 0 {
				prevLine := strings.TrimSpace(current.lines[splitIdx-1])
				if strings.HasPrefix(prevLine, "#") || prevLine == "" {
					splitIdx--
				} else {
					break
				}
			}

			// If current is section 0 and splitIdx == 0, keep all comments at the top of the file in section 0
			// so they are never lost even if the first table is a managed table.
			if current.header == "" && splitIdx == 0 {
				sections = append(sections, current)
				current = tomlSection{header: header, lines: []string{line}}
			} else {
				leadingForNext := current.lines[splitIdx:]
				current.lines = current.lines[:splitIdx]
				sections = append(sections, current)

				newLines := make([]string, 0, len(leadingForNext)+1)
				newLines = append(newLines, leadingForNext...)
				newLines = append(newLines, line)
				current = tomlSection{header: header, lines: newLines}
			}
		} else {
			current.lines = append(current.lines, line)
		}
	}
	sections = append(sections, current)

	// 3. Render the managed servers
	renderedManaged := strings.TrimRight(renderTomlMCPServers(servers), "\r\n")

	// 4. Identify which sections are managed by AgentSync
	firstManagedIdx := -1
	for idx, sec := range sections {
		srvName := mcpServerNameForTable(sec.header)
		if srvName != "" {
			if _, isManaged := servers[srvName]; isManaged {
				if firstManagedIdx == -1 {
					firstManagedIdx = idx
				}
			}
		} else if isRootMCPHeader(sec.header) {
			if firstManagedIdx == -1 {
				firstManagedIdx = idx
			}
		}
	}

	// 5. Reconstruct document lines
	var outLines []string
	inserted := false

	for idx, sec := range sections {
		srvName := mcpServerNameForTable(sec.header)
		isManaged := false
		if srvName != "" {
			_, isManaged = servers[srvName]
		} else if isRootMCPHeader(sec.header) {
			isManaged = true
		}

		if isManaged {
			if idx == firstManagedIdx && !inserted {
				if len(renderedManaged) > 0 {
					if len(outLines) > 0 && strings.TrimSpace(outLines[len(outLines)-1]) != "" {
						outLines = append(outLines, "")
					}
					outLines = append(outLines, strings.Split(renderedManaged, "\n")...)
				}
				inserted = true
			}
			continue
		}

		outLines = append(outLines, sec.lines...)
	}

	// If no previous managed section was found in the file, append the new servers
	if !inserted && len(renderedManaged) > 0 {
		for len(outLines) > 0 && strings.TrimSpace(outLines[len(outLines)-1]) == "" {
			outLines = outLines[:len(outLines)-1]
		}
		if len(outLines) > 0 {
			outLines = append(outLines, "")
		}
		outLines = append(outLines, strings.Split(renderedManaged, "\n")...)
	}

	result := strings.Join(outLines, "\n")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	// 6. Validate the final result with toml.Decode
	var finalCheck map[string]any
	if _, err := toml.Decode(result, &finalCheck); err != nil {
		return "", fmt.Errorf("failed to generate valid TOML: %w", err)
	}

	return result, nil
}

func parseTOMLTableHeader(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	stripped := stripInlineComment(trimmed)
	stripped = strings.TrimSpace(stripped)

	if strings.HasPrefix(stripped, "[") && strings.HasSuffix(stripped, "]") &&
		!strings.HasPrefix(stripped, "[[") && !strings.HasSuffix(stripped, "]]") {
		inner := strings.TrimSpace(stripped[1 : len(stripped)-1])
		if inner != "" {
			return inner, true
		}
	}
	return "", false
}

func stripInlineComment(s string) string {
	inQuote := false
	var quoteChar rune
	for i, r := range s {
		if inQuote {
			if r == quoteChar {
				inQuote = false
			}
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
			} else if r == '#' {
				return s[:i]
			}
		}
	}
	return s
}

func normalizeTOMLTableName(name string) string {
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "'", "")
	return strings.TrimSpace(name)
}

func isRootMCPHeader(header string) bool {
	norm := normalizeTOMLTableName(header)
	return norm == "mcp_servers"
}

func mcpServerNameForTable(tableName string) string {
	norm := normalizeTOMLTableName(tableName)
	if norm == "mcp_servers" {
		return ""
	}
	if strings.HasPrefix(norm, "mcp_servers.") {
		rest := strings.TrimPrefix(norm, "mcp_servers.")
		parts := strings.Split(rest, ".")
		return parts[0]
	}
	return ""
}

func renderTomlMCPServers(servers map[string]config.MCPServer) string {
	if len(servers) == 0 {
		return ""
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	slices.Sort(names)

	var buf strings.Builder
	for i, name := range names {
		server := servers[name]
		if i > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(fmt.Sprintf("[mcp_servers.%s]\n", name))
		buf.WriteString(fmt.Sprintf("command = %q\n", server.Command))
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
			buf.WriteString(fmt.Sprintf("\n[mcp_servers.%s.env]\n", name))
			keys := make([]string, 0, len(server.Env))
			for key := range server.Env {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				buf.WriteString(fmt.Sprintf("%s = %q\n", key, server.Env[key]))
			}
		}
	}
	return buf.String()
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
