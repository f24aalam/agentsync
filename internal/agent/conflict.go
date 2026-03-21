package agent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/f24aalam/agentsync/internal/config"
)

// InstallConflict describes whether a category needs a user prompt (would write + target exists).
type InstallConflict struct {
	AgentID   string   // agent ID (empty for skills-dir–only rows)
	SkillsDir string   // set when Kind is KindSkillsDir
	Kind      ConflictKind
	// AgentIDs lists agents sharing SkillsDir when Kind is KindSkillsDir.
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

	for _, a := range agents {
		if wouldG && guidelinesConflict(root, a) {
			out = append(out, InstallConflict{
				AgentID:  a.ID,
				Kind:     KindGuidelines,
				StepKey:  "install/" + a.ID + "/guidelines",
				Question: a.Name + ": guidelines file already exists. Overwrite with merged .ai/guidelines?",
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
	dest := filepath.Join(root, resolveGuidelinesTarget(a))
	return fileExistsNonEmpty(dest)
}

func skillsDirConflict(root string, skillsDir string) bool {
	if skillsDir == "" {
		return false
	}
	p := filepath.Join(root, skillsDir)
	entries, err := os.ReadDir(p)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

func mcpConflict(root string, a Agent) bool {
	paths, err := resolveMCPDestPaths(a)
	if err != nil {
		return false
	}
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		check := p
		if !filepath.IsAbs(check) {
			check = filepath.Join(root, check)
		}
		if st, err := os.Stat(check); err == nil && !st.IsDir() {
			return true
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
