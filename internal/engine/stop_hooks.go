package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// StopHookContext carries the context available to stop hooks after a turn ends.
type StopHookContext struct {
	// Messages is the conversation history at the point the turn ended.
	// This is a defensive copy — hooks may read but should not mutate.
	Messages []types.Message
	// ToolUseContext is the tool execution context from the completed turn.
	ToolUseContext *tools.UseContext
	// QuerySource is the source tag of the completed query (e.g. "foreground").
	QuerySource string
	// IsBareMode indicates whether the session is in bare/simple mode.
	IsBareMode bool
	// Engine is the query engine, available for forked agent operations.
	Engine QueryEngine
	// CacheParams are the cache-safe parameters for forked agents.
	CacheParams *CacheSafeParams
}

// StopHookFn is the function signature for a stop hook callback.
// Stop hooks run in the background (goroutine) and must not block the caller.
type StopHookFn func(ctx context.Context, hookCtx *StopHookContext)

// StopHookRegistry manages registered stop hooks.
// It is safe for concurrent use.
type StopHookRegistry struct {
	mu    sync.RWMutex
	hooks []registeredHook
}

// registeredHook pairs a hook function with a descriptive name for logging.
type registeredHook struct {
	name string
	fn   StopHookFn
}

// NewStopHookRegistry creates a new empty StopHookRegistry.
func NewStopHookRegistry() *StopHookRegistry {
	return &StopHookRegistry{}
}

// Register adds a named stop hook to the registry.
// Hooks are executed in registration order.
func (r *StopHookRegistry) Register(name string, fn StopHookFn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = append(r.hooks, registeredHook{name: name, fn: fn})
}

// Execute runs all registered stop hooks in background goroutines.
// It does NOT block the caller — each hook runs in its own goroutine.
// A detached context (context.Background) is used so hooks survive the
// parent query's context cancellation.
// Panics in individual hooks are recovered to prevent crashing the process.
func (r *StopHookRegistry) Execute(hookCtx *StopHookContext) {
	r.mu.RLock()
	// Snapshot the hooks slice so we can release the lock before launching goroutines.
	snapshot := make([]registeredHook, len(r.hooks))
	copy(snapshot, r.hooks)
	r.mu.RUnlock()

	// Create a defensive copy of messages so hooks cannot mutate the original.
	msgCopy := make([]types.Message, len(hookCtx.Messages))
	copy(msgCopy, hookCtx.Messages)

	// Build a new StopHookContext with the copied messages.
	safeCtx := &StopHookContext{
		Messages:       msgCopy,
		ToolUseContext: hookCtx.ToolUseContext,
		QuerySource:    hookCtx.QuerySource,
		IsBareMode:     hookCtx.IsBareMode,
		Engine:         hookCtx.Engine,
		CacheParams:    hookCtx.CacheParams,
	}

	for _, h := range snapshot {
		go runHookSafe(h.name, h.fn, safeCtx)
	}
}

// runHookSafe executes a single stop hook with panic recovery.
func runHookSafe(name string, fn StopHookFn, hookCtx *StopHookContext) {
	defer func() {
		if r := recover(); r != nil {
			// Log the panic but do not propagate — individual hook failures
			// must not crash the main process.
			_ = fmt.Errorf("engine: stop hook %q panicked: %v", name, r)
		}
	}()

	// Use a detached context so the hook is not cancelled when the query ends.
	ctx := context.Background()
	fn(ctx, hookCtx)
}

// Len returns the number of registered hooks.
func (r *StopHookRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.hooks)
}

// fireStopHooks is a convenience method on engineImpl that creates the
// StopHookContext and fires registered stop hooks. It is a no-op if no hooks
// are registered or if the stopHooks registry is nil.
func (e *engineImpl) fireStopHooks(params QueryParams, messages []types.Message) {
	if e.stopHooks == nil || e.stopHooks.Len() == 0 {
		return
	}

	cacheParams := CreateCacheSafeParams(params, messages)
	hookCtx := &StopHookContext{
		Messages:       messages,
		ToolUseContext: params.ToolUseContext,
		QuerySource:    params.QuerySource,
		Engine:         e,
		CacheParams:    cacheParams,
	}
	e.stopHooks.Execute(hookCtx)
}
