// Package state provides a thread-safe generic state container and the
// application-level AppState store.
package state

import (
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/anthropics/claude-code-go/internal/config"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// Listener is a callback invoked when the store state changes.
type Listener[T any] func(newState, oldState T)

// Store is a thread-safe generic state container.
//
// Concurrency contract:
//   - GetState acquires a read-lock and returns a snapshot.
//   - SetState acquires a write-lock, runs the updater, stores the result,
//     releases the lock, then notifies listeners (outside the lock to prevent
//     deadlocks when a listener calls back into the store).
//   - Subscribe/Unsubscribe are protected by a separate mutex so they never
//     block or are blocked by state reads/writes.
type Store[T any] struct {
	mu    sync.RWMutex
	state T

	listenerMu sync.Mutex
	listeners  map[uint64]Listener[T]
	nextID     atomic.Uint64

	onChange func(newState, oldState T)
}

// NewStore creates a Store initialised with initialState.
// onChange is called after every SetState (outside the lock); it may be nil.
func NewStore[T any](initialState T, onChange func(newState, oldState T)) *Store[T] {
	return &Store[T]{
		state:    initialState,
		listeners: make(map[uint64]Listener[T]),
		onChange: onChange,
	}
}

// GetState returns a snapshot of the current state (read-lock protected).
func (s *Store[T]) GetState() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// SetState runs updater under the write lock and notifies all subscribers.
// The updater MUST NOT call GetState or SetState — that would deadlock.
func (s *Store[T]) SetState(updater func(prev T) T) {
	s.mu.Lock()
	prev := s.state
	next := updater(prev)
	s.state = next
	s.mu.Unlock()

	// Notify outside the lock to prevent subscriber-induced deadlocks.
	if s.onChange != nil {
		s.onChange(next, prev)
	}
	s.notifyListeners(next, prev)
}

// Subscribe registers a state-change listener and returns an unsubscribe function.
// The returned function removes the listener; calling it more than once is safe.
func (s *Store[T]) Subscribe(l Listener[T]) func() {
	id := s.nextID.Add(1)
	s.listenerMu.Lock()
	s.listeners[id] = l
	s.listenerMu.Unlock()

	return func() {
		s.listenerMu.Lock()
		delete(s.listeners, id)
		s.listenerMu.Unlock()
	}
}

func (s *Store[T]) notifyListeners(newState, oldState T) {
	// Snapshot the listener map under the lock, then call outside.
	s.listenerMu.Lock()
	snapshot := make([]Listener[T], 0, len(s.listeners))
	for _, l := range s.listeners {
		snapshot = append(snapshot, l)
	}
	s.listenerMu.Unlock()

	for _, l := range snapshot {
		l(newState, oldState)
	}
}

// ---------------------------------------------------------------------------
// AppState
// ---------------------------------------------------------------------------

// ModelSetting describes the active model configuration.
type ModelSetting struct {
	ModelID  string `json:"modelId"`
	Provider string `json:"provider,omitempty"` // anthropic | bedrock | vertex
}

// TaskState represents the runtime status of a single Task (sub-agent).
type TaskState struct {
	AgentId   types.AgentId   `json:"agentId"`
	Status    string          `json:"status"` // running | done | error
	SessionId types.SessionId `json:"sessionId,omitempty"`
}

// AppState is the global application state.  All fields are accessed through
// AppStateStore's RWMutex; never access them directly from multiple goroutines.
//
// Copy-on-write contract for map and slice fields:
//
//	store.SetState(func(prev AppState) AppState {
//	    prev.Tasks = maps.Clone(prev.Tasks)
//	    prev.Tasks[id] = newTask
//	    return prev
//	})
//
// This prevents the previous snapshot (held by subscribers) from being
// mutated while a new SetState call is in progress.
type AppState struct {
	// Configuration and model
	Settings              config.SettingsJson         `json:"settings"`
	Verbose               bool                        `json:"verbose"`
	MainLoopModel         ModelSetting                `json:"mainLoopModel"`
	ToolPermissionContext types.ToolPermissionContext `json:"toolPermissionContext"`

	// Session
	SessionId  types.SessionId `json:"sessionId"`
	WorkingDir string          `json:"workingDir"`
	GitBranch  string          `json:"gitBranch,omitempty"`

	// Task tree (sub-agents).  Copy-on-write; clone before mutating.
	Tasks map[string]TaskState `json:"tasks,omitempty"`

	// AgentId registry (name → AgentId).  Copy-on-write; clone before mutating.
	AgentNameRegistry map[string]types.AgentId `json:"agentNameRegistry,omitempty"`

	// MCP clients/tools/commands.  Use MCPConnection interface for type safety.
	MCPClients  []types.MCPConnection `json:"-"`
	MCPTools    []any                 `json:"-"` // TODO(dep): typed once Agent-MCP defines the Tool interface
	MCPCommands []any                 `json:"-"` // TODO(dep): typed once Agent-MCP defines the Command interface

	// Plugins
	PluginsEnabled  []types.LoadedPlugin `json:"-"`
	PluginsDisabled []types.LoadedPlugin `json:"-"`

	// UI / TUI state
	IsLoading        bool   `json:"isLoading"`
	InputPlaceholder string `json:"inputPlaceholder,omitempty"`
}

// AppStateStore is the concrete type alias for Store[AppState].
type AppStateStore = Store[AppState]

// NewAppStateStore creates an initialised AppStateStore.
func NewAppStateStore(initial AppState) *AppStateStore {
	return NewStore(initial, nil)
}

// GetDefaultAppState returns a zero-value AppState with all maps and slices
// initialised (mirrors the TypeScript getDefaultAppState()).
func GetDefaultAppState() AppState {
	return AppState{
		Tasks:             make(map[string]TaskState),
		AgentNameRegistry: make(map[string]types.AgentId),
		ToolPermissionContext: types.ToolPermissionContext{
			Mode:                         types.PermissionModeDefault,
			AdditionalWorkingDirectories: make(map[string]types.AdditionalWorkingDirectory),
			AlwaysAllowRules:             make(types.ToolPermissionRulesBySource),
			AlwaysDenyRules:              make(types.ToolPermissionRulesBySource),
			AlwaysAskRules:               make(types.ToolPermissionRulesBySource),
		},
	}
}

// ---------------------------------------------------------------------------
// AppState implements types.AppStateReader so it can be passed as a read-only
// view to lower-layer modules (commands, tools) without exposing mutation.
// ---------------------------------------------------------------------------

// appStateSnapshot is a value-copy of AppState that satisfies AppStateReader.
// It is returned by AppStateStore.Snapshot() for read-only consumers.
type appStateSnapshot struct {
	permCtx types.ToolPermissionContext
	model   string
	verbose bool
}

func (a appStateSnapshot) GetPermissionContext() types.ToolPermissionContext { return a.permCtx }
func (a appStateSnapshot) GetModel() string                                  { return a.model }
func (a appStateSnapshot) GetVerbose() bool                                  { return a.verbose }

// Snapshot returns a read-only AppStateReader view of the current state.
// Safe to call from any goroutine.
func Snapshot(store *AppStateStore) types.AppStateReader {
	s := store.GetState()
	return appStateSnapshot{
		permCtx: s.ToolPermissionContext,
		model:   s.MainLoopModel.ModelID,
		verbose: s.Verbose,
	}
}

// ---------------------------------------------------------------------------
// Copy-on-write helpers (re-exported from std libs for convenience).
// ---------------------------------------------------------------------------

// CloneTasks returns a shallow clone of the tasks map for copy-on-write updates.
func CloneTasks(m map[string]TaskState) map[string]TaskState {
	return maps.Clone(m)
}

// CloneAgentRegistry returns a shallow clone of the agent name registry.
func CloneAgentRegistry(m map[string]types.AgentId) map[string]types.AgentId {
	return maps.Clone(m)
}

// CloneStringSlice returns a shallow clone of a string slice.
func CloneStringSlice(s []string) []string {
	return slices.Clone(s)
}
