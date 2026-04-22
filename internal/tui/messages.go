package tui

import (
	"time"

	"github.com/tunsuy/claude-code-go/pkg/types"
)

// --- User input ---

// InputSubmittedMsg is sent when the user presses Enter.
type InputSubmittedMsg struct{ Text string }

// InputChangedMsg is sent when the input text changes (e.g. Tab completion).
type InputChangedMsg struct{ Text string }

// SlashCommandMsg is sent when the user submits a slash command.
type SlashCommandMsg struct {
	Name string
	Args string
}

// --- LLM streaming events ---

// StreamTokenMsg carries a text delta from the streaming response.
type StreamTokenMsg struct{ Delta string }

// StreamThinkingMsg carries a thinking block delta.
type StreamThinkingMsg struct{ Delta string }

// StreamToolUseStartMsg is emitted when the LLM begins a tool call.
type StreamToolUseStartMsg struct {
	ToolUseID  string
	ToolName   string
	InputDelta string
}

// StreamToolUseInputDeltaMsg is emitted for each incremental JSON fragment of
// a tool call's input while the LLM is still streaming it.
type StreamToolUseInputDeltaMsg struct {
	ToolUseID  string
	ToolName   string
	InputDelta string
}

// StreamToolUseCompleteMsg is emitted when the LLM completes a tool call block.
type StreamToolUseCompleteMsg struct {
	ToolUseID string
	ToolInput string
}

// StreamToolResultMsg is emitted after a tool finishes executing.
type StreamToolResultMsg struct {
	ToolUseID string
	Content   string
	IsError   bool
}

// StreamAssistantTurnMsg is emitted when one assistant turn completes
// (i.e. the LLM finishes one streaming response). In a tool_use scenario
// this does NOT mean the whole query is done — the engine will continue with
// tool execution and a subsequent LLM call. The TUI must keep pulling events.
type StreamAssistantTurnMsg struct {
	FinalMessage *types.Message // the completed assistant message for this turn
}

// StreamDoneMsg is emitted when the entire query cycle is finished.
// No more events will arrive on the stream channel after this.
type StreamDoneMsg struct {
	FinalMessage *types.Message // the completed assistant message (legacy; may be nil)
}

// StreamErrorMsg is emitted on an unrecoverable streaming error.
type StreamErrorMsg struct{ Err error }

// --- Permission request ---

// PermissionRequestMsg is sent when a tool call requires user confirmation.
type PermissionRequestMsg struct {
	RequestID string
	ToolName  string
	ToolUseID string
	Message   string
	Input     string // JSON
	// RespFn is called by the TUI once the user makes a decision.
	// Using a function avoids channel-send blocking issues.
	RespFn func(allow bool)
}

// --- System events ---

// TermResizedMsg wraps tea.WindowSizeMsg.
type TermResizedMsg struct{ Width, Height int }

// TickMsg is the periodic tick for spinner animation / task eviction.
type TickMsg struct{ Time time.Time }

// CompactDoneMsg is sent when /compact finishes.
type CompactDoneMsg struct{ Summary string }

// AgentStatusMsg is pushed from Agent-Core to update a sub-agent's status.
type AgentStatusMsg struct {
	TaskID string
	Status AgentStatus
}

// CommandResultMsg carries the result of a slash command execution.
type CommandResultMsg struct {
	Text    string
	IsError bool
}

// MemdirLoadedMsg is sent when the initial CLAUDE.md files have been loaded.
type MemdirLoadedMsg struct {
	Paths []string
}

// SystemTextMsg surfaces informational text from the engine (e.g. max turns).
type SystemTextMsg struct{ Text string }
