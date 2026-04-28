package msgqueue

import (
	"fmt"
	"sync/atomic"
	"time"
)

// seq is a process-wide monotonic counter for generating unique command IDs.
var seq uint64

// Priority determines dequeue ordering. Lower numeric value = higher priority.
// Within the same priority level, commands are dequeued in FIFO order.
type Priority int

const (
	// PriorityNow is the highest priority — reserved for P2 (immediate interrupt).
	PriorityNow Priority = 0
	// PriorityNext is for user follow-up messages injected at the next drain point.
	PriorityNext Priority = 1
	// PriorityLater is for background notifications processed between turns.
	PriorityLater Priority = 2
)

// String returns a human-readable label for the priority.
func (p Priority) String() string {
	switch p {
	case PriorityNow:
		return "now"
	case PriorityNext:
		return "next"
	case PriorityLater:
		return "later"
	default:
		return fmt.Sprintf("priority(%d)", int(p))
	}
}

// CommandMode distinguishes slash commands from natural-language input.
type CommandMode int

const (
	// ModePrompt is free-form text that triggers an LLM query.
	ModePrompt CommandMode = iota
	// ModeSlashCommand is a slash command (e.g. "/compact", "/model").
	ModeSlashCommand
)

// String returns a human-readable label for the command mode.
func (m CommandMode) String() string {
	switch m {
	case ModePrompt:
		return "prompt"
	case ModeSlashCommand:
		return "slash"
	default:
		return fmt.Sprintf("mode(%d)", int(m))
	}
}

// QueuedCommand is an immutable snapshot of a queued user action.
// Fields are exported for serialization but should be treated as read-only
// after construction — the queue never mutates a command in place.
type QueuedCommand struct {
	// ID is a unique identifier assigned at construction time.
	ID string
	// Value is the raw text the user typed.
	Value string
	// Mode distinguishes slash commands from natural-language input.
	Mode CommandMode
	// Priority determines dequeue ordering.
	Priority Priority
	// AgentID is the originating agent scope ("" = main session).
	AgentID string
	// CreatedAt is the wall-clock time when the command was created.
	CreatedAt time.Time
}

// NewCommand creates a QueuedCommand with a unique ID and the current timestamp.
// The agentID defaults to "" (main session).
func NewCommand(value string, mode CommandMode, priority Priority) QueuedCommand {
	n := atomic.AddUint64(&seq, 1)
	return QueuedCommand{
		ID:        fmt.Sprintf("qc-%d-%d", time.Now().UnixNano(), n),
		Value:     value,
		Mode:      mode,
		Priority:  priority,
		AgentID:   "",
		CreatedAt: time.Now(),
	}
}

// NewCommandWithAgent creates a QueuedCommand scoped to a specific agent.
func NewCommandWithAgent(value string, mode CommandMode, priority Priority, agentID string) QueuedCommand {
	cmd := NewCommand(value, mode, priority)
	cmd.AgentID = agentID
	return cmd
}
