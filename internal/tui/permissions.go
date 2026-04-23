package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PermissionChoice represents the user's decision in the permission dialog.
type PermissionChoice int

const (
	PermissionChoiceYes         PermissionChoice = iota // Allow once
	PermissionChoiceAlwaysAllow                         // Always allow for this project
	PermissionChoiceNo                                  // Deny
)

// PermissionDialog is the modal dialog shown when a tool needs user confirmation.
type PermissionDialog struct {
	toolName    string
	toolUseID   string
	message     string
	input       string // raw JSON input
	projectPath string // project path for "always allow" option
	options     []string
	cursor      int
	respFn      func(allow bool)
}

// newPermissionDialog creates a PermissionDialog from a PermissionRequestMsg.
func newPermissionDialog(msg PermissionRequestMsg) PermissionDialog {
	// Extract project name from path for display
	projectName := filepath.Base(msg.ProjectPath)
	if projectName == "" || projectName == "." {
		projectName = "this project"
	}

	return PermissionDialog{
		toolName:    msg.ToolName,
		toolUseID:   msg.ToolUseID,
		message:     msg.Message,
		input:       msg.Input,
		projectPath: msg.ProjectPath,
		options: []string{
			"Yes",
			fmt.Sprintf("Yes, and always allow access to %s/ from this project", projectName),
			"No",
		},
		cursor: 0,
		respFn: msg.RespFn,
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

// Choice returns the permission choice at the current cursor position.
func (d PermissionDialog) Choice() PermissionChoice {
	switch d.cursor {
	case 0:
		return PermissionChoiceYes
	case 1:
		return PermissionChoiceAlwaysAllow
	default:
		return PermissionChoiceNo
	}
}

// Confirm returns true if the user chose Yes or AlwaysAllow.
func (d PermissionDialog) Confirm() bool {
	return d.cursor == 0 || d.cursor == 1
}

// View renders the permission dialog matching the original Claude Code style.
func (d PermissionDialog) View(width int, theme Theme) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Warning).
		Padding(0, 1)

	var sb strings.Builder

	// Title: "Bash command" or tool name with command style
	title := formatToolTitle(d.toolName)
	sb.WriteString(warningStyle(theme).Bold(true).Render(title))
	sb.WriteString("\n\n")

	// Command/Input display (indented)
	inputDisplay := formatInputForDisplay(d.toolName, d.input)
	if inputDisplay != "" {
		sb.WriteString("  ")
		sb.WriteString(inputDisplay)
		sb.WriteString("\n")
	}

	// Message/Description (if any)
	if d.message != "" {
		sb.WriteString("  ")
		sb.WriteString(mutedStyle(theme).Render(d.message))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Question prompt
	sb.WriteString("Do you want to proceed?\n")

	// Options with numbered prefix (matching original style)
	for i, opt := range d.options {
		prefix := fmt.Sprintf("%d. ", i+1)
		if i == d.cursor {
			sb.WriteString(primaryStyle(theme).Bold(true).Render("❯ " + prefix + opt))
		} else {
			sb.WriteString("  " + prefix + opt)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	// Footer hints (matching original: Esc to cancel · Tab to amend · ctrl+e to explain)
	sb.WriteString(mutedStyle(theme).Render("Esc to cancel · Tab to amend · ctrl+e to explain"))

	inner := sb.String()
	maxW := width - 4
	if maxW < 20 {
		maxW = 20
	}
	return border.Width(maxW).Render(inner)
}

// formatToolTitle returns a human-readable title for the tool.
func formatToolTitle(toolName string) string {
	switch strings.ToLower(toolName) {
	case "bash", "shell":
		return "Bash command"
	case "write", "file_write":
		return "Write file"
	case "edit", "file_edit":
		return "Edit file"
	case "read", "file_read":
		return "Read file"
	default:
		return toolName + " command"
	}
}

// formatInputForDisplay formats the tool input for display.
func formatInputForDisplay(toolName string, input string) string {
	if input == "" {
		return ""
	}

	// Try to parse as JSON and extract relevant fields
	var data map[string]any
	if err := json.Unmarshal([]byte(input), &data); err != nil {
		return input
	}

	switch strings.ToLower(toolName) {
	case "bash", "shell":
		if cmd, ok := data["command"].(string); ok {
			return cmd
		}
	case "write", "file_write":
		if path, ok := data["file_path"].(string); ok {
			return path
		}
	case "edit", "file_edit":
		if path, ok := data["file_path"].(string); ok {
			return path
		}
	case "read", "file_read":
		if path, ok := data["file_path"].(string); ok {
			return path
		}
	}

	// Fallback: return truncated JSON
	return summariseInput(input, 100)
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
