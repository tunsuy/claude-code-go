package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// handleKey dispatches keyboard events based on the active dialog and loading state.
func (m AppModel) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// --- Global shortcuts that work in all states ---
	switch key.Type {
	case tea.KeyCtrlC:
		if m.isLoading {
			return m.doAbort()
		}
		return m, tea.Quit
	case tea.KeyCtrlD:
		return m, tea.Quit
	case tea.KeyCtrlO:
		// Toggle all tool results expansion
		return m.toggleAllToolResults()
	}

	// --- Route to the active dialog ---
	switch m.activeDialog {
	case dialogPermission:
		return m.handlePermissionKey(key)
	case dialogCompact:
		return m.handleCompactKey(key)
	case dialogExit:
		return m.handleExitKey(key)
	case dialogConfig:
		if key.Type == tea.KeyEsc {
			m.activeDialog = dialogNone
		}
		return m, nil
	}

	// --- Normal / idle state ---
	return m.handleIdleKey(key)
}

// handlePermissionKey handles keys while the permission dialog is open.
// Supports: arrow keys (Up/Down), j/k (Vim), number keys (1/2/3), y/n shortcuts,
// Enter to confirm, Esc to cancel.
func (m AppModel) handlePermissionKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.permReq == nil {
		m.activeDialog = dialogNone
		return m, nil
	}

	// Use key.String() for unified matching — works reliably across terminal
	// emulators regardless of how arrow keys are encoded (ANSI vs raw).
	// Note: some terminals send LF (\n, "ctrl+j") instead of CR (\r, "enter")
	// for the Enter key, so we must match both.
	switch key.String() {
	case "up":
		d := m.permReq.Up()
		m.permReq = &d
		return m, nil
	case "down":
		d := m.permReq.Down()
		m.permReq = &d
		return m, nil
	case "enter", "ctrl+j":
		return m.confirmPermission()
	case "esc":
		return m.denyPermission()
	}

	// Also check key.Type as fallback.
	switch key.Type {
	case tea.KeyUp:
		d := m.permReq.Up()
		m.permReq = &d
		return m, nil
	case tea.KeyDown:
		d := m.permReq.Down()
		m.permReq = &d
		return m, nil
	case tea.KeyEnter:
		return m.confirmPermission()
	case tea.KeyEsc:
		return m.denyPermission()
	}

	// Check character keys (number selection, j/k navigation, y/n shortcuts).
	switch key.String() {
	case "k", "K":
		d := m.permReq.Up()
		m.permReq = &d
	case "j", "J":
		d := m.permReq.Down()
		m.permReq = &d
	case "1":
		// Select "Yes" (index 0) and confirm.
		m.permReq.cursor = 0
		return m.confirmPermission()
	case "2":
		// Select "Always allow" (index 1) and confirm.
		m.permReq.cursor = 1
		return m.confirmPermission()
	case "3":
		// Select "No" (index 2) and confirm.
		m.permReq.cursor = 2
		return m.confirmPermission()
	case "y", "Y":
		// Quick accept (same as "1").
		m.permReq.cursor = 0
		return m.confirmPermission()
	case "n", "N":
		// Quick deny (same as Esc).
		return m.denyPermission()
	}
	return m, nil
}

// confirmPermission confirms the current permission selection.
func (m AppModel) confirmPermission() (tea.Model, tea.Cmd) {
	allow := m.permReq.Confirm()
	if m.permReq.respFn != nil {
		m.permReq.respFn(allow)
	}
	m.permReq = nil
	m.activeDialog = dialogNone
	return m, nil
}

// denyPermission denies the permission request.
func (m AppModel) denyPermission() (tea.Model, tea.Cmd) {
	if m.permReq.respFn != nil {
		m.permReq.respFn(false)
	}
	m.permReq = nil
	m.activeDialog = dialogNone
	return m, nil
}

// handleCompactKey handles keys while the compact confirmation dialog is open.
func (m AppModel) handleCompactKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.String() == "y" || key.String() == "Y":
		m.activeDialog = dialogNone
		// Trigger compact: clear messages and notify engine.
		m.messages = nil
		m.queryEngine.SetMessages(nil)
		return m, func() tea.Msg {
			return CompactDoneMsg{Summary: "Conversation history cleared."}
		}
	case key.String() == "n" || key.String() == "N" || key.Type == tea.KeyEsc:
		m.activeDialog = dialogNone
	}
	return m, nil
}

// handleExitKey handles keys in the exit confirmation dialog.
func (m AppModel) handleExitKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.String() == "y" || key.String() == "Y":
		return m, tea.Quit
	case key.String() == "n" || key.String() == "N" || key.Type == tea.KeyEsc:
		m.activeDialog = dialogNone
	}
	return m, nil
}

