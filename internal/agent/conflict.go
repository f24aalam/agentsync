package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/f24aalam/agentsync/internal/config"
)

// InstallConflict describes whether a category needs a user prompt (would write + target exists).
type InstallConflict struct {
	AgentID            string       // agent ID (empty for skills-dir–only rows)
	SkillsDir          string       // set when Kind is KindSkillPackage or KindSkillsDir
	SkillName          string       // set when Kind is KindSkillPackage
	Kind               ConflictKind
	DestPath           string       // set for guidelines destination or MCP destination
	// AgentIDs lists agents sharing SkillsDir when Kind is KindSkillPackage/KindSkillsDir, or sharing DestPath for guidelines.
	AgentIDs           []string
	Question           string
	StepKey            string
	IsManaged          bool         // true if this resource is known to be managed by AgentSync
	ConflictingServers []string     // set when Kind is KindMCP
}

// ConflictKind identifies which install step may conflict.
type ConflictKind int

const (
	KindGuidelines ConflictKind = iota
	KindSkillsDir
	KindSkillPackage
	KindMCP
)

// DetectInstallConflicts returns prompts needed for the given agents at project root.
// root is usually "."; tests may pass a temp directory.
func DetectInstallConflicts(agents []Agent, root string) ([]InstallConflict, error) {
	if root == "" {
		root = "."
	}

	var out []InstallConflict
	lock, _ := config.ReadFullLock(filepath.Join(root, ".ai/sync.lock"))

	wouldG, err := wouldInstallGuidelines(root)
	if err != nil {
		return nil, err
	}
	wouldS := wouldInstallSkills(root)
	wouldM, err := wouldInstallMCP(root)
	if err != nil {
		return nil, err
	}

	seenGuidelinesDest := make(map[string]bool)
	for _, a := range agents {
		dest := resolveGuidelinesTarget(a)
		if seenGuidelinesDest[dest] {
			continue
		}
		if wouldG && guidelinesConflict(root, a) {
			seenGuidelinesDest[dest] = true
			ids := agentIDsUsingGuidelinesDest(agents, dest)
			slices.Sort(ids)
			question := a.Name + ": guidelines file already exists. Overwrite with merged .ai/guidelines?"

			destData, _ := os.ReadFile(filepath.Join(root, dest))
			_, hasMarkers := ExtractManagedSection(string(destData))

			out = append(out, InstallConflict{
				AgentID:   a.ID,
				Kind:      KindGuidelines,
				DestPath:  dest,
				AgentIDs:  ids,
				StepKey:   "install/" + a.ID + "/guidelines",
				Question:  question,
				IsManaged: hasMarkers,
			})
		}
	}

	if wouldS {
		seenSkills := make(map[string]bool)
		srcSkillsRoot := filepath.Join(root, skillsSourceDir)
		srcSkillEntries, _ := os.ReadDir(srcSkillsRoot)

		for _, raw := range UniqueSkillsDirs(agents) {
			if raw == "" {
				continue
			}
			dirKey := SkillDirKey(raw)
			if seenSkills[dirKey] {
				continue
			}
			seenSkills[dirKey] = true
			if !skillsDirUsedBySupportedAgentKey(agents, dirKey) {
				continue
			}

			ids := agentIDsUsingSkillsDirKey(agents, dirKey)
			slices.Sort(ids)

			for _, entry := range srcSkillEntries {
				if !entry.IsDir() {
					continue
				}
				skillName := entry.Name()
				srcDir := filepath.Join(srcSkillsRoot, skillName)
				dstDir := filepath.Join(root, dirKey, skillName)

				exists, differs, err := compareSkillPackages(srcDir, dstDir)
				if err != nil || !exists || !differs {
					continue
				}

				isManaged := lock != nil && lock.IsSkillPackageManaged(dirKey, skillName)
				stepKey := "install/skills/" + skillsStepKeySegment(dirKey) + "/" + skillName

				question := fmt.Sprintf("Skill package %q in %s already exists and differs from .ai/skills/%s. Overwrite?", skillName, dirKey, skillName)
				if len(ids) > 1 {
					question = fmt.Sprintf("Skill package %q in %s already exists and differs from .ai/skills/%s (agents: %s). Overwrite?", skillName, dirKey, skillName, strings.Join(ids, ", "))
				}

				out = append(out, InstallConflict{
					Kind:      KindSkillPackage,
					SkillsDir: dirKey,
					SkillName: skillName,
					AgentIDs:  ids,
					StepKey:   stepKey,
					Question:  question,
					IsManaged: isManaged,
				})
			}
		}
	}

	if wouldM {
		mcpPath := filepath.Join(root, mcpSourcePath)
		mcpCfg, err := config.ReadMCP(mcpPath)
		if err == nil && len(mcpCfg.Servers) > 0 {
			seenMCPDest := make(map[string]bool)
			for _, a := range agents {
				destPaths, err := resolveMCPDestPaths(a)
				if err != nil {
					continue
				}
				for _, dest := range destPaths {
					if strings.TrimSpace(dest) == "" {
						continue
					}
					destRel := dest
					if filepath.IsAbs(destRel) {
						rel, err := filepath.Rel(root, destRel)
						if err == nil {
							destRel = rel
						}
					}
					destRel = filepath.Clean(destRel)
					if seenMCPDest[destRel] {
						continue
					}
					seenMCPDest[destRel] = true

					conflicting, hasUnmanagedConflict, isMalformed, err := findConflictingMCPServers(root, a, destRel, mcpCfg, lock)
					if err != nil {
						return nil, err
					}
					if isMalformed {
						continue
					}
					if len(conflicting) == 0 {
						continue
					}

					var q strings.Builder
					q.WriteString(fmt.Sprintf("%s MCP configuration differs:\n%s\n\nConflicting servers:\n", a.Name, destRel))
					for _, s := range conflicting {
						q.WriteString(fmt.Sprintf("- %s\n", s))
					}
					q.WriteString("\nReplace these entries using .ai/mcp.toml?")

					ids := agentIDsUsingMCPDest(agents, destRel, root)
					slices.Sort(ids)

					out = append(out, InstallConflict{
						AgentID:            a.ID,
						AgentIDs:           ids,
						Kind:               KindMCP,
						DestPath:           destRel,
						StepKey:            "install/" + a.ID + "/mcp",
						Question:           q.String(),
						IsManaged:          !hasUnmanagedConflict,
						ConflictingServers: conflicting,
					})
				}
			}
		}
	}

	return out, nil
}

