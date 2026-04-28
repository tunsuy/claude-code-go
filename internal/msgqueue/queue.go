package msgqueue

import (
	"sort"
	"sync"
)

// MessageQueue is a thread-safe, priority-ordered command queue.
//
// Ordering guarantee: within the same priority level, commands are FIFO.
// Across priorities, lower numeric Priority dequeues first (Now > Next > Later).
//
// All public methods are safe for concurrent use from multiple goroutines.
// The queue broadcasts to all Signal subscribers on every mutation.
type MessageQueue struct {
	mu     sync.Mutex
	items  []QueuedCommand
	signal *Signal
}

// NewMessageQueue creates an empty queue with its own Signal for change notification.
func NewMessageQueue() *MessageQueue {
	return &MessageQueue{
		signal: NewSignal(),
	}
}

// Enqueue appends a command at the correct priority position (stable insertion).
// Commands with the same priority are ordered by insertion time (FIFO).
// Broadcasts to all subscribers after insertion.
func (q *MessageQueue) Enqueue(cmd QueuedCommand) {
	q.mu.Lock()
	q.insertSorted(cmd)
	q.mu.Unlock()
	q.signal.Broadcast()
}

// Dequeue removes and returns the highest-priority command whose AgentID
// matches the given agentID. Returns (cmd, true) on success, or
// (QueuedCommand{}, false) if no matching command exists.
//
// Filter semantics:
//   - agentID=="" matches commands with AgentID=="" (main session only)
//   - agentID!="" matches that specific agent
func (q *MessageQueue) Dequeue(agentID string) (QueuedCommand, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, cmd := range q.items {
		if cmd.AgentID == agentID {
			q.items = append(q.items[:i], q.items[i+1:]...)
			q.signal.Broadcast()
			return cmd, true
		}
	}
	return QueuedCommand{}, false
}

// DequeueAll removes and returns ALL commands matching the agentID filter,
// ordered by priority then FIFO. Returns nil if none match.
// A single broadcast is emitted after all removals.
func (q *MessageQueue) DequeueAll(agentID string) []QueuedCommand {
	q.mu.Lock()
	defer q.mu.Unlock()

	var matched []QueuedCommand
	remaining := q.items[:0] // reuse backing array
	for _, cmd := range q.items {
		if cmd.AgentID == agentID {
			matched = append(matched, cmd)
		} else {
			remaining = append(remaining, cmd)
		}
	}

	if len(matched) == 0 {
		return nil
	}

	q.items = remaining
	q.signal.Broadcast()
	return matched
}

// GetByMaxPriority peeks (non-destructive) at commands with
// Priority <= maxPri matching the agentID filter. Returns a copy slice.
func (q *MessageQueue) GetByMaxPriority(maxPri Priority, agentID string) []QueuedCommand {
	q.mu.Lock()
	defer q.mu.Unlock()

	var result []QueuedCommand
	for _, cmd := range q.items {
		if cmd.Priority <= maxPri && cmd.AgentID == agentID {
			result = append(result, cmd)
		}
	}
	return result
}

// RemoveByIDs removes commands matching any of the given IDs.
// Returns the number of commands actually removed. Broadcasts if >0 removed.
func (q *MessageQueue) RemoveByIDs(ids []string) int {
	if len(ids) == 0 {
		return 0
	}

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	removed := 0
	remaining := q.items[:0]
	for _, cmd := range q.items {
		if idSet[cmd.ID] {
			removed++
		} else {
			remaining = append(remaining, cmd)
		}
	}

	if removed == 0 {
		return 0
	}

	q.items = remaining
	q.signal.Broadcast()
	return removed
}

// PopAllEditable removes and returns all commands with Priority >= PriorityNext
// matching the agentID filter. "Editable" means they haven't been promoted to
// PriorityNow (which implies in-flight processing). Returns nil if none match.
func (q *MessageQueue) PopAllEditable(agentID string) []QueuedCommand {
	q.mu.Lock()
	defer q.mu.Unlock()

	var popped []QueuedCommand
	remaining := q.items[:0]
	for _, cmd := range q.items {
		if cmd.AgentID == agentID && cmd.Priority >= PriorityNext {
			popped = append(popped, cmd)
		} else {
			remaining = append(remaining, cmd)
		}
	}

	if len(popped) == 0 {
		return nil
	}

	q.items = remaining
	q.signal.Broadcast()
	return popped
}

// Snapshot returns a copy of all queued commands (all agents, all priorities).
// The returned slice is safe to read without holding any lock.
func (q *MessageQueue) Snapshot() []QueuedCommand {
	q.mu.Lock()
	defer q.mu.Unlock()

	snap := make([]QueuedCommand, len(q.items))
	copy(snap, q.items)
	return snap
}

// Len returns the total number of queued commands.
func (q *MessageQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Subscribe returns a channel that wakes on any queue mutation.
// Caller must call Unsubscribe(id) when done to prevent resource leaks.
func (q *MessageQueue) Subscribe() (<-chan struct{}, uint64) {
	return q.signal.Subscribe()
}

// Unsubscribe removes a subscription by ID.
func (q *MessageQueue) Unsubscribe(id uint64) {
	q.signal.Unsubscribe(id)
}

// insertSorted inserts cmd into q.items maintaining priority order.
// Within the same priority, the new command is placed after all existing
// commands of that priority (FIFO within tier). Caller must hold q.mu.
func (q *MessageQueue) insertSorted(cmd QueuedCommand) {
	pos := sort.Search(len(q.items), func(i int) bool {
		return q.items[i].Priority > cmd.Priority
	})
	// Insert at pos.
	q.items = append(q.items, QueuedCommand{}) // grow
	copy(q.items[pos+1:], q.items[pos:])
	q.items[pos] = cmd
}
