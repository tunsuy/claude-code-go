// Package coordinator implements multi-agent coordination for Claude Code Go.
//
// It manages the lifecycle of sub-agents (spawn, message routing, stop, status
// query) and exposes the Coordinator interface used by the AgentTool,
// SendMessageTool, and TaskStopTool.
//
// Design reference: docs/project/design/core.md §6
package coordinator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tunsuy/claude-code-go/pkg/utils/ids"
)

// ─────────────────────────────────────────────────────────────────────────────
// Public types
// ─────────────────────────────────────────────────────────────────────────────

// AgentID is the unique identifier of a spawned sub-agent.
type AgentID string

// AgentStatus is the lifecycle state of a sub-agent.
type AgentStatus string

const (
	AgentStatusRunning   AgentStatus = "running"
	AgentStatusCompleted AgentStatus = "completed"
	AgentStatusFailed    AgentStatus = "failed"
	AgentStatusStopped   AgentStatus = "killed"
)

// ProgressEvent is emitted by the coordinator to report sub-agent activity.
// The TUI layer converts these into AgentProgressMsg / AgentStatusMsg.
type ProgressEvent struct {
	AgentID     AgentID
	Description string // human-readable task description
	Activity    string // short label: "Streaming", "Running Bash", "Reading file"
	Detail      string // one-line detail, e.g. partial text or tool input
}

// OnProgressFn is called by the coordinator to report real-time sub-agent
// progress. Implementations must be safe for concurrent use.
type OnProgressFn func(evt ProgressEvent)

// OnStatusChangeFn is called when a sub-agent's lifecycle status changes.
// Implementations must be safe for concurrent use.
type OnStatusChangeFn func(agentID AgentID, description string, status AgentStatus)

// EventKind discriminates Event types.
type EventKind int

const (
	// EventProgress reports a real-time activity update from a running sub-agent.
	EventProgress EventKind = iota
	// EventStatusChange reports a lifecycle status change (running/completed/failed).
	EventStatusChange
)

// Event is the unified event type pushed to consumers (TUI) from the coordinator.
// It avoids import cycles between bootstrap/tui/coordinator layers.
type Event struct {
	Kind        EventKind
	AgentID     string
	Description string
	// Progress fields (Kind == EventProgress)
	Activity string
	Detail   string
	// Status fields (Kind == EventStatusChange)
	Status string // "running", "completed", "failed", "killed"
}

// SpawnRequest is the parameter bundle passed to SpawnAgent.
type SpawnRequest struct {
	// Description is a human-readable summary of the sub-agent's task.
	Description string
	// SubagentType is the agent kind, e.g. "worker".
	SubagentType string
	// Prompt is the full initial prompt delivered to the sub-agent.
	Prompt string
	// Model overrides the default LLM model; empty means use default.
	Model string
	// AllowedTools restricts the tools available to the sub-agent.
	AllowedTools []string
	// MaxTurns limits the sub-agent's query loops (0 = unlimited).
	MaxTurns int
	// ParentAgentID is set when this is a nested sub-agent spawn.
	ParentAgentID AgentID
}

// AgentUsage records the resource consumption of a completed sub-agent.
type AgentUsage struct {
	TotalTokens int
	ToolUses    int
	DurationMs  int64
}

// TaskNotification is sent by a sub-agent to the coordinator when it finishes.
// It maps to the <task-notification> XML protocol in coordinatorMode.ts.
type TaskNotification struct {
	TaskID  AgentID
	Status  AgentStatus
	Summary string
	Result  string
	Usage   *AgentUsage
}

// MCPClientInfo is the minimal MCP server descriptor used when building the
// coordinator user-context map.
type MCPClientInfo struct {
	Name string
}

// ─────────────────────────────────────────────────────────────────────────────
// Coordinator interface
// ─────────────────────────────────────────────────────────────────────────────

