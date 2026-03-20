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
		if key == "" {
			key = variant.Agent.Name + "::" + variant.Path
		}
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
	var b strings.Builder
	typ := strings.TrimSpace(variant.Server.Type)
	if typ == "" {
		typ = "local"
	}
	b.WriteString(typ)
	b.WriteString("\x00")
	b.WriteString(strings.TrimSpace(variant.Server.Command))
	b.WriteString("\x00")
	b.WriteString(strings.Join(variant.Server.Args, "\x1f"))
	b.WriteString("\x00")

	if len(variant.Server.URL) > 0 {
		b.WriteString("url\x00")
		b.WriteString(variant.Server.URL)
		b.WriteString("\x1f")
	}

	// env
	if len(variant.Server.Env) > 0 {
		keys := make([]string, 0, len(variant.Server.Env))
		for k := range variant.Server.Env {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			b.WriteString(k)
			b.WriteString("\x1e")
			b.WriteString(variant.Server.Env[k])
			b.WriteString("\x1f")
		}
		b.WriteString("\x00")
	}

	// headers
	if len(variant.Server.Headers) > 0 {
		keys := make([]string, 0, len(variant.Server.Headers))
		for k := range variant.Server.Headers {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			b.WriteString(k)
			b.WriteString("\x1e")
			b.WriteString(variant.Server.Headers[k])
			b.WriteString("\x1f")
		}
		b.WriteString("\x00")
	}

	// oauth
	if variant.Server.OAuth != nil {
		b.WriteString("oauth\x00")
		b.WriteString(strings.TrimSpace(variant.Server.OAuth.ClientID))
		b.WriteString("\x1f")
		b.WriteString(strings.TrimSpace(variant.Server.OAuth.ClientSecret))
		b.WriteString("\x1f")
		b.WriteString(strings.TrimSpace(variant.Server.OAuth.Scope))
		b.WriteString("\x00")
	}

	return b.String()
}

func firstSourceLabel(sources []DetectedGuideline) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[0].Path
}
