package tui

import (
	"strings"
)

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

	// --- Message list ---
	msgs := m.visibleMessages()
	msgView := MessageListView(msgs, m.termWidth, m.darkMode, m.theme, m.markdownRenderer())

	// Apply scroll offset if not pinned to bottom.
	if m.pinnedToBottom || m.scrollOffset == 0 {
		sb.WriteString(msgView)
	} else {
		lines := strings.Split(msgView, "\n")
		start := 0
		if m.scrollOffset < len(lines) {
			start = len(lines) - m.scrollOffset
			if start < 0 {
				start = 0
			}
		}
		// P1-A fix: clamp the visible window to the available viewport height so
		// the message list never overflows the terminal.
		const reservedLines = 5 // status bar (1) + input (2) + spinner (1) + padding (1)
		viewH := m.termHeight - reservedLines
		if viewH < 1 {
			viewH = 1
		}
		end := start + viewH
		if end > len(lines) {
			end = len(lines)
		}
		visible := lines[start:end]
		sb.WriteString(strings.Join(visible, "\n"))
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
			// The dialog is rendered as an overlay by appending after the main view.
			// In a real Lipgloss overlay this would be layered; here we suffix it.
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
