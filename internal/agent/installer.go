package agent

import (
	"bytes"
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

var (
	osRename    = os.Rename
	osRemoveAll = os.RemoveAll
	osWriteFile = os.WriteFile
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

	if (target.ID == "cursor" || strings.HasSuffix(dest, ".mdc")) && !strings.HasPrefix(strings.TrimSpace(existingContent), "---") {
		frontmatter := "---\ndescription: Project Guidelines\nglobs: *\nalwaysApply: true\n---\n\n"
		if existingContent != "" {
			existingContent = frontmatter + existingContent
		} else {
			existingContent = frontmatter
		}
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
	if err := syncSkillTrees(root, target.SkillsDir, nil); err != nil {
		return StepResult{Name: "Skills", Target: destDir, Status: StepStatusError, Err: err}
	}
	return StepResult{Name: "Skills", Target: target.SkillsDir, Status: StepStatusOK}
}

// copySkillTreesFromAI copies each child directory under root/.ai/skills into root/destSkillsDir.
func copySkillTreesFromAI(root, destSkillsDir string) error {
	return syncSkillTrees(root, destSkillsDir, nil)
}

// compareSkillPackages compares the contents of a canonical skill package (srcDir)
// and an existing destination skill package (dstDir).
// It returns (exists bool, differs bool, err error).
// If dstDir does not exist, exists=false, differs=false.
// If dstDir exists, differs=true if any file is missing, extra, or has different contents/symlinks.
func compareSkillPackages(srcDir, dstDir string) (bool, bool, error) {
	st, err := os.Lstat(dstDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	if !st.IsDir() {
		return true, true, nil
	}

	srcFiles := make(map[string]os.FileInfo)
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		srcFiles[rel] = info
		return nil
	})
	if err != nil {
		return true, false, err
	}

	dstFiles := make(map[string]os.FileInfo)
	err = filepath.Walk(dstDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dstDir, path)
		if err != nil {
			return err
		}
		dstFiles[rel] = info
		return nil
	})
	if err != nil {
		return true, false, err
	}

	if len(srcFiles) != len(dstFiles) {
		return true, true, nil
	}

	for rel, srcInfo := range srcFiles {
		dstInfo, ok := dstFiles[rel]
		if !ok {
			return true, true, nil
		}

		srcPath := filepath.Join(srcDir, rel)
		dstPath := filepath.Join(dstDir, rel)

		srcIsSymlink := srcInfo.Mode()&os.ModeSymlink != 0
		dstIsSymlink := dstInfo.Mode()&os.ModeSymlink != 0

		if srcIsSymlink != dstIsSymlink {
			return true, true, nil
		}

		if srcIsSymlink {
			srcTarget, err := os.Readlink(srcPath)
			if err != nil {
				return true, true, nil
			}
			dstTarget, err := os.Readlink(dstPath)
			if err != nil {
				return true, true, nil
			}
			if srcTarget != dstTarget {
				return true, true, nil
			}
			continue
		}

		if srcInfo.Size() != dstInfo.Size() {
			return true, true, nil
		}

		srcData, err := os.ReadFile(srcPath)
		if err != nil {
			return true, false, err
		}
		dstData, err := os.ReadFile(dstPath)
		if err != nil {
			return true, false, err
		}

		if !bytes.Equal(srcData, dstData) {
			return true, true, nil
		}
	}

	return true, false, nil
}

func syncSkillTrees(root, destSkillsDir string, lock *config.LockFile) error {
	plan := NewInstallPlan()
	plan.Lock = lock
	return syncSkillTreesWithPlan(root, destSkillsDir, plan)
}

func syncSkillTreesWithPlan(root, destSkillsDir string, plan InstallPlan) error {
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

	dirKey := SkillDirKey(destSkillsDir)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		pkgKey := dirKey + "/" + skillName

		// If skipped in plan (user declined, or --yes on unknown package), leave complete package untouched!
		if plan.SkipSkillsDir[dirKey] || (plan.SkipSkillPackages != nil && plan.SkipSkillPackages[pkgKey]) {
			continue
		}

		srcDir := filepath.Join(srcRoot, skillName)
		dstDir := filepath.Join(destRoot, skillName)

		exists, differs, err := compareSkillPackages(srcDir, dstDir)
		if err != nil {
			return err
		}
		if exists && !differs {
			// Complete package matches canonical package! Skip writing to preserve mtime.
			// Matching contents do not prove ownership; do not adopt ownership merely because contents match.
			continue
		}

		if err := copySkillPackage(srcDir, dstDir, plan.Lock, dirKey, skillName, root); err != nil {
			return err
		}
	}

	return nil
}

