package detect

import (
	"slices"
	"strings"
)

type GuidelineGroup struct {
	Content string
	Sources []DetectedGuideline
}

type SkillGroup struct {
	Name     string
	Variants []DetectedSkill
	Conflict bool
}

type MCPGroup struct {
	Name     string
	Variants []DetectedMCPServer
	Conflict bool
}

func DedupGuidelines(input []DetectedGuideline) []GuidelineGroup {
	byContent := make(map[string][]DetectedGuideline)
	for _, item := range input {
		byContent[item.Content] = append(byContent[item.Content], item)
	}

	groups := make([]GuidelineGroup, 0, len(byContent))
	for content, sources := range byContent {
		groups = append(groups, GuidelineGroup{
			Content: content,
			Sources: sources,
		})
	}

	slices.SortFunc(groups, func(a, b GuidelineGroup) int {
		return strings.Compare(firstSourceLabel(a.Sources), firstSourceLabel(b.Sources))
	})
	return groups
}

func DedupSkills(input []DetectedSkill) []SkillGroup {
	byName := make(map[string][]DetectedSkill)
	for _, item := range input {
		byName[item.Name] = append(byName[item.Name], item)
	}

	groups := make([]SkillGroup, 0, len(byName))
	for name, variants := range byName {
		unique := uniqueSkillVariants(variants)
		groups = append(groups, SkillGroup{
			Name:     name,
			Variants: unique,
			Conflict: len(unique) > 1,
		})
	}

	slices.SortFunc(groups, func(a, b SkillGroup) int {
		return strings.Compare(a.Name, b.Name)
	})
	return groups
}

func DedupMCP(input []DetectedMCPServer) []MCPGroup {
	byName := make(map[string][]DetectedMCPServer)
	for _, item := range input {
		byName[item.Name] = append(byName[item.Name], item)
	}

	groups := make([]MCPGroup, 0, len(byName))
	for name, variants := range byName {
		unique := uniqueMCPVariants(variants)
		groups = append(groups, MCPGroup{
			Name:     name,
			Variants: unique,
			Conflict: len(unique) > 1,
		})
	}

	slices.SortFunc(groups, func(a, b MCPGroup) int {
		return strings.Compare(a.Name, b.Name)
	})
	return groups
}

func uniqueSkillVariants(variants []DetectedSkill) []DetectedSkill {
	seen := make(map[string]DetectedSkill)
	for _, variant := range variants {
		key := variant.SkillMD
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = variant
	}

	out := make([]DetectedSkill, 0, len(seen))
	for _, variant := range seen {
		out = append(out, variant)
	}

	slices.SortFunc(out, func(a, b DetectedSkill) int {
		if a.Agent.Name != b.Agent.Name {
			return strings.Compare(a.Agent.Name, b.Agent.Name)
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out
}

func uniqueMCPVariants(variants []DetectedMCPServer) []DetectedMCPServer {
	seen := make(map[string]DetectedMCPServer)
	for _, variant := range variants {
		key := mcpSignature(variant)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = variant
	}

	out := make([]DetectedMCPServer, 0, len(seen))
	for _, variant := range seen {
		out = append(out, variant)
	}

	slices.SortFunc(out, func(a, b DetectedMCPServer) int {
		if a.Agent.Name != b.Agent.Name {
			return strings.Compare(a.Agent.Name, b.Agent.Name)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func mcpSignature(variant DetectedMCPServer) string {
	return variant.Server.Command + "\x00" + strings.Join(variant.Server.Args, "\x1f")
}

func firstSourceLabel(sources []DetectedGuideline) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[0].Path
}