func skillsStepKeySegment(dir string) string {
	return strings.ReplaceAll(filepath.ToSlash(strings.TrimSuffix(dir, "/")), "/", "_")
}

func skillsDirUsedBySupportedAgentKey(agents []Agent, dirKey string) bool {
	for _, a := range agents {
		if a.SkillsSupported && SkillDirKey(a.SkillsDir) == dirKey {
			return true
		}
	}
	return false
}

func agentIDsUsingSkillsDirKey(agents []Agent, dirKey string) []string {
	var ids []string
	for _, a := range agents {
		if a.SkillsSupported && SkillDirKey(a.SkillsDir) == dirKey {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func wouldInstallGuidelines(root string) (bool, error) {
	files, err := markdownFiles(filepath.Join(root, guidelinesSourceDir))
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

func wouldInstallSkills(root string) bool {
	entries, err := os.ReadDir(filepath.Join(root, skillsSourceDir))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			return true
		}
	}
	return false
}

func wouldInstallMCP(root string) (bool, error) {
	p := filepath.Join(root, mcpSourcePath)
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	cfg, err := config.ReadMCP(p)
	if err != nil {
		return false, err
	}
	return len(cfg.Servers) > 0, nil
}

func guidelinesConflict(root string, a Agent) bool {
	dest := resolveGuidelinesTarget(a)
	destPath := filepath.Join(root, dest)
	if !fileExistsNonEmpty(destPath) {
		return false
	}

	existingData, err := os.ReadFile(destPath)
	if err != nil {
		return false
	}
	existingContent := string(existingData)

	if (a.ID == "cursor" || strings.HasSuffix(dest, ".mdc")) && !strings.HasPrefix(strings.TrimSpace(existingContent), "---") {
		frontmatter := "---\ndescription: Project Guidelines\nglobs: *\nalwaysApply: true\n---\n\n"
		if existingContent != "" {
			existingContent = frontmatter + existingContent
		} else {
			existingContent = frontmatter
		}
	}

	files, err := markdownFiles(filepath.Join(root, guidelinesSourceDir))
	if err != nil || len(files) == 0 {
		return false
	}
	var parts []string
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err == nil {
			trimmed := strings.TrimSpace(string(data))
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}
	if len(parts) == 0 {
		return false
	}
	mergedContent := strings.Join(parts, "\n\n")

	finalContent, err := injectManagedGuidelines(existingContent, mergedContent)
	if err != nil {
		return false
	}

	// 1. If target file already matches what would be written, NO CONFLICT!
	if finalContent == existingContent {
		return false
	}

	// 2. Existing file differs from desired output.
	// Whether it has managed markers (IsManaged=true) or is pre-existing without markers (IsManaged=false),
	// this is a conflict requiring prompt (or --yes for managed).
	return true
}

func agentIDsUsingGuidelinesDest(agents []Agent, dest string) []string {
	var ids []string
	for _, a := range agents {
		if resolveGuidelinesTarget(a) == dest {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func agentNamesUsingGuidelinesDest(agents []Agent, dest string) []string {
	var names []string
	for _, a := range agents {
		if resolveGuidelinesTarget(a) == dest {
			names = append(names, a.Name)
		}
	}
	return names
}

func agentIDsUsingMCPDest(agents []Agent, destPath string, root string) []string {
	var ids []string
	cleanDest := filepath.Clean(destPath)
	for _, a := range agents {
		destPaths, err := resolveMCPDestPaths(a)
		if err != nil {
			continue
		}
		for _, d := range destPaths {
			destRel := d
			if filepath.IsAbs(destRel) {
				rel, err := filepath.Rel(root, destRel)
				if err == nil {
					destRel = rel
				}
			}
			if filepath.Clean(destRel) == cleanDest {
				ids = append(ids, a.ID)
				break
			}
		}
	}
	return ids
}

func skillsDirConflict(root string, skillsDir string) bool {
	if skillsDir == "" {
		return false
	}
	srcRoot := filepath.Join(root, skillsSourceDir)
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		srcDir := filepath.Join(srcRoot, entry.Name())
		dstDir := filepath.Join(root, skillsDir, entry.Name())
		exists, differs, err := compareSkillPackages(srcDir, dstDir)
		if err == nil && exists && differs {
			return true
		}
	}
	return false
}

func mcpConflict(root string, a Agent) bool {
	destPaths, err := resolveMCPDestPaths(a)
	if err != nil {
		return false
	}
	mcpPath := filepath.Join(root, mcpSourcePath)
	cfg, err := config.ReadMCP(mcpPath)
	if err != nil || len(cfg.Servers) == 0 {
		return false
	}

	for _, p := range destPaths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		destRel := p
		if filepath.IsAbs(destRel) {
			rel, err := filepath.Rel(root, destRel)
			if err == nil {
				destRel = rel
			}
		}
		conflicting, _, _, err := findConflictingMCPServers(root, a, destRel, cfg, nil)
		if err == nil && len(conflicting) > 0 {
			return true
		}
	}
	return false
}

func findConflictingMCPServers(root string, a Agent, dest string, cfg config.MCPConfig, lock *config.LockFile) (conflicting []string, hasUnmanagedConflict bool, isMalformed bool, err error) {
	checkPath := filepath.Join(root, dest)
	data, err := os.ReadFile(checkPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}

	if a.MCPFormat == MCPFormatTOML {
		var decoded map[string]any
		if _, err := toml.Decode(string(data), &decoded); err != nil {
			return nil, false, true, nil
		}
		existingServers, _ := decoded["mcp_servers"].(map[string]any)

		for name, server := range cfg.Servers {
			if strings.TrimSpace(server.Command) == "" {
				continue
			}
			existingServerRaw, exists := existingServers[name]
			if !exists {
				continue
			}
			existingServerMap, ok := existingServerRaw.(map[string]any)
			if !ok || !mcpServerMatches(existingServerMap, server) {
				conflicting = append(conflicting, name)
				if lock == nil || !lock.IsMCPManaged(dest, name) {
					hasUnmanagedConflict = true
				}
				continue
			}
		}

		slices.Sort(conflicting)
		return conflicting, hasUnmanagedConflict, false, nil
	}

	// JSON format
	var rootMap map[string]any
	if err := json.Unmarshal(data, &rootMap); err != nil {
		return nil, false, true, nil
	}

	rootKey, _ := mcpJSONRoot(a)
	existingServers, _ := rootMap[rootKey].(map[string]any)

	for name, server := range cfg.Servers {
		rendered := renderManagedServerJSON(a, name, server)
		if rendered == nil {
			continue
		}
		existingServer, exists := existingServers[name]
		if !exists {
			continue
		}

		if !jsonServerMatches(existingServer, rendered) {
			conflicting = append(conflicting, name)
			if lock == nil || !lock.IsMCPManaged(dest, name) {
				hasUnmanagedConflict = true
			}
		}
	}

	slices.Sort(conflicting)
	return conflicting, hasUnmanagedConflict, false, nil
}

func fileExistsNonEmpty(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return st.Size() > 0
}
