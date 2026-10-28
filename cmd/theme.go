package cmd

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	stepflow "github.com/f24aalam/stepflow/pkg"
)

// cliTheme is shared by interactive flows and standalone CLI output.
var cliTheme = stepflow.DefaultTheme()

// PrintError prints a fatal error using the CLI theme and the output's color capabilities.
func PrintError(out io.Writer, err error) {
	if err == nil {
		return
	}
	style := lipgloss.NewRenderer(out).NewStyle().Bold(true).Foreground(cliTheme.Error)
	_, _ = fmt.Fprintf(out, "%s %s\n", style.Render("Error:"), err)
}
