package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/charmbracelet/lipgloss"
	"github.com/f24aalam/agentsync/internal/agent"
	"github.com/f24aalam/agentsync/internal/config"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured guidelines, skills, MCP servers, and target agents",
		RunE:  runListCommand,
	}
}

func runListCommand(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(aiDirName); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			printInitRequired(cmd, ".ai")
			return wrapSilentError(err)
		}

		return err
	}

	guidelines, err := listGuidelineFiles()
	if err != nil {
		return err
	}

	skills, err := listSkillDirs()
	if err != nil {
		return err
	}

	servers, err := listMCPServers()
	if err != nil {
		return err
	}

	agents, initialized, err := listSelectedAgents()
	if err != nil {
		return err
	}

	headerStyle := lipgloss.NewStyle().Bold(true)
	renderSection := func(title string, items []string, emptyLabel string) {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render(title))
		if len(items) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n\n", emptyLabel)
			return
		}

		for _, item := range items {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", item)
		}

		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	renderSection(fmt.Sprintf("Guidelines (%d)", len(guidelines)), guidelines, "none")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render(fmt.Sprintf("Skills (%d) \u2192 .agents/skills/", len(skills))))

	if len(skills) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  none\n\n")
	} else {
		for _, skill := range skills {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", skill)
		}

		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}
	renderSection(fmt.Sprintf("MCP Servers (%d)", len(servers)), servers, "none")

	agentsTitle := "Agents (sync.lock)"
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), headerStyle.Render(agentsTitle))

	if !initialized {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  not initialized")
		return nil
	}

	if len(agents) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  none")
		return nil
	}

	for _, name := range agents {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", name)
	}

	return nil
}

func listGuidelineFiles() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(aiDirName, "guidelines"))
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

		files = append(files, entry.Name())
	}
	slices.Sort(files)

	return files, nil
}

func listSkillDirs() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(aiDirName, "skills"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	slices.Sort(dirs)

	return dirs, nil
}

func listMCPServers() ([]string, error) {
	cfg, err := config.ReadMCP(filepath.Join(aiDirName, "mcp.toml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	servers := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		servers = append(servers, name)
	}

	slices.Sort(servers)

	return servers, nil
}

func listSelectedAgents() ([]string, bool, error) {
	ids, err := config.ReadLock(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}

		return nil, false, err
	}

	names := make([]string, 0, len(ids))
	for _, id := range ids {
		target, ok := agent.ByID(id)
		if ok {
			names = append(names, target.Name)
			continue
		}
		names = append(names, fmt.Sprintf("%s (unknown)", id))
	}

	return names, true, nil
}
