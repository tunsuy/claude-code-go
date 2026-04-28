package msgqueue

import (
	"errors"
	"fmt"
	"sync"
)

// Sentinel errors returned by QueryGuard methods.
var (
	// ErrNotIdle is returned when Reserve is called in a non-Idle state.
	ErrNotIdle = errors.New("guard: not idle")
	// ErrNotDispatching is returned when TryStart is called in a non-Dispatching state.
	ErrNotDispatching = errors.New("guard: not in dispatching state")
	// ErrGenerationMismatch is returned when a stale goroutine attempts a
	// transition with an outdated generation number.
	ErrGenerationMismatch = errors.New("guard: generation mismatch (stale goroutine)")
)

// GuardState represents the current phase of the query lifecycle.
type GuardState int

const (
	// Idle means no query is running and no dispatch is pending.
	Idle GuardState = iota
	// Dispatching means a goroutine has claimed the right to start a query
	// but hasn't begun streaming yet. This covers the synchronous gap between
	// dequeue and the async engine.Query() call.
	Dispatching
	// Running means a query is actively streaming tokens.
	Running
)

// String returns a human-readable label for the guard state.
func (s GuardState) String() string {
	switch s {
	case Idle:
		return "idle"
	case Dispatching:
		return "dispatching"
	case Running:
		return "running"
	default:
		return fmt.Sprintf("GuardState(%d)", int(s))
	}
}

// QueryGuard is a three-state machine that serializes query dispatch.
//
// State transitions:
//
//	Idle ──Reserve()──▶ Dispatching ──TryStart()──▶ Running ──End(gen)──▶ Idle
//	                    CancelReservation(gen)──▶ Idle
//	                    Any ──ForceEnd()──▶ Idle (bumps generation)
//
// The generation counter prevents a slow goroutine (whose query was cancelled
// via ForceEnd) from corrupting state when it finally calls End with a stale
// generation number.
type QueryGuard struct {
	mu    sync.Mutex
	state GuardState
	gen   uint64
	sig   *Signal
}

// NewQueryGuard creates a guard in Idle state at generation 0.
func NewQueryGuard() *QueryGuard {
	return &QueryGuard{
		sig: NewSignal(),
	}
}

// Reserve atomically transitions Idle → Dispatching.
// Returns the current generation number on success.
// Returns ErrNotIdle if the guard is not in Idle state.
func (g *QueryGuard) Reserve() (uint64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.state != Idle {
		return 0, ErrNotIdle
	}
	g.state = Dispatching
	g.sig.Broadcast()
	return g.gen, nil
}

// CancelReservation transitions Dispatching → Idle if gen matches the current
// generation. Returns ErrGenerationMismatch if the generation has changed
// (meaning ForceEnd was called in between).
func (g *QueryGuard) CancelReservation(gen uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if gen != g.gen {
		return ErrGenerationMismatch
	}
	if g.state != Dispatching {
		return fmt.Errorf("guard: CancelReservation called in state %s", g.state)
	}
	g.state = Idle
	g.sig.Broadcast()
	return nil
}

// TryStart transitions Dispatching → Running if gen matches.
// Returns ErrGenerationMismatch if stale, ErrNotDispatching if state is wrong.
func (g *QueryGuard) TryStart(gen uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if gen != g.gen {
		return ErrGenerationMismatch
	}
	if g.state != Dispatching {
		return ErrNotDispatching
	}
	g.state = Running
	g.sig.Broadcast()
	return nil
}

// End transitions Running → Idle if gen matches the current generation.
// Increments the generation counter. Returns ErrGenerationMismatch if the
// generation has changed (e.g., due to ForceEnd from an abort).
func (g *QueryGuard) End(gen uint64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if gen != g.gen {
		return ErrGenerationMismatch
	}
	if g.state != Running {
		return fmt.Errorf("guard: End called in state %s", g.state)
	}
	g.state = Idle
	g.gen++
	g.sig.Broadcast()
	return nil
}

// ForceEnd unconditionally transitions to Idle and bumps the generation.
// Used by abort/interrupt to ensure any stale goroutine with an older
// generation cannot affect the guard. Always succeeds.
func (g *QueryGuard) ForceEnd() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.state = Idle
	g.gen++
	g.sig.Broadcast()
}

// IsActive returns true if the guard is in Dispatching or Running state.
func (g *QueryGuard) IsActive() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state != Idle
}

// State returns the current (state, generation) atomically.
func (g *QueryGuard) State() (GuardState, uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state, g.gen
}

// Subscribe returns a channel notified on every state change.
// Caller must call Unsubscribe(id) when done.
func (g *QueryGuard) Subscribe() (<-chan struct{}, uint64) {
	return g.sig.Subscribe()
}

// Unsubscribe removes a state-change subscription.
func (g *QueryGuard) Unsubscribe(id uint64) {
	g.sig.Unsubscribe(id)
}
