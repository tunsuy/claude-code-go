package tui

import (
	"runtime"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// BLACK_CIRCLE is the bullet used for tool indicators.
// On macOS, use the filled circle (⏺); on other platforms, use the standard bullet (●).
func blackCircle() string {
	if runtime.GOOS == "darwin" {
		return "⏺"
	}
	return "●"
}

// ToolStatus represents the execution status of a tool call.
type ToolStatus int

const (
	// ToolStatusQueued means the tool is waiting to be executed.
	ToolStatusQueued ToolStatus = iota
	// ToolStatusInProgress means the tool is currently executing.
	ToolStatusInProgress
	// ToolStatusResolved means the tool has completed successfully.
	ToolStatusResolved
	// ToolStatusError means the tool completed with an error.
	ToolStatusError
)

// MessageLookups contains precomputed indexes for efficient message lookup.
type MessageLookups struct {
	// ToolUseToResult maps tool_use_id to its corresponding tool_result block.
	ToolUseToResult map[string]types.ContentBlock
	// ResolvedToolUseIDs contains IDs of tool_use blocks that have a result.
	ResolvedToolUseIDs map[string]bool
	// ErroredToolUseIDs contains IDs of tool_use blocks that resulted in error.
	ErroredToolUseIDs map[string]bool
	// InProgressToolUseIDs contains IDs of tool_use blocks currently executing.
	InProgressToolUseIDs map[string]bool
}

// buildMessageLookups creates precomputed indexes for message rendering.
// This mirrors the TypeScript buildMessageLookups function.
func buildMessageLookups(messages []types.Message) MessageLookups {
	lookups := MessageLookups{
		ToolUseToResult:      make(map[string]types.ContentBlock),
		ResolvedToolUseIDs:   make(map[string]bool),
		ErroredToolUseIDs:    make(map[string]bool),
		InProgressToolUseIDs: make(map[string]bool),
	}

	// First pass: collect all tool_result blocks from user messages
	for _, msg := range messages {
		if msg.Role != types.RoleUser {
			continue
		}
		for _, blk := range msg.Content {
			if blk.Type == types.ContentTypeToolResult && blk.ToolUseID != nil {
				lookups.ToolUseToResult[*blk.ToolUseID] = blk
				lookups.ResolvedToolUseIDs[*blk.ToolUseID] = true
				if blk.IsError != nil && *blk.IsError {
					lookups.ErroredToolUseIDs[*blk.ToolUseID] = true
				}
			}
		}
	}

	return lookups
}

// newGlamourStyle creates a custom glamour style with no padding/margin
// to avoid extra indentation that interferes with our bullet point layout.
func newGlamourStyle(dark bool) ansi.StyleConfig {
	// Start with a minimal style - no margins or padding
	// This prevents glamour from adding its default 2-space paragraph indent
	baseColor := "252" // light gray for dark mode
	if !dark {
		baseColor = "235" // dark gray for light mode
	}

	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			Margin: uintPtr(0),
		},
		Paragraph: ansi.StyleBlock{
			Margin: uintPtr(0),
		},
		Text: ansi.StylePrimitive{
			Color: stringPtr(baseColor),
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr("203"), // red for inline code
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				Margin: uintPtr(0),
			},
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:  boolPtr(true),
				Color: stringPtr("212"), // pink for headings
			},
		},
		Link: ansi.StylePrimitive{
			Color:     stringPtr("33"),
			Underline: boolPtr(true),
		},
		LinkText: ansi.StylePrimitive{
			Color: stringPtr("33"),
		},
		Emph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
	}
}

func uintPtr(v uint) *uint       { return &v }
func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }

