package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// startQueryCmd builds a tea.Cmd that launches a new query against the engine.
// It cancels any in-flight query first, then opens a new context and channel.
// The userText message is already appended to m.messages by the caller (handleSubmit).
func startQueryCmd(m *AppModel, userText string) tea.Cmd {
	// Cancel any running query.
	if m.abortFn != nil {
		m.abortFn()
		m.abortFn = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.abortCtx = ctx
	m.abortFn = cancel

	// Build user message for the engine.
	userMsg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr(userText)},
		},
	}

	// Snapshot current messages (already includes the user message for display).
	messages := m.queryEngine.GetMessages()
	messages = append(messages, userMsg)

	// F-1 fix: build a complete QueryParams, not just Messages.
	// Inject the CLAUDE.md system prompt when available.
	var sysPrompt engine.SystemPrompt
	if m.memdirPrompt != "" {
		sysPrompt = engine.SystemPrompt{
			Parts: []engine.SystemPromptPart{
				{Text: m.memdirPrompt, CacheControl: "ephemeral"},
			},
		}
	}

	params := engine.QueryParams{
		Messages:     messages,
		SystemPrompt: sysPrompt,
		QuerySource:  "foreground",
	}

	qe := m.queryEngine

	return func() tea.Msg {
		ch, err := qe.Query(ctx, params)
		if err != nil {
			return StreamErrorMsg{Err: err}
		}
		return streamChanReady{ch: ch}
	}
}

// streamChanReady is an internal Msg that carries the newly-opened stream
// channel back into the Update loop so it can be stored and polled.
type streamChanReady struct {
	ch <-chan engine.Msg
}

// waitForStreamEvent returns a Cmd that blocks until the next engine.Msg
// arrives on ch, then wraps it in an appropriate tea.Msg.
func waitForStreamEvent(ch <-chan engine.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			// Channel closed — stream finished without explicit TurnComplete.
			return StreamDoneMsg{}
		}
		result := dispatchEngineMsg(msg)
		if result == nil {
			// Unknown/unhandled type: pull the next event immediately.
			return waitForStreamEvent(ch)()
		}
		return result
	}
}

// dispatchEngineMsg converts an engine.Msg into the appropriate TUI Msg.
func dispatchEngineMsg(msg engine.Msg) tea.Msg {
	switch msg.Type {
	case engine.MsgTypeStreamText:
		return StreamTokenMsg{Delta: msg.TextDelta}
	case engine.MsgTypeThinkingDelta:
		return StreamThinkingMsg{Delta: msg.TextDelta}
	case engine.MsgTypeToolUseStart:
		return StreamToolUseStartMsg{
			ToolUseID:  msg.ToolUseID,
			ToolName:   msg.ToolName,
			InputDelta: msg.InputDelta,
		}
	case engine.MsgTypeToolUseInputDelta:
		return StreamToolUseInputDeltaMsg{
			ToolUseID:  msg.ToolUseID,
			ToolName:   msg.ToolName,
			InputDelta: msg.InputDelta,
		}
	case engine.MsgTypeToolUseComplete:
		return StreamToolUseCompleteMsg{
			ToolUseID: msg.ToolUseID,
			ToolInput: msg.ToolInput,
		}
	case engine.MsgTypeToolResult:
		if msg.ToolResult != nil {
			return StreamToolResultMsg{
				ToolUseID: msg.ToolResult.ToolUseID,
				Content:   msg.ToolResult.Content,
				IsError:   msg.ToolResult.IsError,
			}
		}
	case engine.MsgTypeAssistantMessage:
		return StreamAssistantTurnMsg{FinalMessage: msg.AssistantMsg}
	case engine.MsgTypeUserMessage:
		return StreamUserTurnMsg{FinalMessage: msg.UserMsg}
	case engine.MsgTypeTurnComplete:
		// Only treat as "stream done" when the stop reason is terminal.
		// For tool_use / max_tokens the engine will continue the loop,
		// so the TUI must keep pulling events from the channel.
		switch msg.StopReason {
		case "tool_use", "max_tokens":
			return StreamAssistantTurnMsg{}
		default:
			// "end_turn", "stop_sequence", "" → query finished.
			return StreamDoneMsg{}
		}
	case engine.MsgTypeError:
		return StreamErrorMsg{Err: msg.Err}
	case engine.MsgTypeSystemMessage:
		return SystemTextMsg{Text: msg.SystemText}
	}
	return nil
}

// abortQueryCmd interrupts the current query.
func abortQueryCmd(qe engine.QueryEngine) tea.Cmd {
	return func() tea.Msg {
		qe.Interrupt(context.Background())
		return nil
	}
}
