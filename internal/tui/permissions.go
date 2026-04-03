package tui

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PermissionDialog is the modal dialog shown when a tool needs user confirmation.
type PermissionDialog struct {
	toolName  string
	toolUseID string
	message   string
	input     string // raw JSON input
	options   []string
	cursor    int
	respFn    func(allow bool)
}

// newPermissionDialog creates a PermissionDialog from a PermissionRequestMsg.
func newPermissionDialog(msg PermissionRequestMsg) PermissionDialog {
	return PermissionDialog{
		toolName:  msg.ToolName,
		toolUseID: msg.ToolUseID,
		message:   msg.Message,
		input:     msg.Input,
		options:   []string{"Yes, allow once", "No, deny"},
		cursor:    0,
		respFn:    msg.RespFn,
	}
}

// Up moves the cursor up.
func (d PermissionDialog) Up() PermissionDialog {
	if d.cursor > 0 {
		d.cursor--
	}
	return d
}

// Down moves the cursor down.
func (d PermissionDialog) Down() PermissionDialog {
	if d.cursor < len(d.options)-1 {
		d.cursor++
	}
	return d
}

// Confirm returns the decision at the current cursor position.
func (d PermissionDialog) Confirm() bool {
	return d.cursor == 0 // "Yes, allow once"
}

// View renders the permission dialog.
func (d PermissionDialog) View(width int, theme Theme) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Warning).
		Padding(0, 1)

	var sb strings.Builder

	// Title
	sb.WriteString(warningStyle(theme).Bold(true).Render("Permission Required"))
	sb.WriteString("\n\n")

	// Tool name
	sb.WriteString(mutedStyle(theme).Render("Tool: "))
	sb.WriteString(toolNameStyle(theme).Render(d.toolName))
	sb.WriteString("\n")

	// Message (if any)
	if d.message != "" {
		sb.WriteString(mutedStyle(theme).Render("Reason: "))
		sb.WriteString(d.message)
		sb.WriteString("\n")
	}

	// Input summary
	inputSummary := summariseInput(d.input, 200)
	if inputSummary != "" {
		sb.WriteString("\n")
		sb.WriteString(mutedStyle(theme).Render("Input:\n"))
		sb.WriteString(codeStyle(theme).Render(inputSummary))
		sb.WriteString("\n")
	}

	// Options
	sb.WriteString("\n")
	for i, opt := range d.options {
		if i == d.cursor {
			sb.WriteString(primaryStyle(theme).Bold(true).Render("▶ " + opt))
		} else {
			sb.WriteString(mutedStyle(theme).Render("  " + opt))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(mutedStyle(theme).Render("↑↓ navigate · Enter confirm · Esc deny"))

	inner := sb.String()
	maxW := width - 4
	if maxW < 20 {
		maxW = 20
	}
	return border.Width(maxW).Render(inner)
}

// summariseInput returns a truncated JSON summary.
func summariseInput(raw string, maxLen int) string {
	if raw == "" {
		return ""
	}
	// Pretty-print if valid JSON, otherwise use raw.
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err == nil {
		if b, err := json.MarshalIndent(v, "", "  "); err == nil {
			raw = string(b)
		}
	}
	if len(raw) > maxLen {
		return raw[:maxLen] + "…"
	}
	return raw
}
