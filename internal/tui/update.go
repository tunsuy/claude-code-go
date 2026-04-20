package tui

import (
	"github.com/tunsuy/claude-code-go/internal/commands"
	"github.com/tunsuy/claude-code-go/internal/memdir"
	"github.com/tunsuy/claude-code-go/internal/state"
	tea "github.com/charmbracelet/bubbletea"
)

// Update is the BubbleTea Update method — the single message dispatcher.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// --- Window resize ---
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.input = m.input.SetWidth(msg.Width - 2)
		return m, nil

	// --- Periodic tick (spinner + task eviction) ---
	case TickMsg:
		elapsed := msg.Time.Sub(m.lastTickTime)
		m.lastTickTime = msg.Time
		if m.isLoading {
			m.spinner = m.spinner.Tick(elapsed)
		}
		return m, tickCmd()

	// --- Memdir load complete ---
	case MemdirLoadedMsg:
		// P1-E fix: store paths and eagerly load the concatenated CLAUDE.md content
		// so startQueryCmd can inject it as a system prompt on every query.
		m.memdirPaths = msg.Paths
		m.memdirPrompt = memdir.LoadMemoryPrompt(msg.Paths)
		return m, nil

	// --- Internal: stream channel ready ---
	case streamChanReady:
		m.streamCh = msg.ch
		m.streamingText = ""
		m.streamingHasMsg = false
		return m, waitForStreamEvent(msg.ch)

	// --- Streaming events ---
	// Note: isLoading guards; channel mismatch is prevented by context cancellation.
	case StreamTokenMsg:
		if m.streamCh == nil {
			return m, nil
		}
		m = m.appendStreamDelta(msg.Delta)
		m.streamingHasMsg = true
		return m, waitForStreamEvent(m.streamCh)

	case StreamThinkingMsg:
		if m.streamCh == nil {
			return m, nil
		}
		// Thinking deltas not rendered in streaming view; just continue pulling.
		return m, waitForStreamEvent(m.streamCh)

	case StreamToolUseStartMsg:
		if m.streamCh == nil {
			return m, nil
		}
		return m, waitForStreamEvent(m.streamCh)

	case StreamToolUseInputDeltaMsg:
		if m.streamCh == nil {
			return m, nil
		}
		return m, waitForStreamEvent(m.streamCh)

	case StreamToolUseCompleteMsg:
		if m.streamCh == nil {
			return m, nil
		}
		return m, waitForStreamEvent(m.streamCh)

	case StreamToolResultMsg:
		if m.streamCh == nil {
			return m, nil
		}
		return m, waitForStreamEvent(m.streamCh)

	case StreamDoneMsg:
		m.isLoading = false
		m.showSpinner = false
		m.spinner = m.spinner.Reset()
		m.abortFn = nil
		m.streamCh = nil

		if msg.FinalMessage != nil {
			// FinalMessage replaces any in-progress partial.
			m.messages = append(m.messages, *msg.FinalMessage)
		} else if m.streamingHasMsg && m.streamingText != "" {
			// No explicit final message — promote the streamed text.
			m.messages = append(m.messages, m.inProgressAssistantMessage())
		}
		m.streamingText = ""
		m.streamingHasMsg = false
		m.pinnedToBottom = true
		return m, nil

	case StreamErrorMsg:
		m.isLoading = false
		m.showSpinner = false
		m.spinner = m.spinner.Reset()
		m.abortFn = nil
		m.streamCh = nil
		m.streamingText = ""
		m.streamingHasMsg = false
		errText := "Error: " + msg.Err.Error()
		m.messages = append(m.messages, newSystemMessage(errText))
		return m, nil

	// --- System informational text ---
	case SystemTextMsg:
		m.messages = append(m.messages, newSystemMessage(msg.Text))
		return m, nil

	// --- Permission dialog ---
	case PermissionRequestMsg:
		m.activeDialog = dialogPermission
		d := newPermissionDialog(msg)
		m.permReq = &d
		return m, nil

	// --- Slash command result ---
	case CommandResultMsg:
		m.commandResult = &msg
		if msg.Text != "" {
			m.messages = append(m.messages, newSystemMessage(msg.Text))
		}
		return m, nil

	// --- Agent status (coordinator panel) ---
	case AgentStatusMsg:
		task := m.coordinatorPanel.Tasks[msg.TaskID]
		task.Status = msg.Status
		m.coordinatorPanel.Tasks[msg.TaskID] = task
		return m, nil

	// --- Compact done ---
	case CompactDoneMsg:
		m.messages = append(m.messages, newSystemMessage("Conversation compacted: "+msg.Summary))
		m.activeDialog = dialogNone
		return m, nil

	// --- Keyboard / input ---
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleSlashCommand executes a slash command and returns updated model + cmd.
func (m AppModel) handleSlashCommand(text string) (AppModel, tea.Cmd) {
	name, args := parseSlashInput(text)
	if name == "" {
		return m, nil
	}

	slashCmd := m.commandRegistry.Lookup(name)
	if slashCmd == nil {
		m.messages = append(m.messages, newSystemMessage("Unknown command: /"+name))
		return m, nil
	}

	st := m.appState.GetState()
	cmdCtx := commands.CommandContext{
		WorkingDir:   st.WorkingDir,
		Model:        st.MainLoopModel.ModelID,
		SessionID:    m.sessionID,
		DarkMode:     m.darkMode,
		VimEnabled:   m.input.vimEnabled,
		MessageCount: len(m.messages),
	}

	result := slashCmd.Execute(cmdCtx, args)
	return m.applyCommandResult(result, name)
}

