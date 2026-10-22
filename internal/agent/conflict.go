package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/f24aalam/agentsync/internal/config"
)

// InstallConflict describes whether a category needs a user prompt (would write + target exists).
type InstallConflict struct {
	AgentID   string       // agent ID (empty for skills-dir–only rows)
	SkillsDir string       // set when Kind is KindSkillsDir
	Kind      ConflictKind
	DestPath  string       // set for guidelines destination
	// AgentIDs lists agents sharing SkillsDir when Kind is KindSkillsDir, or sharing DestPath for guidelines.
	AgentIDs []string
	Question string
	StepKey  string
}

// ConflictKind identifies which install step may conflict.
type ConflictKind int

const (
	KindGuidelines ConflictKind = iota
	KindSkillsDir
	KindMCP
)

// DetectInstallConflicts returns prompts needed for the given agents at project root.
// root is usually "."; tests may pass a temp directory.
func DetectInstallConflicts(agents []Agent, root string) ([]InstallConflict, error) {
	if root == "" {
		root = "."
	}

	var out []InstallConflict

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
			if len(ids) > 1 {
				names := agentNamesUsingGuidelinesDest(agents, dest)
				question = a.Name + ": guidelines file already exists. Overwrite with merged .ai/guidelines?"
				_ = names
			}
			out = append(out, InstallConflict{
				AgentID:  a.ID,
				Kind:     KindGuidelines,
				DestPath: dest,
				AgentIDs: ids,
				StepKey:  "install/" + a.ID + "/guidelines",
				Question: question,
			})
		}
	}

	seenSkills := make(map[string]bool)
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
		if wouldS && skillsDirConflict(root, dirKey) {
			ids := agentIDsUsingSkillsDirKey(agents, dirKey)
			slices.Sort(ids)
			out = append(out, InstallConflict{
				Kind:      KindSkillsDir,
				SkillsDir: dirKey,
				AgentIDs:  ids,
				StepKey:   "install/skills/" + skillsStepKeySegment(dirKey),
				Question: "Skills directory " + dirKey + " already has content (agents: " + strings.Join(ids, ", ") + "). Overwrite from .ai/skills?",
			})
		}
	}

	for _, a := range agents {
		if wouldM && mcpConflict(root, a) {
			out = append(out, InstallConflict{
				AgentID:  a.ID,
				Kind:     KindMCP,
				StepKey:  "install/" + a.ID + "/mcp",
				Question: a.Name + ": MCP config file(s) already exist. Merge/overwrite from .ai/mcp.toml?",
			})
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

	// 2. Check if file has managed markers
	currentManaged, hasMarkers := ExtractManagedSection(existingContent)
	if !hasMarkers {
		// File has pre-existing user content without managed markers.
		return true
	}

	// 3. File has managed markers: check sync.lock to see if user modified it
	lock, err := config.ReadFullLock(filepath.Join(root, ".ai/sync.lock"))
	if err != nil {
		return true
	}

	storedLock, ok := lock.Guidelines[dest]
	if !ok || storedLock.Hash == "" {
		return true
	}

	// If current managed section matches stored hash, user did not touch it (safe update)
	if config.HashString(currentManaged) == storedLock.Hash {
		return false
	}

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

func skillsDirConflict(root string, skillsDir string) bool {
	if skillsDir == "" {
		return false
	}
	destDir := filepath.Join(root, skillsDir)
	if _, err := os.Stat(destDir); err != nil {
		return false
	}

	srcRoot := filepath.Join(root, skillsSourceDir)
	srcEntries, err := os.ReadDir(srcRoot)
	if err != nil {
		return false
	}

	lock, _ := config.ReadFullLock(filepath.Join(root, ".ai/sync.lock"))

	hasAnyConflict := false
	hasAnyFiles := false

	for _, entry := range srcEntries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		skillSrcDir := filepath.Join(srcRoot, skillName)
		skillDestDir := filepath.Join(destDir, skillName)

		_ = filepath.Walk(skillSrcDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			hasAnyFiles = true
			rel, _ := filepath.Rel(skillSrcDir, path)
			destPath := filepath.Join(skillDestDir, rel)
			normDestRel, _ := filepath.Rel(root, destPath)

			destData, err := os.ReadFile(destPath)
			if err != nil {
				return nil
			}
			srcData, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			if bytes.Equal(destData, srcData) {
				return nil
			}

			if lock != nil {
				if stored, ok := lock.Skills[normDestRel]; ok && stored.Hash != "" {
					if config.HashBytes(destData) == stored.Hash {
						return nil
					}
				}
			}

			hasAnyConflict = true
			return nil
		})
	}

	if !hasAnyConflict && hasAnyFiles {
		if lock == nil || len(lock.Skills) == 0 {
			entries, _ := os.ReadDir(destDir)
			if len(entries) > 0 {
				return true
			}
		}
	}

	return hasAnyConflict
}

