package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// lipgloss style helpers — all created dynamically from Theme to avoid
// global mutable Style instances.

func primaryStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Primary)
}

func secondaryStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Secondary)
}

func accentStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Accent)
}

func mutedStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Muted)
}

func errorStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Error)
}

func warningStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Warning)
}

func successStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.Success)
}

func toolNameStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.ToolName).Bold(true)
}

func toolInputStyle(t Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.ToolInput)
}