// applyCommandResult applies a commands.Result to the model.
func (m AppModel) applyCommandResult(result commands.Result, name string) (AppModel, tea.Cmd) {
	if result.ShouldExit {
		return m, tea.Quit
	}

	if result.NewTheme != "" {
		if t, ok := BuiltinThemes[result.NewTheme]; ok {
			m.theme = t
			m.darkMode = result.NewTheme != "light"
		} else {
			m.messages = append(m.messages, newSystemMessage("Unknown theme: "+result.NewTheme))
			return m, nil
		}
	}

	if result.ToggleVim {
		m.input.vimEnabled = !m.input.vimEnabled
	}

	if result.NewModel != "" {
		m.queryEngine.SetModel(result.NewModel)
		m.statusBar.model = result.NewModel
		m.appState.SetState(func(prev state.AppState) state.AppState {
			prev.MainLoopModel.ModelID = result.NewModel
			return prev
		})
	}

	// P1-B fix: use semantic OpenDialog field instead of magic name=="compact" string.
	if result.OpenDialog != "" {
		switch result.OpenDialog {
		case "compact":
			m.activeDialog = dialogCompact
		case "exit":
			m.activeDialog = dialogExit
		case "config":
			m.activeDialog = dialogConfig
		}
		return m, nil
	}

	// P1-B fix: use ClearHistory field instead of magic name=="clear" string.
	if result.ClearHistory {
		m.messages = nil
		m.queryEngine.SetMessages(nil)
		m.streamingText = ""
		m.streamingHasMsg = false
	}

	switch result.Display {
	case commands.DisplayMessage:
		if result.Text != "" {
			m.messages = append(m.messages, newSystemMessage(result.Text))
		}
	case commands.DisplayError:
		if result.Text != "" {
			m.messages = append(m.messages, newSystemMessage("Error: "+result.Text))
		}
	case commands.DisplayNone:
		// If there is a text payload it becomes the user query (e.g. /review, /commit).
		if result.Text != "" {
			m.isLoading = true
			m.showSpinner = true
			m.spinner = m.spinner.Reset()
			queryCmd := startQueryCmd(&m, result.Text)
			return m, queryCmd
		}
	}

	return m, nil
}
