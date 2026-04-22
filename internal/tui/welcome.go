package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// appVersion is the version string for the welcome banner.
// This should be kept in sync with bootstrap.appVersion.
const appVersion = "0.1.0"

// WelcomeHeader contains data for the startup welcome banner.
type WelcomeHeader struct {
	version  string
	model    string
	cwd      string
	shown    bool // whether the header has been shown (only show once)
}

// NewWelcomeHeader creates a new welcome header with default values.
func NewWelcomeHeader(model, cwd string) WelcomeHeader {
	return WelcomeHeader{
		version: appVersion,
		model:   model,
		cwd:     cwd,
		shown:   false,
	}
}

// MarkShown marks the header as shown.
func (w WelcomeHeader) MarkShown() WelcomeHeader {
	w.shown = true
	return w
}

// IsShown returns whether the header has been shown.
func (w WelcomeHeader) IsShown() bool {
	return w.shown
}

// View renders the welcome header banner.
// Format:
//
//	     Claude Code Go v0.1.0
//	     claude-sonnet-4-20250514 · API Usage Billing
//	     ~/path/to/cwd
//	     Welcome to Claude Code Go!  /effort to tune speed vs. intelligence
func (w WelcomeHeader) View(width int, theme Theme) string {
	if w.shown {
		return ""
	}

	var sb strings.Builder

	// ASCII art logo (cute face without ears - cleaner look)
	logo := renderLogo(theme)

	// Version line
	versionLine := primaryStyle(theme).Bold(true).Render(fmt.Sprintf("Claude Code Go v%s", w.version))

	// Model + billing info
	modelStr := w.model
	if modelStr == "" {
		modelStr = "claude-sonnet-4-20250514"
	}
	modelLine := lipgloss.JoinHorizontal(
		lipgloss.Left,
		secondaryStyle(theme).Render(modelStr),
		mutedStyle(theme).Render(" · "),
		mutedStyle(theme).Render("API Usage Billing"),
	)

	// Working directory
	cwdLine := mutedStyle(theme).Render(shortenPath(w.cwd))

	// Welcome message
	welcomeLine := lipgloss.JoinHorizontal(
		lipgloss.Left,
		successStyle(theme).Render("Welcome to Claude Code Go!"),
		mutedStyle(theme).Render("  "),
		accentStyle(theme).Render("/effort"),
		mutedStyle(theme).Render(" to tune speed vs. intelligence"),
	)

	// Combine info lines vertically
	infoBlock := lipgloss.JoinVertical(
		lipgloss.Left,
		versionLine,
		modelLine,
		cwdLine,
		welcomeLine,
	)

	// Join logo and info horizontally with center alignment
	banner := lipgloss.JoinHorizontal(
		lipgloss.Center,
		logo,
		"  ",
		infoBlock,
	)

	sb.WriteString(banner)
	sb.WriteString("\n")

	return sb.String()
}

// renderLogo renders the ASCII art logo without ears for a cleaner look.
func renderLogo(theme Theme) string {
	// ASCII art - removed the ears (\_/) for cleaner appearance
	logoLines := []string{
		"  (•‿•) ",
		" /|░░░|\\",
		"( |░░░| )",
		"  \"^ ^\" ",
	}

	// Use cyan/teal color for Go Gopher
	gopherColor := lipgloss.Color("#00ADD8") // Go's official cyan color
	logoStyle := lipgloss.NewStyle().
		Foreground(gopherColor).
		Bold(true)

	var rendered []string
	for _, line := range logoLines {
		rendered = append(rendered, logoStyle.Render(line))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rendered...)
}

// shortenPath shortens a path for display, replacing home dir with ~.
func shortenPath(path string) string {
	// This is a simple implementation - could be enhanced to use os.UserHomeDir()
	if strings.HasPrefix(path, "/Users/") {
		parts := strings.SplitN(path, "/", 4)
		if len(parts) >= 4 {
			return "~/" + parts[3]
		}
	}
	if strings.HasPrefix(path, "/home/") {
		parts := strings.SplitN(path, "/", 4)
		if len(parts) >= 4 {
			return "~/" + parts[3]
		}
	}
	return path
}