func copySkillPackage(srcDir, dstDir string, lock *config.LockFile, dirKey, skillName, root string) error {
	// 1. Validate the canonical skill package before destructive operations
	if err := validateSkillPackage(srcDir, skillName); err != nil {
		return err
	}

	destParent := filepath.Dir(dstDir)
	if err := os.MkdirAll(destParent, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory for skill package %q: %w", skillName, err)
	}

	// 2. Prepare replacement content in a temporary directory alongside the destination
	tmpDir, err := os.MkdirTemp(destParent, "."+skillName+"-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary staging directory for skill package %q: %w", skillName, err)
	}
	defer func() {
		_ = osRemoveAll(tmpDir)
	}()

	// 3. Ensure copied files and symlinks cannot escape the package
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(tmpDir, rel)

		if info.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		return CopyFileWithinPackage(path, targetPath, srcDir)
	})
	if err != nil {
		return fmt.Errorf("failed to copy files for skill package %q: %w", skillName, err)
	}

	// 4. Replace only the approved destination skill package
	if _, err := os.Lstat(dstDir); err == nil {
		bakDir, err := os.MkdirTemp(destParent, "."+skillName+"-bak-*")
		if err != nil {
			return fmt.Errorf("failed to create backup staging for skill package %q: %w", skillName, err)
		}
		if err := os.Remove(bakDir); err != nil {
			_ = osRemoveAll(bakDir)
			return fmt.Errorf("failed to prepare backup staging for skill package %q: %w", skillName, err)
		}

		if err := osRename(dstDir, bakDir); err != nil {
			return fmt.Errorf("failed to stage existing skill package %q: %w", skillName, err)
		}

		if err := osRename(tmpDir, dstDir); err != nil {
			rollbackErr := osRename(bakDir, dstDir)
			if rollbackErr != nil {
				return errors.Join(
					fmt.Errorf("failed to install replacement for skill package %q: %w", skillName, err),
					fmt.Errorf("rollback to restore original package from backup %s failed: %w", bakDir, rollbackErr),
				)
			}
			return fmt.Errorf("failed to install replacement for skill package %q: %w (original package restored)", skillName, err)
		}

		// 5. Record ownership only after successful replacement
		if lock != nil {
			lock.RecordSkillPackage(dirKey, skillName)
		}

		if err := osRemoveAll(bakDir); err != nil {
			return fmt.Errorf("skill package %q installed successfully, but failed to remove temporary backup at %s: %w", skillName, bakDir, err)
		}
	} else {
		if err := osRename(tmpDir, dstDir); err != nil {
			return fmt.Errorf("failed to install skill package %q: %w", skillName, err)
		}

		// 5. Record ownership only after successful replacement
		if lock != nil {
			lock.RecordSkillPackage(dirKey, skillName)
		}
	}

	return nil
}

func validateSkillPackage(srcDir, skillName string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := validateSymlinkWithinPackage(path, srcDir); err != nil {
				return fmt.Errorf("unsafe symlink in skill %q: %w", skillName, err)
			}
		}
		return nil
	})
}

func installMCP(target Agent, root string, skip bool) StepResult {
	return installMCPWithLock(target, root, skip, nil)
}

func installMCPWithLock(target Agent, root string, skip bool, lock *config.LockFile) StepResult {
	plan := NewInstallPlan()
	plan.Lock = lock
	plan.SkipMCP[target.ID] = skip
	return installMCPWithPlan(target, root, plan)
}

