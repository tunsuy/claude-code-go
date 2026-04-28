// Package msgqueue implements a unified command queue and query guard for
// mid-session message processing. It provides thread-safe priority-ordered
// queueing, a multi-subscriber pub-sub broadcast primitive (Signal), and a
// three-state query dispatch guard (QueryGuard).
//
// This package has zero dependencies on other internal packages (tui, engine,
// etc.) — it depends only on the standard library. Both the TUI and engine
// layers may import it without creating circular dependencies.
package msgqueue

import "sync"

// Signal is a lightweight multi-subscriber broadcast primitive.
//
// Each subscriber receives a buffered(1) channel. When Broadcast is called,
// a non-blocking send is attempted on every subscriber channel. Multiple
// broadcasts between reads coalesce into a single wakeup — exactly the
// "level-triggered" semantic needed for queue change notifications.
//
// Zero value is NOT usable; create via NewSignal().
type Signal struct {
	mu   sync.Mutex
	subs map[uint64]chan struct{}
	seq  uint64
}

// NewSignal creates a ready-to-use Signal with no subscribers.
func NewSignal() *Signal {
	return &Signal{
		subs: make(map[uint64]chan struct{}),
	}
}

// Subscribe registers a new listener and returns:
//   - ch: a buffered(1) channel that receives struct{}{} on each Broadcast.
//     Multiple broadcasts between reads coalesce into one pending wakeup.
//   - id: a unique subscriber ID for use with Unsubscribe.
//
// The caller must call Unsubscribe(id) when done to prevent resource leaks.
func (s *Signal) Subscribe() (ch <-chan struct{}, id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	id = s.seq
	c := make(chan struct{}, 1)
	s.subs[id] = c
	return c, id
}

// Unsubscribe removes a subscriber by ID and closes its channel.
// Safe to call multiple times or with an unknown ID (no-op).
// Closing the channel ensures any goroutine blocked on <-ch gets a zero-value
// wakeup before the channel becomes unreachable.
func (s *Signal) Unsubscribe(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ch, ok := s.subs[id]; ok {
		close(ch)
		delete(s.subs, id)
	}
}

// Broadcast wakes all current subscribers. Non-blocking: if a subscriber's
// channel already has a pending value, it is skipped (the subscriber already
// has an unread wakeup, so no information is lost).
func (s *Signal) Broadcast() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
			// Channel already has a pending wakeup — coalesce.
		}
	}
}

// Len returns the current number of subscribers. Intended for testing and
// debugging; callers should not rely on the count being stable across calls.
func (s *Signal) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}