// Coordinator manages the lifecycle and message routing of multiple sub-agents.
// Implementations must be safe for concurrent use.
type Coordinator interface {
	// SpawnAgent launches a new sub-agent and returns its AgentID.
	SpawnAgent(ctx context.Context, req SpawnRequest) (AgentID, error)

	// SendMessage delivers a follow-up message to a running sub-agent.
	SendMessage(ctx context.Context, to AgentID, message string) error

	// StopAgent stops a running sub-agent.
	StopAgent(ctx context.Context, agentID AgentID) error

	// GetAgentStatus returns the current lifecycle state of a sub-agent.
	GetAgentStatus(ctx context.Context, agentID AgentID) (AgentStatus, error)

	// Subscribe returns a channel that receives TaskNotification events for the
	// given sub-agent. The channel is closed after the agent terminates.
	Subscribe(agentID AgentID) (<-chan TaskNotification, error)

	// IsCoordinatorMode reports whether coordinator mode is active.
	IsCoordinatorMode() bool
}

// ─────────────────────────────────────────────────────────────────────────────
// agentEntry – internal per-agent state
// ─────────────────────────────────────────────────────────────────────────────

const inboxBufferSize = 16

// agentEntry holds the mutable runtime state of a single sub-agent.
type agentEntry struct {
	id          AgentID
	req         SpawnRequest
	status      AgentStatus
	startedAt   time.Time
	finishedAt  time.Time
	result      string
	err         error
	inboxCh     chan string             // buffered message queue from coordinator
	cancelFn    context.CancelFunc     // stops the agent's goroutine
	subscribers []chan TaskNotification // notification fan-out
	mu          sync.Mutex             // protects status, finishedAt, result, err, subscribers
}

// ─────────────────────────────────────────────────────────────────────────────
// coordinatorImpl – concrete Coordinator
// ─────────────────────────────────────────────────────────────────────────────

// coordinatorImpl is the default Coordinator implementation.
// Sub-agents are simulated via goroutines; real engine integration is performed
// by injecting a RunAgent function via Config.
type coordinatorImpl struct {
	coordinatorMode bool
	runAgent        RunAgentFn
	onProgress      OnProgressFn
	onStatusChange  OnStatusChangeFn

	mu     sync.RWMutex
	agents map[AgentID]*agentEntry
}

// RunAgentFn is the function signature used to actually run a sub-agent.
// Implementations should block until the agent finishes and return (result, err).
// An empty result with a nil error indicates successful completion with no output.
//
// The agentID parameter is the unique identifier assigned by the coordinator,
// allowing the function to emit progress events tagged with the correct ID.
type RunAgentFn func(ctx context.Context, agentID AgentID, req SpawnRequest, inboxCh <-chan string) (string, error)

// Config is the constructor parameter bundle for New.
type Config struct {
	// CoordinatorMode activates the coordinator system-prompt injection.
	CoordinatorMode bool
	// RunAgent is the function used to execute each sub-agent.
	// If nil, a no-op stub is used (useful for testing the coordination logic
	// without a real LLM).
	RunAgent RunAgentFn
	// OnProgress is called to report real-time sub-agent progress.
	// May be nil; the coordinator tolerates a nil callback.
	OnProgress OnProgressFn
	// OnStatusChange is called when a sub-agent's lifecycle status changes.
	// May be nil; the coordinator tolerates a nil callback.
	OnStatusChange OnStatusChangeFn
}

// New creates and returns a new Coordinator.
func New(cfg Config) Coordinator {
	fn := cfg.RunAgent
	if fn == nil {
		fn = noopRunAgent
	}
	return &coordinatorImpl{
		coordinatorMode: cfg.CoordinatorMode,
		runAgent:        fn,
		onProgress:      cfg.OnProgress,
		onStatusChange:  cfg.OnStatusChange,
		agents:          make(map[AgentID]*agentEntry),
	}
}