// handleIdleKey handles keys when no dialog is open.
func (m AppModel) handleIdleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		if m.isLoading {
			return m.doAbort()
		}
		return m, nil

	case tea.KeyEnter:
		if key.Alt {
			// Alt-Enter inserts a newline; delegate to input.
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(key)
			return m, cmd
		}
		return m.handleSubmit()

	case tea.KeyTab:
		return m.handleTabCompletion()

	case tea.KeyPgUp:
		m.pinnedToBottom = false
		m.viewport.HalfViewUp()
		return m, nil

	case tea.KeyPgDown:
		m.viewport.HalfViewDown()
		if m.viewport.AtBottom() {
			m.pinnedToBottom = true
		}
		return m, nil

	case tea.KeyUp:
		m.pinnedToBottom = false
		m.viewport.LineUp(3)
		return m, nil

	case tea.KeyDown:
		m.viewport.LineDown(3)
		if m.viewport.AtBottom() {
			m.pinnedToBottom = true
		}
		return m, nil
	}

	// Vim normal mode uses j/k for navigation when vim is enabled.
	if m.input.vimEnabled && m.input.vimMode == VimModeNormal {
		switch key.String() {
		case "k":
			m.pinnedToBottom = false
			m.viewport.LineUp(3)
			return m, nil
		case "j":
			m.viewport.LineDown(3)
			if m.viewport.AtBottom() {
				m.pinnedToBottom = true
			}
			return m, nil
		case "g":
			m.pinnedToBottom = false
			m.viewport.GotoTop()
			return m, nil
		case "G":
			m.pinnedToBottom = true
			m.viewport.GotoBottom()
			return m, nil
		}
	}

	// Delegate to input model.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	return m, cmd
}

// handleMouse handles mouse events (scroll wheel).
func (m AppModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Action {
	case tea.MouseActionPress:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.pinnedToBottom = false
			m.viewport.LineUp(3)
		case tea.MouseButtonWheelDown:
			m.viewport.LineDown(3)
			if m.viewport.AtBottom() {
				m.pinnedToBottom = true
			}
		}
	}
	return m, nil
}

// handleSubmit is called when the user presses Enter in the input box.
func (m AppModel) handleSubmit() (tea.Model, tea.Cmd) {
	text := m.input.Value()
	if text == "" {
		return m, nil
	}

	// Keep welcome header visible (always show)

	// Clear input.
	m.input = m.input.SetValue("")
	m.pinnedToBottom = true

	// Check for slash commands.
	if IsSlashCommand(text) {
		return m.handleSlashCommand(text)
	}

	// Append user message to local display.
	m.messages = append(m.messages, newUserMessage(text))
	m.syncViewportContent()

	// Normal query.
	m.isLoading = true
	m.showSpinner = true
	m.spinner = m.spinner.Reset()
	queryCmd := startQueryCmd(&m, text)
	return m, queryCmd
}

// handleTabCompletion cycles through slash command completions.
func (m AppModel) handleTabCompletion() (tea.Model, tea.Cmd) {
	prefix := m.input.SlashPrefix()
	if prefix == "" {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyTab})
		return m, cmd
	}
	// Find completions.
	matches := m.commandRegistry.CompletePrefix(prefix)
	if len(matches) == 0 {
		return m, nil
	}
	// Autocomplete to the first match.
	m.input = m.input.SetValue("/" + matches[0].Name + " ")
	return m, nil
}

// doAbort cancels the in-flight query.
func (m AppModel) doAbort() (tea.Model, tea.Cmd) {
	abortCmd := abortQueryCmd(m.queryEngine)
	m.isLoading = false
	m.showSpinner = false
	m.spinner = m.spinner.Reset()
	// P1-D fix: clear abort handle and stream channel so stale events are ignored.
	m.abortFn = nil
	m.streamCh = nil
	return m, abortCmd
}

// toggleAllToolResults toggles the expansion state of all tool results.
// If any are collapsed, expand all; if all are expanded, collapse all.
func (m AppModel) toggleAllToolResults() (tea.Model, tea.Cmd) {
	// Collect all tool use IDs from messages
	toolIDs := m.collectToolUseIDs()
	if len(toolIDs) == 0 {
		return m, nil
	}

	// Check if any are currently collapsed
	anyCollapsed := false
	for _, id := range toolIDs {
		if !m.expandedToolResults[id] {
			anyCollapsed = true
			break
		}
	}

	// Toggle: if any collapsed, expand all; otherwise collapse all
	if anyCollapsed {
		for _, id := range toolIDs {
			m.expandedToolResults[id] = true
		}
	} else {
		for _, id := range toolIDs {
			m.expandedToolResults[id] = false
		}
	}

	m.syncViewportContent()
	return m, nil
}

// collectToolUseIDs returns all tool use IDs from the message history.
// We collect from tool_use blocks in assistant messages (not tool_result in user messages)
// because tool_use.ID is the primary key for tracking expansion state.
func (m AppModel) collectToolUseIDs() []string {
	var ids []string
	for _, msg := range m.messages {
		for _, blk := range msg.Content {
			if blk.Type == "tool_use" && blk.ID != nil {
				ids = append(ids, *blk.ID)
			}
		}
	}
	return ids
}
