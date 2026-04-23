// Package engine implements the core query loop that drives LLM conversations,
// tool execution, and streaming events to the TUI layer.
package engine

import (
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// MsgType is the discriminator for Msg events emitted by the engine.
type MsgType string

const (
	// MsgTypeStreamRequestStart is emitted when an API streaming request begins.
	MsgTypeStreamRequestStart MsgType = "stream_request_start"
	// MsgTypeStreamText is emitted for each incremental text delta.
	MsgTypeStreamText MsgType = "stream_text"
	// MsgTypeThinkingDelta is emitted for each incremental thinking delta.
	MsgTypeThinkingDelta MsgType = "thinking_delta"
	// MsgTypeToolUseStart is emitted when the LLM begins a tool call.
	MsgTypeToolUseStart MsgType = "tool_use_start"
	// MsgTypeToolUseInputDelta is emitted for each incremental JSON fragment of a tool's input.
	MsgTypeToolUseInputDelta MsgType = "tool_use_input_delta"
	// MsgTypeToolUseComplete is emitted when the full tool input is known.
	MsgTypeToolUseComplete MsgType = "tool_use_complete"
	// MsgTypeToolResult is emitted after a tool finishes executing.
	MsgTypeToolResult MsgType = "tool_result"
	// MsgTypeAssistantMessage is emitted when an assistant turn is finalised.
	MsgTypeAssistantMessage MsgType = "assistant_message"
	// MsgTypeUserMessage is emitted when a user message is appended.
	MsgTypeUserMessage MsgType = "user_message"
	// MsgTypeProgress is emitted for long-running tool progress updates.
	MsgTypeProgress MsgType = "progress"
	// MsgTypeError is emitted on unrecoverable errors.
	MsgTypeError MsgType = "error"
	// MsgTypeRequestStart is emitted at the start of each LLM request round.
	MsgTypeRequestStart MsgType = "request_start"
	// MsgTypeTurnComplete is emitted when a full query cycle finishes.
	MsgTypeTurnComplete MsgType = "turn_complete"
	// MsgTypeCompactStart is emitted when context compaction begins.
	MsgTypeCompactStart MsgType = "compact_start"
	// MsgTypeCompactEnd is emitted when context compaction finishes.
	MsgTypeCompactEnd MsgType = "compact_end"
	// MsgTypeSystemMessage is emitted to surface informational system text.
	MsgTypeSystemMessage MsgType = "system_message"
	// MsgTypeTombstone marks a message that was deleted from the history.
	MsgTypeTombstone MsgType = "tombstone"
	// MsgTypePermissionRequest is emitted when a tool requires user permission.
	MsgTypePermissionRequest MsgType = "permission_request"
	// MsgTypePermissionResponse is received when the user responds to a permission request.
	MsgTypePermissionResponse MsgType = "permission_response"
)

// Msg is the event type emitted by the engine over the channel returned by
// QueryEngine.Query. The TUI layer dispatches on Type.
//
// Only the fields relevant to a given Type are populated; all others are zero.
type Msg struct {
	Type MsgType

	// --- MsgTypeStreamText / MsgTypeThinkingDelta ---
	TextDelta string

	// --- MsgTypeToolUseStart / MsgTypeToolUseInputDelta ---
	ToolUseID  string
	ToolName   string
	InputDelta string // streaming JSON fragment (MsgTypeToolUseInputDelta)

	// --- MsgTypeToolUseComplete ---
	ToolInput string // complete JSON

	// --- MsgTypeToolResult ---
	ToolResult *ToolResultMsg

	// --- MsgTypeAssistantMessage ---
	AssistantMsg *types.Message

	// --- MsgTypeUserMessage ---
	UserMsg *types.Message

	// --- MsgTypeProgress ---
	ProgressData any

	// --- MsgTypeError ---
	Err error

	// --- MsgTypeRequestStart ---
	RequestID string
	Model     string

	// --- MsgTypeTurnComplete ---
	StopReason         string
	InputTokens        int
	OutputTokens       int
	CacheReadTokens    int
	CacheCreatedTokens int

	// --- MsgTypeCompactStart / MsgTypeCompactEnd ---
	CompactStrategy string // "auto" | "micro" | "snip"

	// --- MsgTypeSystemMessage ---
	SystemText string

	// --- MsgTypePermissionRequest ---
	PermissionReq *PermissionRequestMsg
}

// ToolResultMsg carries the result of a single tool execution.
type ToolResultMsg struct {
	ToolUseID string
	Content   string
	IsError   bool
}

// PermissionRequestMsg carries a permission request for a tool call.
// The TUI layer should display a dialog and call RespFn with the user's decision.
type PermissionRequestMsg struct {
	RequestID string `json:"request_id"`
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
	Message   string `json:"message"`
	Input     string `json:"input"` // JSON input for display
	// RespFn is called by the TUI once the user makes a decision.
	// allow=true means permit the tool call; allow=false means deny.
	RespFn func(allow bool) `json:"-"` // excluded from JSON serialization
}
