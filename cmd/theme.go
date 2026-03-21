package cmd

import "github.com/charmbracelet/lipgloss"

// Green-only palette for all CLI output (aligned with stepflow DefaultTheme).
const (
	ThemeGreen      = lipgloss.Color("#4ade80") // primary accent
	ThemeGreenMuted = lipgloss.Color("#22c55e") // secondary (still green)
)
