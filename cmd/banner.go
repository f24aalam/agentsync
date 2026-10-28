package cmd

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
)

// agentsyncBanner is the final Unicode block wordmark (~80 columns).
const agentsyncBanner = ` █████╗  ██████╗ ███████╗███╗   ██╗████████╗███████╗██╗   ██╗███╗   ██╗ ██████╗
██╔══██╗██╔════╝ ██╔════╝████╗  ██║╚══██╔══╝██╔════╝╚██╗ ██╔╝████╗  ██║██╔════╝
███████║██║  ███╗█████╗  ██╔██╗ ██║   ██║   ███████╗ ╚████╔╝ ██╔██╗ ██║██║     
██╔══██║██║   ██║██╔══╝  ██║╚██╗██║   ██║   ╚════██║  ╚██╔╝  ██║╚██╗██║██║     
██║  ██║╚██████╔╝███████╗██║ ╚████║   ██║   ███████║   ██║   ██║ ╚████║╚██████╗
╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚══════╝   ╚═╝   ╚═╝  ╚═══╝ ╚═════╝
                                                                               `

const agentsyncTagline = "Sync AI guidelines, skills & MCP across coding agents"

func printAgentsyncBanner(out io.Writer) {
	bannerStyle := lipgloss.NewStyle().
		Foreground(cliTheme.Primary).
		Bold(true)
	tagStyle := lipgloss.NewStyle().
		Foreground(cliTheme.Muted).
		MarginLeft(4)

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, bannerStyle.Render(agentsyncBanner))
	_, _ = fmt.Fprintln(out, tagStyle.Render(agentsyncTagline))
	_, _ = fmt.Fprintln(out)
}
