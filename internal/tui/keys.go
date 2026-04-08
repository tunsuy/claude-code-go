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
func (m AppModel) handlePermissionKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.permReq == nil {
		m.activeDialog = dialogNone
		return m, nil
	}
	switch key.Type {
	case tea.KeyUp:
		d := m.permReq.Up()
		m.permReq = &d
	case tea.KeyDown:
		d := m.permReq.Down()
		m.permReq = &d
	case tea.KeyEnter:
		allow := m.permReq.Confirm()
		if m.permReq.respFn != nil {
			m.permReq.respFn(allow)
		}
		m.permReq = nil
		m.activeDialog = dialogNone
	case tea.KeyEsc:
		// Esc = deny.
		if m.permReq.respFn != nil {
			m.permReq.respFn(false)
		}
		m.permReq = nil
		m.activeDialog = dialogNone
	}
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
		if m.scrollOffset+10 < 1000 {
			m.scrollOffset += 10
		}
		return m, nil

	case tea.KeyPgDown:
		if m.scrollOffset > 10 {
			m.scrollOffset -= 10
		} else {
			m.scrollOffset = 0
			m.pinnedToBottom = true
		}
		return m, nil
	}

	// Vim normal mode uses j/k for navigation when vim is enabled.
	if m.input.vimEnabled && m.input.vimMode == VimModeNormal {
		switch key.String() {
		case "k":
			m.pinnedToBottom = false
			m.scrollOffset += 3
			return m, nil
		case "j":
			if m.scrollOffset > 3 {
				m.scrollOffset -= 3
			} else {
				m.scrollOffset = 0
				m.pinnedToBottom = true
			}
			return m, nil
		}
	}

	// Delegate to input model.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	return m, cmd
}

// handleSubmit is called when the user presses Enter in the input box.
func (m AppModel) handleSubmit() (tea.Model, tea.Cmd) {
	text := m.input.Value()
	if text == "" {
		return m, nil
	}

	// Clear input.
	m.input = m.input.SetValue("")
	m.pinnedToBottom = true

	// Check for slash commands.
	if IsSlashCommand(text) {
		return m.handleSlashCommand(text)
	}

	// Append user message to local display.
	m.messages = append(m.messages, newUserMessage(text))

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
