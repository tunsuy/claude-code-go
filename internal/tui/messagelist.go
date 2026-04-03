package tui

import (
	"strings"

	"github.com/anthropics/claude-code-go/pkg/types"
	"github.com/charmbracelet/glamour"
)

// renderMarkdown converts a Markdown string to an ANSI-colored string
// suitable for terminal display using charmbracelet/glamour.
func renderMarkdown(md string, width int, dark bool) string {
	style := "dark"
	if !dark {
		style = "light"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return md // fall back to raw markdown
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return out
}

// MessageListView renders all messages to a single string.
func MessageListView(
	messages []types.Message,
	width int,
	darkMode bool,
	theme Theme,
) string {
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(renderMessage(msg, width, darkMode, theme))
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderMessage dispatches to the appropriate renderer for a single message.
func renderMessage(msg types.Message, width int, darkMode bool, theme Theme) string {
	switch msg.Role {
	case types.RoleUser:
		return renderUserMessage(msg, theme)
	case types.RoleAssistant:
		return renderAssistantMessage(msg, width, darkMode, theme)
	default:
		return renderSystemMessage(msg, theme)
	}
}

// renderUserMessage renders a user turn.
func renderUserMessage(msg types.Message, theme Theme) string {
	var sb strings.Builder
	prefix := primaryStyle(theme).Bold(true).Render("You: ")
	for _, blk := range msg.Content {
		switch blk.Type {
		case types.ContentTypeText:
			if blk.Text != nil {
				sb.WriteString(prefix + *blk.Text + "\n")
			}
		case types.ContentTypeToolResult:
			sb.WriteString(renderToolResultBlock(blk, theme))
		}
	}
	return sb.String()
}

// renderAssistantMessage renders an assistant turn, including markdown and tool
// use blocks.
func renderAssistantMessage(msg types.Message, width int, darkMode bool, theme Theme) string {
	var sb strings.Builder
	prefix := secondaryStyle(theme).Bold(true).Render("Claude: ")
	firstText := true
	for _, blk := range msg.Content {
		switch blk.Type {
		case types.ContentTypeText:
			if blk.Text != nil && *blk.Text != "" {
				if firstText {
					sb.WriteString(prefix)
					firstText = false
				}
				sb.WriteString(renderMarkdown(*blk.Text, width-2, darkMode))
			}
		case types.ContentTypeThinking:
			if blk.Thinking != nil && *blk.Thinking != "" {
				sb.WriteString(renderThinkingBlock(*blk.Thinking, theme))
			}
		case types.ContentTypeToolUse:
			sb.WriteString(renderToolUseBlock(blk, theme))
		}
	}
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

// renderToolUseBlock renders a tool_use content block.
func renderToolUseBlock(blk types.ContentBlock, theme Theme) string {
	var sb strings.Builder
	name := ""
	if blk.Name != nil {
		name = *blk.Name
	}
	sb.WriteString(toolNameStyle(theme).Render("⚙ "+name))

	// Show input summary if present.
	if len(blk.Input) > 0 {
		summary := summariseMapInput(blk.Input, 120)
		if summary != "" {
			sb.WriteString(mutedStyle(theme).Render("("))
			sb.WriteString(toolInputStyle(theme).Render(summary))
			sb.WriteString(mutedStyle(theme).Render(")"))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// summariseMapInput converts a map[string]any input to a short display string.
func summariseMapInput(input map[string]any, maxLen int) string {
	if len(input) == 0 {
		return ""
	}
	// If single key, show value directly.
	if len(input) == 1 {
		for _, v := range input {
			s := toString(v)
			if len(s) > maxLen {
				return s[:maxLen] + "…"
			}
			return s
		}
	}
	// Multiple keys: show key=value pairs.
	parts := make([]string, 0, len(input))
	for k, v := range input {
		s := k + "=" + toString(v)
		if len(s) > 40 {
			s = s[:40] + "…"
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

// renderToolResultBlock renders a tool_result content block.
func renderToolResultBlock(blk types.ContentBlock, theme Theme) string {
	var sb strings.Builder
	isError := blk.IsError != nil && *blk.IsError
	for _, inner := range blk.Content {
		if inner.Type == types.ContentTypeText && inner.Text != nil {
			if isError {
				sb.WriteString(errorStyle(theme).Render("✗ Tool error: "+*inner.Text) + "\n")
			} else {
				sb.WriteString(mutedStyle(theme).Render("Tool result: "+truncate(*inner.Text, 200)) + "\n")
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
