package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tunsuy/claude-code-go/internal/commands"
	"github.com/tunsuy/claude-code-go/internal/memdir"
	"github.com/tunsuy/claude-code-go/internal/msgqueue"
	"github.com/tunsuy/claude-code-go/internal/state"
)

// Update is the BubbleTea Update method — the single message dispatcher.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// --- Window resize ---
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		m.input = m.input.SetWidth(msg.Width - 2)
		// Resize the viewport to fill available space.
		m.viewport.Width = msg.Width
		m.viewport.Height = m.viewportHeight()
		m.syncViewportContent()
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
		m.memdirPaths = msg.Paths
		if len(msg.ScopedFiles) > 0 {
			m.memdirPrompt = memdir.LoadScopedMemoryPrompt(msg.ScopedFiles)
		} else {
			m.memdirPrompt = memdir.LoadMemoryPrompt(msg.Paths)
		}
		// Initialize memory store for relevant memory surfacing.
		if m.memoryStore == nil {
			workDir := m.appState.GetState().WorkingDir
			if ms, err := memdir.NewMemoryStore(workDir); err == nil {
				m.memoryStore = ms
			}
		}
		return m, nil

	// --- Internal: stream channel ready ---
	case streamChanReady:
		m.streamCh = msg.ch
		m.streamingText = ""
		m.streamingHasMsg = false
		m.queryGen = msg.gen
		return m, waitForStreamEvent(msg.ch)

	// --- Streaming events ---
	case StreamTokenMsg:
		if m.streamCh == nil {
			return m, nil
		}
		m = m.appendStreamDelta(msg.Delta)
		m.streamingHasMsg = true
		m.syncViewportContent()
		return m, waitForStreamEvent(m.streamCh)

	case StreamThinkingMsg:
		if m.streamCh == nil {
			return m, nil
		}
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

	case StreamAssistantTurnMsg:
		if m.streamCh == nil {
			return m, nil
		}
		if msg.FinalMessage != nil {
			m.messages = append(m.messages, *msg.FinalMessage)
		}
		m.streamingText = ""
		m.streamingHasMsg = false
		m.syncViewportContent()
		return m, waitForStreamEvent(m.streamCh)

	case StreamUserTurnMsg:
		if m.streamCh == nil {
			return m, nil
		}
		if msg.FinalMessage != nil {
			m.messages = append(m.messages, *msg.FinalMessage)
		}
		m.syncViewportContent()
		return m, waitForStreamEvent(m.streamCh)

	case StreamDoneMsg:
		m.isLoading = false
		m.showSpinner = false
		m.spinner = m.spinner.Reset()
		m.abortFn = nil
		m.streamCh = nil

		if msg.FinalMessage != nil {
			m.messages = append(m.messages, *msg.FinalMessage)
		} else if m.streamingHasMsg && m.streamingText != "" {
			m.messages = append(m.messages, m.inProgressAssistantMessage())
		}
		m.streamingText = ""
		m.streamingHasMsg = false
		m.pinnedToBottom = true
		m.syncViewportContent()

		// P1: End the query guard (best-effort; stale gen is harmless).
		if m.queryGuard != nil {
			_ = m.queryGuard.End(m.queryGen)
		}

		// P1: Between-turn drain — process queued commands now that the query finished.
		if m.msgQueue != nil && m.msgQueue.Len() > 0 {
			return m, processQueueCmd(&m)
		}
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
		m.syncViewportContent()
		return m, nil

	// --- System informational text ---
	case SystemTextMsg:
		m.messages = append(m.messages, newSystemMessage(msg.Text))
		m.syncViewportContent()
		return m, nil

	// --- Permission dialog ---
	case PermissionRequestMsg:
		m.activeDialog = dialogPermission
		d := newPermissionDialog(msg)
		m.permReq = &d
		// Continue listening for the next permission request.
		if m.permAskCh != nil {
			return m, listenForPermissionRequest(m.permAskCh, m.permRespCh)
		}
		return m, nil

	// --- Slash command result ---
	case CommandResultMsg:
		m.commandResult = &msg
		if msg.Text != "" {
			m.messages = append(m.messages, newSystemMessage(msg.Text))
			m.syncViewportContent()
		}
		return m, nil

	// --- Agent status (coordinator panel) ---
	case AgentStatusMsg:
		task, exists := m.coordinatorPanel.Tasks[msg.TaskID]
		if !exists {
			// First time seeing this agent — register it.
			task = AgentTaskState{
				ID:        msg.TaskID,
				Name:      msg.Description,
				StartTime: time.Now(),
			}
			m.coordinatorPanel.TaskOrder = append(m.coordinatorPanel.TaskOrder, msg.TaskID)
		}
		task.Status = msg.Status
		if msg.Description != "" {
			task.Name = msg.Description
			task.Description = msg.Description
		}
		// When agent completes/fails, set eviction timer and clear activity.
		if msg.Status == AgentCompleted || msg.Status == AgentFailed {
			evict := time.Now().Add(agentEvictDelay)
			task.EvictAfter = &evict
			task.ElapsedMs = time.Since(task.StartTime).Milliseconds()
			task.Activity = ""
			task.Detail = ""
		}
		m.coordinatorPanel.Tasks[msg.TaskID] = task
		// Continue listening for agent events.
		if m.agentEventCh != nil {
			return m, listenForAgentEvent(m.agentEventCh)
		}
		return m, nil

	// --- Agent progress (coordinator panel) ---
	case AgentProgressMsg:
		task, exists := m.coordinatorPanel.Tasks[msg.TaskID]
		if !exists {
			// Agent not yet registered — create a placeholder entry.
			task = AgentTaskState{
				ID:        msg.TaskID,
				Name:      msg.TaskID,
				Status:    AgentRunning,
				StartTime: time.Now(),
			}
			m.coordinatorPanel.TaskOrder = append(m.coordinatorPanel.TaskOrder, msg.TaskID)
		}
		task.Activity = msg.Activity
		task.Detail = msg.Detail
		task.ElapsedMs = time.Since(task.StartTime).Milliseconds()
		m.coordinatorPanel.Tasks[msg.TaskID] = task
		// Continue listening for agent events.
		if m.agentEventCh != nil {
			return m, listenForAgentEvent(m.agentEventCh)
		}
		return m, nil

	// --- Compact done ---
	case CompactDoneMsg:
		m.messages = append(m.messages, newSystemMessage("Conversation compacted: "+msg.Summary))
		m.activeDialog = dialogNone
		m.syncViewportContent()
		return m, nil

	// --- Mid-session message queue events ---
	case queueChangedMsg:
		var cmds []tea.Cmd
		// Re-subscribe for future queue changes.
		cmds = append(cmds, listenForQueueChange(m.msgQueue))
		// If idle and queue has commands, drain.
		if !m.isLoading && m.msgQueue != nil && m.msgQueue.Len() > 0 && m.activeDialog == dialogNone {
			cmds = append(cmds, processQueueCmd(&m))
		}
		return m, tea.Batch(cmds...)

	case queuedSlashMsg:
		return m.handleSlashCommand(msg.Text)

	// --- Mouse events (scroll wheel) ---
	case tea.MouseMsg:
		return m.handleMouse(msg)

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
		m.syncViewportContent()
		return m, nil
	}

	st := m.appState.GetState()
	cmdCtx := commands.CommandContext{
		WorkingDir:   st.WorkingDir,
		Model:        st.MainLoopModel.ModelID,
		SessionID:    m.sessionID,
		DarkMode:     m.darkMode,
		VimEnabled:   m.input.vimEnabled,
		Effort:       m.effort,
		MessageCount: len(m.messages),
	}

	result := slashCmd.Execute(cmdCtx, args)
	return m.applyCommandResult(result, name)
}

