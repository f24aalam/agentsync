package detect

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func Overview(detection ProjectDetection) string {
	var b strings.Builder
	b.WriteString("Found existing agent configs:\n\n")

	groups := DedupGuidelines(detection.Guidelines)
	writeGuidelinesOverview(&b, groups)
	b.WriteString("\n")

	skills := DedupSkills(detection.Skills)
	writeSkillsOverview(&b, skills)
	b.WriteString("\n")

	mcp := DedupMCP(detection.MCPServers)
	writeMCPOverview(&b, mcp)

	return b.String()
}

func writeGuidelinesOverview(b *strings.Builder, groups []GuidelineGroup) {
	b.WriteString("Guidelines\n")
	if len(groups) == 0 {
		b.WriteString("  none\n")
		return
	}
	for _, group := range groups {
		line := fmt.Sprintf("  %s", guidelineLabel(group))
		if len(group.Sources) > 1 {
			line += "  (same content, will merge as one)"
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func guidelineLabel(group GuidelineGroup) string {
	files := make([]string, 0, len(group.Sources))
	agents := make([]string, 0, len(group.Sources))
	for _, source := range group.Sources {
		files = append(files, filepath.Base(source.Path))
		agents = append(agents, source.Agent.Name)
	}
	files = uniqueStrings(files)
	agents = uniqueStrings(agents)
	return fmt.Sprintf("%s (%s)", strings.Join(files, ", "), strings.Join(agents, ", "))
}

func writeSkillsOverview(b *strings.Builder, groups []SkillGroup) {
	b.WriteString("Skills\n")
	if len(groups) == 0 {
		b.WriteString("  none\n")
		return
	}
	for _, group := range groups {
		agents := skillAgents(group.Variants)
		line := fmt.Sprintf("  %s  (%s)", group.Name, strings.Join(agents, ", "))
		if group.Conflict {
			line += "  (conflict: different content)"
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func writeMCPOverview(b *strings.Builder, groups []MCPGroup) {
	b.WriteString("MCP Servers\n")
	if len(groups) == 0 {
		b.WriteString("  none\n")
		return
	}
	for _, group := range groups {
		agents := mcpAgents(group.Variants)
		line := fmt.Sprintf("  %s  (%s)", group.Name, strings.Join(agents, ", "))
		if group.Conflict {
			line += "  (conflict: different config)"
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
}

func skillAgents(variants []DetectedSkill) []string {
	agents := make([]string, 0, len(variants))
	for _, variant := range variants {
		agents = append(agents, variant.Agent.Name)
	}
	return uniqueStrings(agents)
}

func mcpAgents(variants []DetectedMCPServer) []string {
	agents := make([]string, 0, len(variants))
	for _, variant := range variants {
		agents = append(agents, variant.Agent.Name)
	}
	return uniqueStrings(agents)
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	slices.Sort(out)
	return out
}