// noopRunAgent is the default RunAgentFn used when none is supplied.
// It immediately returns an empty result so that coordination logic can be
// exercised without a real LLM.
func noopRunAgent(_ context.Context, _ AgentID, _ SpawnRequest, _ <-chan string) (string, error) {
	return "", nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Coordinator method implementations
// ─────────────────────────────────────────────────────────────────────────────

// defaultMaxTurns is the safety limit applied when the caller does not specify
// a MaxTurns value. This prevents sub-agents from running indefinitely.
const defaultMaxTurns = 30

// SpawnAgent starts a new sub-agent goroutine and registers it.
// If an agent with the same Description is already running, the existing
// agent's ID is returned instead of spawning a duplicate.
func (c *coordinatorImpl) SpawnAgent(ctx context.Context, req SpawnRequest) (AgentID, error) {
	if req.Prompt == "" {
		return "", fmt.Errorf("coordinator: SpawnRequest.Prompt must not be empty")
	}

	// Apply a default MaxTurns safety limit when the caller does not set one.
	if req.MaxTurns <= 0 {
		req.MaxTurns = defaultMaxTurns
	}

	// ── Deduplication + creation under a single write lock to prevent TOCTOU
	// races where two concurrent spawns with the same description both pass
	// the read-check and create duplicate agents. ──────────────────────────
	c.mu.Lock()
	for _, existing := range c.agents {
		existing.mu.Lock()
		sameDesc := existing.req.Description == req.Description
		isRunning := existing.status == AgentStatusRunning
		existingID := existing.id
		existing.mu.Unlock()
		if sameDesc && isRunning {
			c.mu.Unlock()
			return existingID, nil
		}
	}

	// Generate a unique, correctly-formatted AgentID.
	raw := ids.NewAgentId("worker")
	agentID := AgentID(raw)

	agentCtx, cancelFn := context.WithCancel(ctx)

	entry := &agentEntry{
		id:        agentID,
		req:       req,
		status:    AgentStatusRunning,
		startedAt: time.Now(),
		inboxCh:   make(chan string, inboxBufferSize),
		cancelFn:  cancelFn,
	}

	c.agents[agentID] = entry
	c.mu.Unlock()

	// Notify TUI of the new agent.
	if c.onStatusChange != nil {
		c.onStatusChange(agentID, req.Description, AgentStatusRunning)
	}

	// Run the agent asynchronously.
	go c.runAgentLoop(agentCtx, entry)

	return agentID, nil
}

// runAgentLoop executes a sub-agent and updates its status when done.
func (c *coordinatorImpl) runAgentLoop(ctx context.Context, entry *agentEntry) {
	result, err := c.runAgent(ctx, entry.id, entry.req, entry.inboxCh)

	entry.mu.Lock()
	entry.finishedAt = time.Now()
	entry.result = result
	entry.err = err
	if err != nil {
		entry.status = AgentStatusFailed
	} else {
		entry.status = AgentStatusCompleted
	}
	finalStatus := entry.status
	durationMs := entry.finishedAt.Sub(entry.startedAt).Milliseconds()

	// Build notification.
	notif := TaskNotification{
		TaskID:  entry.id,
		Status:  entry.status,
		Summary: summaryFor(entry),
		Result:  result,
		Usage: &AgentUsage{
			DurationMs: durationMs,
		},
	}

	// Fan-out to all subscribers, then close channels.
	subs := entry.subscribers
	entry.subscribers = nil
	desc := entry.req.Description
	entry.mu.Unlock()

	// Notify TUI of status change.
	if c.onStatusChange != nil {
		c.onStatusChange(entry.id, desc, finalStatus)
	}

	for _, ch := range subs {
		// Non-blocking send: if the subscriber's buffer is full we drop rather
		// than deadlock. Callers should use a buffered channel.
		select {
		case ch <- notif:
		default:
		}
		close(ch)
	}
}

// summaryFor builds a human-readable summary for a finished agent.
func summaryFor(entry *agentEntry) string {
	switch entry.status {
	case AgentStatusCompleted:
		return fmt.Sprintf("agent %s completed successfully", entry.id)
	case AgentStatusFailed:
		return fmt.Sprintf("agent %s failed: %v", entry.id, entry.err)
	case AgentStatusStopped:
		return fmt.Sprintf("agent %s was stopped", entry.id)
	default:
		return fmt.Sprintf("agent %s status: %s", entry.id, entry.status)
	}
}

// SendMessage enqueues a message into the target agent's inbox channel.
func (c *coordinatorImpl) SendMessage(_ context.Context, to AgentID, message string) error {
	entry, err := c.lookupRunning(to)
	if err != nil {
		return err
	}

	select {
	case entry.inboxCh <- message:
		return nil
	default:
		return fmt.Errorf("coordinator: agent %s inbox is full (capacity %d)", to, inboxBufferSize)
	}
}

// StopAgent cancels the target agent's context, transitioning it to Stopped.
func (c *coordinatorImpl) StopAgent(_ context.Context, agentID AgentID) error {
	c.mu.RLock()
	entry, ok := c.agents[agentID]
	c.mu.RUnlock()
	if !ok {
		return fmt.Errorf("coordinator: unknown agent %s", agentID)
	}

	entry.mu.Lock()
	if entry.status != AgentStatusRunning {
		// Capture status before releasing the lock to avoid a data race on the
		// string read inside fmt.Errorf.
		currentStatus := entry.status
		entry.mu.Unlock()
		return fmt.Errorf("coordinator: agent %s is not running (status=%s)", agentID, currentStatus)
	}
	entry.status = AgentStatusStopped
	entry.mu.Unlock()

	// Cancel the agent's context to unblock its goroutine.
	entry.cancelFn()
	return nil
}

// GetAgentStatus returns the current status of a registered agent.
func (c *coordinatorImpl) GetAgentStatus(_ context.Context, agentID AgentID) (AgentStatus, error) {
	c.mu.RLock()
	entry, ok := c.agents[agentID]
	c.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("coordinator: unknown agent %s", agentID)
	}

	entry.mu.Lock()
	status := entry.status
	entry.mu.Unlock()
	return status, nil
}