func installMCPWithPlan(target Agent, root string, plan InstallPlan) StepResult {
	if plan.SkipMCP[target.ID] {
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
	allSkipped := true
	for _, dest := range destPaths {
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
		normKey, _ := filepath.Rel(root, destWrite)
		normKey = filepath.Clean(normKey)

		if plan.SkipMCPDest != nil && (plan.SkipMCPDest[normKey] || plan.SkipMCPDest[filepath.Clean(dest)]) {
			continue
		}
		allSkipped = false

		if target.MCPFormat == MCPFormatTOML {
			if err := mergeOrWriteTomlMCP(destWrite, target, cfg, plan, normKey); err != nil {
				firstErr = err
			}
			continue
		}

		if err := mergeOrWriteJSONMCP(destWrite, target, cfg, plan, normKey); err != nil {
			firstErr = err
		}
	}

	if allSkipped && len(destPaths) > 0 {
		return StepResult{Name: "MCP", Target: target.MCPConfig, Status: StepStatusSkipped}
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

func mergeOrWriteJSONMCP(dest string, target Agent, cfg config.MCPConfig, plan InstallPlan, normKey string) error {
	// Build managed server entries in the per-agent schema shape.
	managed := make(map[string]any, len(cfg.Servers))
	for name, server := range cfg.Servers {
		if plan.IsMCPServerSkipped(normKey, name) {
			continue
		}
		rendered := renderManagedServerJSON(target, name, server)
		if rendered != nil {
			managed[name] = rendered
		}
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

		// Update or insert managed servers
		for name, v := range managed {
			existingServers[name] = v
			if plan.Lock != nil {
				plan.Lock.RecordMCP(normKey, name)
			}
		}

		root[rootKey] = existingServers

		data, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')

		if bytes.Equal(existing, data) {
			return nil
		}

		return writeFileWithParents(dest, data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// File doesn't exist: write a fresh root.
	rootMap := renderRoot()
	rootMap[rootKey] = managed
	data, err := json.MarshalIndent(rootMap, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if plan.Lock != nil {
		for name := range managed {
			plan.Lock.RecordMCP(normKey, name)
		}
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
		if strings.TrimSpace(server.Command) == "" {
			return nil
		}
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
		if strings.TrimSpace(server.Command) == "" {
			return nil
		}
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

func mergeOrWriteTomlMCP(dest string, target Agent, cfg config.MCPConfig, plan InstallPlan, normKey string) error {
	serversToWrite := make(map[string]config.MCPServer, len(cfg.Servers))
	for name, srv := range cfg.Servers {
		if !plan.IsMCPServerSkipped(normKey, name) {
			serversToWrite[name] = srv
		}
	}

	existing, err := os.ReadFile(dest)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		rendered := renderTomlMCPServers(serversToWrite)
		if !strings.HasSuffix(rendered, "\n") {
			rendered += "\n"
		}
		if plan.Lock != nil {
			for name := range serversToWrite {
				plan.Lock.RecordMCP(normKey, name)
			}
		}
		return writeFileWithParents(dest, []byte(rendered))
	}

	if len(serversToWrite) == 0 {
		return nil
	}

	updated, err := updateTomlMCPServersWithLock(string(existing), serversToWrite, plan.Lock, normKey)
	if err != nil {
		return fmt.Errorf("cannot parse existing TOML config at %s: %w", dest, err)
	}

	if plan.Lock != nil {
		for name := range serversToWrite {
			plan.Lock.RecordMCP(normKey, name)
		}
	}

	if string(existing) == updated {
		return nil
	}

	return writeFileWithParents(dest, []byte(updated))
}

func updateTomlMCPServers(existingContent string, servers map[string]config.MCPServer) (string, error) {
	return updateTomlMCPServersWithLock(existingContent, servers, nil, "")
}

func updateTomlMCPServersWithLock(existingContent string, servers map[string]config.MCPServer, lock *config.LockFile, normKey string) (string, error) {
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
	inBasic := false
	inLiteral := false

	for _, line := range rawLines {
		isInside := inBasic || inLiteral
		header := ""
		isTable := false
		if !isInside {
			header, isTable = parseTOMLTableHeader(line)
		}

		if isTable {
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

		inBasic, inLiteral = scanMultilineState(line, inBasic, inLiteral)
	}
	sections = append(sections, current)

	// 3. Render the managed servers
	renderedManaged := strings.TrimRight(renderTomlMCPServers(servers), "\r\n")

	// 4. Identify which sections are managed by AgentSync
	firstManagedIdx := -1
	managedIndices := make(map[int]bool)

	for idx, sec := range sections {
		srvName := mcpServerNameForTable(sec.header)
		isManaged := false
		if srvName != "" {
			if _, inConfig := servers[srvName]; inConfig {
				isManaged = true
			}
		} else if isRootMCPHeader(sec.header) {
			isManaged = true
		}

		if isManaged {
			managedIndices[idx] = true
			if firstManagedIdx == -1 {
				firstManagedIdx = idx
			}
		}
	}

	// 5. Reconstruct document lines
	var outLines []string
	inserted := false

	for idx, sec := range sections {
		if managedIndices[idx] {
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

func scanMultilineState(line string, inBasic, inLiteral bool) (bool, bool) {
	i := 0
	n := len(line)

	for i < n {
		if inBasic {
			if i+2 < n && line[i] == '"' && line[i+1] == '"' && line[i+2] == '"' {
				bs := 0
				for k := i - 1; k >= 0 && line[k] == '\\'; k-- {
					bs++
				}
				if bs%2 == 0 {
					inBasic = false
					i += 3
					continue
				}
			}
			i++
			continue
		}

		if inLiteral {
			if i+2 < n && line[i] == '\'' && line[i+1] == '\'' && line[i+2] == '\'' {
				inLiteral = false
				i += 3
				continue
			}
			i++
			continue
		}

		if line[i] == '#' {
			break
		}

		if i+2 < n && line[i] == '"' && line[i+1] == '"' && line[i+2] == '"' {
			inBasic = true
			i += 3
			continue
		}
		if i+2 < n && line[i] == '\'' && line[i+1] == '\'' && line[i+2] == '\'' {
			inLiteral = true
			i += 3
			continue
		}

		if line[i] == '"' {
			i++
			for i < n {
				if line[i] == '"' {
					bs := 0
					for k := i - 1; k >= 0 && line[k] == '\\'; k-- {
						bs++
					}
					if bs%2 == 0 {
						i++
						break
					}
				}
				i++
			}
			continue
		}

		if line[i] == '\'' {
			i++
			for i < n {
				if line[i] == '\'' {
					i++
					break
				}
				i++
			}
			continue
		}

		i++
	}

	return inBasic, inLiteral
}

func extractTableBody(lines []string) string {
	idx := 0
	for idx < len(lines) {
		trimmed := strings.TrimSpace(lines[idx])
		if strings.HasPrefix(trimmed, "[") {
			break
		}
		idx++
	}
	if idx >= len(lines) {
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return strings.TrimSpace(strings.Join(lines[idx:], "\n"))
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
	for name, srv := range servers {
		if strings.TrimSpace(srv.Command) == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
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

func renderSingleTomlMCPServer(name string, server config.MCPServer) string {
	if strings.TrimSpace(server.Command) == "" {
		return ""
	}
	var buf strings.Builder
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
	return buf.String()
}

func resolveGuidelinesTarget(target Agent) string {
	if target.ID == "cursor" {
		return cursorGuidelinesOut
	}

	return target.GuidelinesFile
}

func markdownFiles(dir string) ([]string, error) {
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) == ".md" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.Sort(files)
	return files, nil
}

func writeFileWithParents(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return osWriteFile(path, data, 0o644)
}

func copyDir(src, dst string) error {
	return CopyPackageDir(src, dst, src)
}

// CopyPackageDir copies a directory tree from src to dst, validating that any symlinks stay within pkgRoot.
func CopyPackageDir(src, dst, pkgRoot string) error {
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
			if err := CopyPackageDir(srcPath, dstPath, pkgRoot); err != nil {
				return err
			}
			continue
		}

		if err := CopyFileWithinPackage(srcPath, dstPath, pkgRoot); err != nil {
			return err
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	return CopyFileWithinPackage(src, dst, filepath.Dir(src))
}

// CopyFileWithinPackage copies src to dst. If src is a symlink, it validates that the symlink
// is relative and stays strictly within pkgRoot. If unsafe or difficult to guarantee, it returns an error.
func CopyFileWithinPackage(src, dst, pkgRoot string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}

	dstDir := filepath.Dir(dst)
	if dirInfo, err := os.Lstat(dstDir); err == nil {
		if dirInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe destination %s: parent directory is a symlink", dst)
		}
	}

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return copySymlinkWithinPackage(src, dst, pkgRoot)
	}

	// Never follow destination symlinks
	if dstInfo, err := os.Lstat(dst); err == nil {
		if dstInfo.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(dst); err != nil {
				return fmt.Errorf("failed to remove destination symlink %s: %w", dst, err)
			}
		}
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	if err := out.Chmod(info.Mode()); err != nil {
		return err
	}

	return out.Close()
}

func copySymlinkWithinPackage(src, dst, pkgRoot string) error {
	if err := validateSymlinkWithinPackage(src, pkgRoot); err != nil {
		return err
	}

	linkTarget, err := os.Readlink(src)
	if err != nil {
		return err
	}

	cleanPkgRoot := filepath.Clean(pkgRoot)
	srcDir := filepath.Dir(src)

	if filepath.IsAbs(linkTarget) {
		pkgRootAbs, _ := filepath.Abs(cleanPkgRoot)
		srcDirAbs, _ := filepath.Abs(srcDir)
		relToSrcDir, _ := filepath.Rel(srcDirAbs, linkTarget)
		if rel, err := filepath.Rel(pkgRootAbs, linkTarget); err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			linkTarget = relToSrcDir
		}
	}

	_ = os.Remove(dst)
	return os.Symlink(linkTarget, dst)
}

func validateSymlinkWithinPackage(src, pkgRoot string) error {
	linkTarget, err := os.Readlink(src)
	if err != nil {
		return err
	}

	cleanPkgRoot := filepath.Clean(pkgRoot)
	srcDir := filepath.Dir(src)

	// 1. If symlink target is absolute, verify whether it points inside the package.
	// If it points outside, reject it. If it points inside, normalize to relative.
	if filepath.IsAbs(linkTarget) {
		pkgRootAbs, err := filepath.Abs(cleanPkgRoot)
		if err != nil {
			return err
		}
		relToPkg, err := filepath.Rel(pkgRootAbs, linkTarget)
		if err != nil || strings.HasPrefix(relToPkg, "..") || relToPkg == ".." || filepath.IsAbs(relToPkg) {
			return fmt.Errorf("unsafe symlink %s: absolute target %s escapes skill package %s", src, linkTarget, cleanPkgRoot)
		}
		srcDirAbs, err := filepath.Abs(srcDir)
		if err != nil {
			return err
		}
		relToSrcDir, err := filepath.Rel(srcDirAbs, linkTarget)
		if err != nil {
			return fmt.Errorf("unsafe symlink %s: %w", src, err)
		}
		linkTarget = relToSrcDir
	}

	// 2. Reject symlinks that lexically escape the package
	targetPath := filepath.Clean(filepath.Join(srcDir, linkTarget))

	relToPkg, err := filepath.Rel(cleanPkgRoot, targetPath)
	if err != nil || strings.HasPrefix(relToPkg, "..") || relToPkg == ".." || filepath.IsAbs(relToPkg) {
		return fmt.Errorf("unsafe symlink %s: target %s escapes skill package %s", src, linkTarget, cleanPkgRoot)
	}

	// 3. Reject broken symlinks
	if _, err := os.Stat(targetPath); err != nil {
		return fmt.Errorf("broken symlink %s: target %s does not exist: %w", src, targetPath, err)
	}

	// 4. Reject circular symlinks or symlinks escaping through intermediate symlinks
	evalTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		return fmt.Errorf("invalid symlink %s: %w", src, err)
	}

	evalPkgRoot, err := filepath.EvalSymlinks(cleanPkgRoot)
	if err != nil {
		evalPkgRoot = cleanPkgRoot
	}

	relEval, err := filepath.Rel(evalPkgRoot, evalTarget)
	if err != nil || strings.HasPrefix(relEval, "..") || relEval == ".." || filepath.IsAbs(relEval) {
		return fmt.Errorf("unsafe symlink %s: resolved target %s escapes skill package %s", src, evalTarget, evalPkgRoot)
	}

	return nil
}