func mcpConflict(root string, a Agent) bool {
	paths, err := resolveMCPDestPaths(a)
	if err != nil {
		return false
	}
	mcpPath := filepath.Join(root, mcpSourcePath)
	cfg, err := config.ReadMCP(mcpPath)
	if err != nil || len(cfg.Servers) == 0 {
		return false
	}

	lock, _ := config.ReadFullLock(filepath.Join(root, ".ai/sync.lock"))

	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		check := p
		if !filepath.IsAbs(check) {
			check = filepath.Join(root, check)
		}
		st, err := os.Stat(check)
		if err != nil || st.IsDir() {
			continue
		}

		normKey, _ := filepath.Rel(root, check)

		existingData, err := os.ReadFile(check)
		if err != nil {
			continue
		}

		if a.MCPFormat == MCPFormatTOML {
			updated, err := updateTomlMCPServers(string(existingData), cfg.Servers)
			if err != nil {
				continue
			}
			if strings.TrimSpace(updated) == strings.TrimSpace(string(existingData)) {
				continue
			}
			if lock == nil || lock.MCP == nil || len(lock.MCP[normKey]) == 0 {
				return true
			}
			continue
		}

		// JSON format
		var rootMap map[string]any
		if err := json.Unmarshal(existingData, &rootMap); err != nil {
			continue
		}
		rootKey, _ := mcpJSONRoot(a)
		existingServers, _ := rootMap[rootKey].(map[string]any)

		managed := make(map[string]any, len(cfg.Servers))
		for name, server := range cfg.Servers {
			managed[name] = renderManagedServerJSON(a, name, server)
		}

		allMatch := true
		for name, val := range managed {
			curr, exists := existingServers[name]
			if !exists {
				allMatch = false
				break
			}
			valBytes, _ := json.Marshal(val)
			currBytes, _ := json.Marshal(curr)
			if !bytes.Equal(valBytes, currBytes) {
				allMatch = false
				break
			}
		}

		if allMatch {
			hasObsolete := false
			if lock != nil && lock.MCP != nil && lock.MCP[normKey] != nil {
				for trackedName := range lock.MCP[normKey] {
					if _, inConfig := cfg.Servers[trackedName]; !inConfig {
						if _, exists := existingServers[trackedName]; exists {
							hasObsolete = true
							break
						}
					}
				}
			}
			if !hasObsolete {
				continue
			}
		}

		if lock == nil || lock.MCP == nil || len(lock.MCP[normKey]) == 0 {
			return true
		}

		for name, val := range managed {
			curr, exists := existingServers[name]
			if exists {
				valBytes, _ := json.Marshal(val)
				currBytes, _ := json.Marshal(curr)
				if !bytes.Equal(valBytes, currBytes) {
					if stored, ok := lock.MCP[normKey][name]; ok {
						if config.HashBytes(currBytes) != stored.Hash {
							return true
						}
					} else {
						return true
					}
				}
			}
		}
	}

	return false
}

func fileExistsNonEmpty(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return false
	}
	return st.Size() > 0
}