// Subscribe returns a buffered channel that will receive exactly one
// TaskNotification when the specified agent finishes.
//
// If the agent has already finished, the notification is sent immediately and
// the channel is closed.
func (c *coordinatorImpl) Subscribe(agentID AgentID) (<-chan TaskNotification, error) {
	c.mu.RLock()
	entry, ok := c.agents[agentID]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("coordinator: unknown agent %s", agentID)
	}

	ch := make(chan TaskNotification, 1)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.status != AgentStatusRunning {
		// Agent already done — deliver notification immediately.
		notif := TaskNotification{
			TaskID:  entry.id,
			Status:  entry.status,
			Summary: summaryFor(entry),
			Result:  entry.result,
			Usage: &AgentUsage{
				DurationMs: entry.finishedAt.Sub(entry.startedAt).Milliseconds(),
			},
		}
		ch <- notif
		close(ch)
		return ch, nil
	}

	entry.subscribers = append(entry.subscribers, ch)
	return ch, nil
}

// IsCoordinatorMode reports whether coordinator mode is enabled.
func (c *coordinatorImpl) IsCoordinatorMode() bool {
	return c.coordinatorMode
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// lookupRunning retrieves an agent entry and asserts it is still running.
func (c *coordinatorImpl) lookupRunning(agentID AgentID) (*agentEntry, error) {
	c.mu.RLock()
	entry, ok := c.agents[agentID]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("coordinator: unknown agent %s", agentID)
	}

	entry.mu.Lock()
	status := entry.status
	entry.mu.Unlock()
	if status != AgentStatusRunning {
		return nil, fmt.Errorf("coordinator: agent %s is not running (status=%s)", agentID, status)
	}
	return entry, nil
}

// ListAgents returns a snapshot of all currently-registered agents and their
// statuses.  This is primarily useful for testing and diagnostics.
func (c *coordinatorImpl) ListAgents() map[AgentID]AgentStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make(map[AgentID]AgentStatus, len(c.agents))
	for id, entry := range c.agents {
		entry.mu.Lock()
		out[id] = entry.status
		entry.mu.Unlock()
	}
	return out
}