// renderMarkdown converts a Markdown string to an ANSI-colored string
// suitable for terminal display using charmbracelet/glamour.
// Pass a non-nil renderer to reuse a cached instance (P1-C fix).
func renderMarkdown(md string, width int, dark bool, r *glamour.TermRenderer) string {
	if r == nil {
		// Fall back: build a one-shot renderer with custom style (no padding).
		var err error
		r, err = glamour.NewTermRenderer(
			glamour.WithStyles(newGlamourStyle(dark)),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return md
		}
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	// glamour adds leading/trailing newlines (often \n\n at the end);
	// use TrimLeft/TrimRight to remove ALL leading/trailing newlines
	// to avoid blank line accumulation between messages.
	out = strings.TrimLeft(out, "\n")
	out = strings.TrimRight(out, "\n")
	return out
}

// MessageListView renders all messages to a single string.
// Pass a non-nil mdRenderer to reuse a cached glamour renderer (P1-C).
// expandedToolResults tracks which tool results should be shown in full.
func MessageListView(
	messages []types.Message,
	width int,
	darkMode bool,
	theme Theme,
	mdRenderer *glamour.TermRenderer,
	expandedToolResults map[string]bool,
) string {
	// Build message lookups for efficient rendering
	lookups := buildMessageLookups(messages)

	var sb strings.Builder
	for i, msg := range messages {
		sb.WriteString(renderMessage(msg, width, darkMode, theme, mdRenderer, expandedToolResults, lookups))
		// Add a blank line between messages for visual separation,
		// except after the last message.
		if i < len(messages)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// renderMessage dispatches to the appropriate renderer for a single message.
func renderMessage(msg types.Message, width int, darkMode bool, theme Theme, mdRenderer *glamour.TermRenderer, expandedToolResults map[string]bool, lookups MessageLookups) string {
	switch msg.Role {
	case types.RoleUser:
		return renderUserMessage(msg, theme)
	case types.RoleAssistant:
		return renderAssistantMessage(msg, width, darkMode, theme, mdRenderer, expandedToolResults, lookups)
	default:
		return renderSystemMessage(msg, theme)
	}
}

// renderUserMessage renders a user turn.
// Tool_result blocks are skipped here because they are rendered inline with tool_use in assistant messages.
func renderUserMessage(msg types.Message, theme Theme) string {
	var sb strings.Builder
	prefix := primaryStyle(theme).Bold(true).Render("> ")
	for _, blk := range msg.Content {
		switch blk.Type {
		case types.ContentTypeText:
			if blk.Text != nil {
				sb.WriteString(prefix + *blk.Text + "\n")
			}
		case types.ContentTypeToolResult:
			// Skip tool_result blocks - they are now rendered inline with tool_use in assistant messages
			// This avoids duplicate display
			continue
		}
	}
	return sb.String()
}

// renderAssistantMessage renders an assistant turn, including markdown and tool
// use blocks. Tool results are displayed inline after their corresponding tool_use.
// Matches the original TypeScript Claude Code display format.
func renderAssistantMessage(msg types.Message, width int, darkMode bool, theme Theme, mdRenderer *glamour.TermRenderer, expandedToolResults map[string]bool, lookups MessageLookups) string {
	var sb strings.Builder
	for _, blk := range msg.Content {
		switch blk.Type {
		case types.ContentTypeText:
			if blk.Text != nil && *blk.Text != "" {
				// Render text with bullet prefix, same as original Claude Code
				sb.WriteString(renderTextBlock(*blk.Text, width, darkMode, theme, mdRenderer))
			}
		case types.ContentTypeThinking:
			if blk.Thinking != nil && *blk.Thinking != "" {
				sb.WriteString(renderThinkingBlock(*blk.Thinking, theme))
			}
		case types.ContentTypeToolUse:
			sb.WriteString(renderToolUseBlock(blk, theme, lookups))
			// Render the corresponding tool result inline if available
			if blk.ID != nil {
				if result, ok := lookups.ToolUseToResult[*blk.ID]; ok {
					expanded := false
					if expandedToolResults != nil {
						expanded = expandedToolResults[*blk.ID]
					}
					sb.WriteString(renderToolResultBlock(result, theme, expanded))
				}
			}
		}
	}
	// Ensure assistant message ends with a newline for consistent formatting.
	if sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderTextBlock renders a text content block with bullet prefix.
// Format: ● text content (matches original Claude Code style)
func renderTextBlock(text string, width int, darkMode bool, _ Theme, mdRenderer *glamour.TermRenderer) string {
	var sb strings.Builder
	bullet := blackCircle() + " "
	// Render the bullet in primary style
	sb.WriteString(bullet)
	// Render markdown content
	rendered := renderMarkdown(text, width-4, darkMode, mdRenderer)
	// Trim leading/trailing whitespace and normalize line breaks
	rendered = strings.TrimSpace(rendered)
	// Add proper indentation for multi-line content
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if i == 0 {
			sb.WriteString(line)
		} else {
			// Skip consecutive empty lines, keep at most one
			sb.WriteString("\n  " + line)
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// renderSystemMessage renders a system/error message.
func renderSystemMessage(msg types.Message, theme Theme) string {
	var sb strings.Builder
	for _, blk := range msg.Content {
		if blk.Type == types.ContentTypeText && blk.Text != nil {
			sb.WriteString(mutedStyle(theme).Render("System: "+*blk.Text) + "\n")
		}
	}
	return sb.String()
}

// renderThinkingBlock renders a thinking block in a collapsed/muted style.
func renderThinkingBlock(text string, theme Theme) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	// Show only first 3 lines of thinking.
	visible := lines
	truncated := false
	if len(lines) > 3 {
		visible = lines[:3]
		truncated = true
	}
	prefix := mutedStyle(theme).Render("  ▸ ")
	var sb strings.Builder
	sb.WriteString(mutedStyle(theme).Render("Thinking:\n"))
	for _, l := range visible {
		sb.WriteString(prefix + mutedStyle(theme).Render(l) + "\n")
	}
	if truncated {
		sb.WriteString(mutedStyle(theme).Render("  … (truncated)\n"))
	}
	return sb.String()
}

// renderToolUseBlock renders a tool_use content block in Claude Code style.
// Format: ● ToolName(args)
// The bullet color indicates status:
// - Gray/dim: queued or in progress
// - Green: resolved successfully
// - Red: error
func renderToolUseBlock(blk types.ContentBlock, theme Theme, lookups MessageLookups) string {
	var sb strings.Builder
	name := ""
	if blk.Name != nil {
		name = *blk.Name
	}

	toolID := ""
	if blk.ID != nil {
		toolID = *blk.ID
	}

	// Determine tool status for coloring
	isResolved := lookups.ResolvedToolUseIDs[toolID]
	isError := lookups.ErroredToolUseIDs[toolID]

	// Render bullet with appropriate color based on status
	bullet := blackCircle() + " "
	if isResolved {
		if isError {
			// Red bullet for errors
			bullet = errorStyle(theme).Render(blackCircle()) + " "
		} else {
			// Green bullet for success
			bullet = successStyle(theme).Render(blackCircle()) + " "
		}
	} else {
		// Dim bullet for queued/in-progress
		bullet = mutedStyle(theme).Render(blackCircle()) + " "
	}

	sb.WriteString(bullet)
	sb.WriteString(toolNameStyle(theme).Bold(true).Render(name))

	// Show input in parentheses like: Bash(ls -la)
	if len(blk.Input) > 0 {
		summary := summariseToolInput(blk.Input, 60)
		if summary != "" {
			sb.WriteString(mutedStyle(theme).Render("("))
			sb.WriteString(toolInputStyle(theme).Render(summary))
			sb.WriteString(mutedStyle(theme).Render(")"))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// summariseToolInput formats tool input for display.
// For single-key inputs like {command: "ls"}, just show the value.
// For multi-key inputs, show key=value pairs.
func summariseToolInput(input map[string]any, maxLen int) string {
	if len(input) == 0 {
		return ""
	}
	// Common single-value tool inputs
	if len(input) == 1 {
		for k, v := range input {
			// For command-like inputs, just show the value
			if k == "command" || k == "query" || k == "path" || k == "content" {
				s := toString(v)
				if len(s) > maxLen {
					return s[:maxLen] + "…"
				}
				return s
			}
			// Otherwise show key=value
			s := k + "=" + toString(v)
			if len(s) > maxLen {
				return s[:maxLen] + "…"
			}
			return s
		}
	}
	// Multiple keys: show key=value pairs
	parts := make([]string, 0, len(input))
	for k, v := range input {
		s := k + "=" + toString(v)
		if len(s) > 30 {
			s = s[:30] + "…"
		}
		parts = append(parts, s)
	}
	result := strings.Join(parts, ", ")
	if len(result) > maxLen {
		return result[:maxLen] + "…"
	}
	return result
}



func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return itoa(int(t))
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return "<complex>"
	}
}

// renderToolResultBlock renders a tool_result content block in Claude Code style.
// Format matches original TypeScript version:
//
//	│ first line of output
//	│ second line
//	… +N lines (ctrl+o to expand)
//
// Uses vertical bar (│) as left border to match original style.
func renderToolResultBlock(blk types.ContentBlock, theme Theme, expanded bool) string {
	var sb strings.Builder
	isError := blk.IsError != nil && *blk.IsError

	// Vertical bar prefix for tool result (matches original Claude Code)
	barPrefix := mutedStyle(theme).Render("│ ")
	errorBarPrefix := errorStyle(theme).Render("│ ")

	for _, inner := range blk.Content {
		if inner.Type == types.ContentTypeText && inner.Text != nil {
			text := *inner.Text
			if text == "" {
				continue
			}

			lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
			if len(lines) == 0 {
				continue
			}

			// Determine how many lines to show
			maxVisibleLines := 4
			if expanded {
				maxVisibleLines = len(lines)
			}

			// Render visible lines with vertical bar prefix
			for i, line := range lines {
				if i >= maxVisibleLines {
					break
				}

				// Truncate long lines
				displayLine := line
				if len(displayLine) > 120 {
					displayLine = displayLine[:117] + "…"
				}

				if isError {
					sb.WriteString(errorBarPrefix)
					sb.WriteString(errorStyle(theme).Render(displayLine))
				} else {
					sb.WriteString(barPrefix)
					sb.WriteString(mutedStyle(theme).Render(displayLine))
				}
				sb.WriteString("\n")
			}

			// Show truncation indicator if not expanded
			if !expanded && len(lines) > maxVisibleLines {
				remaining := len(lines) - maxVisibleLines
				hint := mutedStyle(theme).Render("… +" + itoa(remaining) + " lines (ctrl+o to expand)")
				sb.WriteString(hint)
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
