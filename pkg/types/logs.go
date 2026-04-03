// Package types defines the shared types for claude-code-go.
// All types in this package are stable once reviewed; changes require
// cross-package coordination. Do NOT add non-stdlib imports.
package types

import (
	"encoding/json"
	"time"
)

// EntryType enumerates the JSONL log entry types (mirrors TypeScript src/types/logs.ts).
type EntryType string

const (
	EntryTypeTranscript     EntryType = "transcript"
	EntryTypeSummary        EntryType = "summary"
	EntryTypeTag            EntryType = "tag"
	EntryTypePRLink         EntryType = "pr_link"
	EntryTypeWorktree       EntryType = "worktree_state"
	EntryTypeDebug          EntryType = "debug"
	EntryTypeToolResult     EntryType = "tool_result"
	EntryTypeThinking       EntryType = "thinking"
	EntryTypeBash           EntryType = "bash"
	EntryTypeImage          EntryType = "image"
	EntryTypeError          EntryType = "error"
	EntryTypeInfo           EntryType = "info"
	EntryTypeSystem         EntryType = "system"
	EntryTypeToolUse        EntryType = "tool_use"
	EntryTypeAssistant      EntryType = "assistant"
	EntryTypeUser           EntryType = "user"
	EntryTypeProgress       EntryType = "progress"
	EntryTypeResult         EntryType = "result"
	EntryTypeCompletion     EntryType = "completion"
	EntryTypeBranchSwitch   EntryType = "branch_switch"
	EntryTypeSessionStart   EntryType = "session_start"
	EntryTypeSessionEnd     EntryType = "session_end"
	EntryTypeContextCompact EntryType = "context_compact"
)

// EntryEnvelope is a JSONL line envelope used to quickly determine the entry type
// before full decoding.
// Raw is NOT populated by json.Unmarshal; callers must populate it manually
// (e.g. from the raw scanner bytes) after type determination.
type EntryEnvelope struct {
	Type EntryType `json:"type"`
	// Raw holds the full original JSON bytes for this entry.
	// It is not a JSON field and must be populated explicitly by the reader.
	Raw json.RawMessage `json:"-"`
}

// SerializedMessage is a message snapshot persisted to JSONL.
// Uses explicit composition (named field) instead of embedding to avoid field shadowing.
type SerializedMessage struct {
	Message   Message   `json:"message"`
	CWD       string    `json:"cwd"`
	UserType  string    `json:"userType"`
	SessionId SessionId `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	GitBranch string    `json:"gitBranch,omitempty"`
	Slug      string    `json:"slug,omitempty"`
}

// TranscriptMessage is a full message record with multi-chain information.
type TranscriptMessage struct {
	SerializedMessage SerializedMessage `json:"serializedMessage"`
	ParentUUID        string            `json:"parentUuid,omitempty"`
	IsSidechain       bool              `json:"isSidechain,omitempty"`
	AgentId           AgentId           `json:"agentId,omitempty"`
}

// LogOption is a session summary shown in the /resume selector.
type LogOption struct {
	SessionId   SessionId           `json:"sessionId"`
	Date        time.Time           `json:"date"`
	FirstPrompt string              `json:"firstPrompt"`
	IsSidechain bool                `json:"isSidechain,omitempty"`
	Messages    []SerializedMessage `json:"messages,omitempty"`
	Title       string              `json:"title,omitempty"`
	GitBranch   string              `json:"gitBranch,omitempty"`
}

// SummaryEntry is a compressed session summary entry.
type SummaryEntry struct {
	Type    EntryType `json:"type"` // "summary"
	Summary string    `json:"summary"`
	LeafId  string    `json:"leafId,omitempty"`
}
