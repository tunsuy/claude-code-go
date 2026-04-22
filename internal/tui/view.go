package tui

import (
	"strings"
)

// reservedLines is the number of terminal lines reserved for non-message UI
// (status bar + input area + spinner + padding).
const reservedLines = 6

// viewportHeight returns the available height for the message viewport.
func (m AppModel) viewportHeight() int {
	h := m.termHeight - reservedLines
	if h < 1 {
		h = 1
	}
	return h
}

// View is the BubbleTea View method. It renders the entire TUI to a string.
func (m AppModel) View() string {
	if m.termWidth == 0 {
		return ""
	}

	var sb strings.Builder

	// --- Coordinator panel (multi-agent) ---
	if m.coordinatorMode {
		panel := m.coordinatorPanel.View(m.termWidth, m.theme)
		if panel != "" {
			sb.WriteString(panel)
			sb.WriteString("\n")
		}
	}

	// --- Message list rendered through viewport ---
	// Only show viewport area when there is content to display;
	// otherwise avoid filling the terminal with blank lines.
	vpContent := strings.TrimRight(m.viewport.View(), "\n ")
	if vpContent != "" {
		sb.WriteString(vpContent)
		sb.WriteString("\n")
	}

	// --- Spinner / loading indicator ---
	if m.isLoading && m.showSpinner {
		sb.WriteString(m.spinner.View(m.theme))
		sb.WriteString("\n")
	}

	// --- Status bar ---
	sb.WriteString(m.statusBar.View(m.termWidth, m.theme))
	sb.WriteString("\n")

	// --- Input area ---
	sb.WriteString(m.input.View(m.theme))

	// --- Active dialogs (overlay) ---
	switch m.activeDialog {
	case dialogPermission:
		if m.permReq != nil {
			sb.WriteString("\n")
			sb.WriteString(m.permReq.View(m.termWidth, m.theme))
		}
	case dialogCompact:
		sb.WriteString("\n")
		sb.WriteString(renderConfirmDialog(
			"Compact conversation?",
			"This will summarise the history and reduce context usage.",
			m.termWidth, m.theme,
		))
	case dialogExit:
		sb.WriteString("\n")
		sb.WriteString(renderConfirmDialog(
			"Exit Claude Code?",
			"Press y to confirm, n to cancel.",
			m.termWidth, m.theme,
		))
	}

	return sb.String()
}

// syncViewportContent re-renders the message list into the viewport content
// and optionally scrolls to bottom when pinnedToBottom is true.
// The welcome header is included as part of the content so it scrolls with messages.
func (m *AppModel) syncViewportContent() {
	var contentBuilder strings.Builder

	// Always include welcome header at the top of the content
	header := m.welcomeHeader.View(m.termWidth, m.theme)
	if header != "" {
		contentBuilder.WriteString(header)
	}

	// Render messages
	msgs := m.visibleMessages()
	msgContent := MessageListView(msgs, m.termWidth, m.darkMode, m.theme, m.markdownRenderer(), m.expandedToolResults)
	contentBuilder.WriteString(msgContent)

	// Trim trailing newlines to avoid excessive blank space at bottom.
	content := strings.TrimRight(contentBuilder.String(), "\n")
	m.viewport.SetContent(content)
	if m.pinnedToBottom {
		m.viewport.GotoBottom()
	}
}

// renderConfirmDialog renders a minimal yes/no confirmation dialog.
func renderConfirmDialog(title, body string, width int, theme Theme) string {
	var sb strings.Builder
	sb.WriteString(warningStyle(theme).Bold(true).Render(title))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle(theme).Render(body))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle(theme).Render("(y/n)"))
	_ = width
	return sb.String()
}