// processQueueCmd dequeues the next command from the message queue and
// dispatches it. Handles both slash commands and user messages.
// Returns a tea.Cmd that performs the dispatch, or nil if the queue is empty.
func processQueueCmd(m *AppModel) tea.Cmd {
	if m.msgQueue == nil {
		return nil
	}

	cmd, ok := m.msgQueue.Dequeue("") // main session agentID=""
	if !ok {
		return nil
	}

	switch cmd.Mode {
	case msgqueue.ModeSlashCommand:
		text := cmd.Value
		return func() tea.Msg {
			return queuedSlashMsg{Text: text}
		}
	case msgqueue.ModePrompt:
		// The user message was already appended to m.messages when enqueued
		// (in handleSubmit), so we don't append again. Just start the query.
		m.isLoading = true
		m.showSpinner = true
		m.spinner = m.spinner.Reset()
		return startQueryCmd(m, cmd.Value)
	}
	return nil
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
			m.syncViewportContent()
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

	if result.NewEffort != "" {
		m.effort = result.NewEffort
		// Update status bar to show new effort level
		m.statusBar.effort = result.NewEffort
	}

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

	if result.ClearHistory {
		m.messages = nil
		m.queryEngine.SetMessages(nil)
		m.streamingText = ""
		m.streamingHasMsg = false
		m.syncViewportContent()
	}

	switch result.Display {
	case commands.DisplayMessage:
		if result.Text != "" {
			m.messages = append(m.messages, newSystemMessage(result.Text))
			m.syncViewportContent()
		}
	case commands.DisplayError:
		if result.Text != "" {
			m.messages = append(m.messages, newSystemMessage("Error: "+result.Text))
			m.syncViewportContent()
		}
	case commands.DisplayNone:
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
