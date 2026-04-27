package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/api"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// ─────────────────────────────────────────────────────────────────────────────
// StopHookRegistry — Register / Len
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		hookNames []string
		wantLen   int
	}{
		{
			name:      "register one hook",
			hookNames: []string{"hook-a"},
			wantLen:   1,
		},
		{
			name:      "register multiple hooks",
			hookNames: []string{"hook-a", "hook-b", "hook-c"},
			wantLen:   3,
		},
		{
			name:      "empty registry",
			hookNames: nil,
			wantLen:   0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := NewStopHookRegistry()
			for _, n := range tt.hookNames {
				reg.Register(n, func(_ context.Context, _ *StopHookContext) {})
			}
			if got := reg.Len(); got != tt.wantLen {
				t.Errorf("Len() = %d, want %d", got, tt.wantLen)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute — all hooks are called
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_Execute(t *testing.T) {
	t.Parallel()

	reg := NewStopHookRegistry()
	var called int32

	reg.Register("hook-1", func(_ context.Context, _ *StopHookContext) {
		atomic.AddInt32(&called, 1)
	})
	reg.Register("hook-2", func(_ context.Context, _ *StopHookContext) {
		atomic.AddInt32(&called, 1)
	})
	reg.Register("hook-3", func(_ context.Context, _ *StopHookContext) {
		atomic.AddInt32(&called, 1)
	})

	hookCtx := &StopHookContext{
		Messages:    []types.Message{},
		QuerySource: "foreground",
	}
	reg.Execute(hookCtx)

	// Wait for goroutines to complete.
	deadline := time.After(2 * time.Second)
	for {
		if atomic.LoadInt32(&called) == 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for hooks; called=%d, want 3", atomic.LoadInt32(&called))
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute — registration order (all hooks execute)
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_ExecuteOrder(t *testing.T) {
	t.Parallel()

	reg := NewStopHookRegistry()

	var mu sync.Mutex
	var order []string

	for _, name := range []string{"first", "second", "third"} {
		name := name
		reg.Register(name, func(_ context.Context, _ *StopHookContext) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
		})
	}

	hookCtx := &StopHookContext{Messages: []types.Message{}}
	reg.Execute(hookCtx)

	// Wait for all hooks.
	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		n := len(order)
		mu.Unlock()
		if n == 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out; got %d hooks, want 3", n)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// All three hooks must have run (order may vary due to goroutine scheduling).
	mu.Lock()
	defer mu.Unlock()
	seen := map[string]bool{}
	for _, n := range order {
		seen[n] = true
	}
	for _, want := range []string{"first", "second", "third"} {
		if !seen[want] {
			t.Errorf("hook %q was not executed", want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute — goroutine isolation (concurrency)
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_ExecuteConcurrency(t *testing.T) {
	t.Parallel()

	reg := NewStopHookRegistry()
	var wg sync.WaitGroup

	// Register hooks that block until signalled — proves goroutine isolation.
	barrier := make(chan struct{})
	const numHooks = 5

	for i := 0; i < numHooks; i++ {
		wg.Add(1)
		reg.Register("concurrent-hook", func(_ context.Context, _ *StopHookContext) {
			defer wg.Done()
			<-barrier // Block until released.
		})
	}

	hookCtx := &StopHookContext{Messages: []types.Message{}}
	reg.Execute(hookCtx)

	// Release all hooks at once.
	close(barrier)

	// All hooks must complete.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success.
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for concurrent hooks")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute — panic recovery
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_ExecutePanicRecovery(t *testing.T) {
	t.Parallel()

	reg := NewStopHookRegistry()
	var called int32

	// First hook panics.
	reg.Register("panicker", func(_ context.Context, _ *StopHookContext) {
		panic("intentional test panic")
	})

	// Second hook should still run.
	reg.Register("survivor", func(_ context.Context, _ *StopHookContext) {
		atomic.AddInt32(&called, 1)
	})

	hookCtx := &StopHookContext{Messages: []types.Message{}}
	reg.Execute(hookCtx)

	deadline := time.After(2 * time.Second)
	for {
		if atomic.LoadInt32(&called) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("survivor hook was not called; panic may have propagated")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute — bare mode context propagation
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_ExecuteBareMode(t *testing.T) {
	t.Parallel()

	reg := NewStopHookRegistry()
	var receivedBareMode int32

	reg.Register("bare-check", func(_ context.Context, hookCtx *StopHookContext) {
		if hookCtx.IsBareMode && hookCtx.QuerySource == "foreground" {
			atomic.StoreInt32(&receivedBareMode, 1)
		} else {
			atomic.StoreInt32(&receivedBareMode, -1)
		}
	})

	hookCtx := &StopHookContext{
		Messages:    []types.Message{{Role: types.RoleUser}},
		QuerySource: "foreground",
		IsBareMode:  true,
	}
	reg.Execute(hookCtx)

	deadline := time.After(2 * time.Second)
	for {
		v := atomic.LoadInt32(&receivedBareMode)
		if v != 0 {
			if v != 1 {
				t.Fatalf("hook received incorrect context; receivedBareMode=%d", v)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for bare-mode hook")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute — empty registry is safe
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_EmptyRegistry(t *testing.T) {
	t.Parallel()

	reg := NewStopHookRegistry()

	if reg.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", reg.Len())
	}

	// Execute on empty registry must not panic.
	hookCtx := &StopHookContext{Messages: []types.Message{}}
	reg.Execute(hookCtx)
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute — messages are defensively copied
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_ExecuteMessagesCopied(t *testing.T) {
	t.Parallel()

	var hookMsgs []types.Message
	var hookDone sync.WaitGroup
	hookDone.Add(1)

	reg := NewStopHookRegistry()
	reg.Register("copy_check", func(_ context.Context, hookCtx *StopHookContext) {
		defer hookDone.Done()
		hookMsgs = hookCtx.Messages
	})

	originalMsgs := []types.Message{
		{Role: types.RoleUser},
		{Role: types.RoleAssistant},
	}

	hookCtx := &StopHookContext{Messages: originalMsgs}
	reg.Execute(hookCtx)

	hookDone.Wait()

	// Mutating the original slice after Execute should not affect what the hook saw.
	originalMsgs[0].Role = "mutated"

	if hookMsgs[0].Role == "mutated" {
		t.Error("hook received the original slice reference instead of a copy")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Execute — hooks receive context.Background (not cancelled query context)
// ─────────────────────────────────────────────────────────────────────────────

func TestStopHookRegistry_ExecuteUsesDetachedContext(t *testing.T) {
	t.Parallel()

	var ctxErr error
	var mu sync.Mutex
	var hookDone sync.WaitGroup
	hookDone.Add(1)

	reg := NewStopHookRegistry()
	reg.Register("ctx_check", func(ctx context.Context, _ *StopHookContext) {
		defer hookDone.Done()
		mu.Lock()
		ctxErr = ctx.Err()
		mu.Unlock()
	})

	hookCtx := &StopHookContext{Messages: []types.Message{}}
	reg.Execute(hookCtx)

	hookDone.Wait()

	mu.Lock()
	defer mu.Unlock()
	if ctxErr != nil {
		t.Errorf("hook context should not be cancelled, got: %v", ctxErr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration: engine Config wiring
// ─────────────────────────────────────────────────────────────────────────────

func TestNew_StopHooksWired(t *testing.T) {
	t.Parallel()

	reg := NewStopHookRegistry()
	reg.Register("test", func(_ context.Context, _ *StopHookContext) {})

	eng := New(Config{
		Client:    &mockClient{},
		Registry:  tools.NewRegistry(),
		Model:     "test",
		StopHooks: reg,
	})
	impl := eng.(*engineImpl)

	if impl.stopHooks == nil {
		t.Fatal("stopHooks should be wired from Config")
	}
	if impl.stopHooks.Len() != 1 {
		t.Errorf("stopHooks.Len() = %d, want 1", impl.stopHooks.Len())
	}
}

func TestNew_StopHooksNil(t *testing.T) {
	t.Parallel()

	eng := New(Config{
		Client:   &mockClient{},
		Registry: tools.NewRegistry(),
		Model:    "test",
	})
	impl := eng.(*engineImpl)

	if impl.stopHooks != nil {
		t.Error("stopHooks should be nil when not provided in Config")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration: fireStopHooks called on end_turn
// ─────────────────────────────────────────────────────────────────────────────

func TestQuery_EndTurn_FiresStopHooks(t *testing.T) {
	var hookCalled atomic.Bool
	var hookSource atomic.Value

	reg := NewStopHookRegistry()
	reg.Register("integration_test", func(_ context.Context, hookCtx *StopHookContext) {
		hookCalled.Store(true)
		hookSource.Store(hookCtx.QuerySource)
	})

	events := buildEndTurnEvents("Hello")
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(events...), nil
		},
	}

	eng := New(Config{
		Client:    client,
		Registry:  tools.NewRegistry(),
		Model:     "test",
		StopHooks: reg,
	})

	params := QueryParams{
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("hi")},
			}},
		},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		QuerySource:    "foreground",
	}

	ch, err := eng.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	drainMsgs(ch)

	// Wait for the hook goroutine to complete.
	deadline := time.After(2 * time.Second)
	for {
		if hookCalled.Load() {
			break
		}
		select {
		case <-deadline:
			t.Fatal("stop hook was not called after end_turn")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	if src := hookSource.Load(); src != "foreground" {
		t.Errorf("QuerySource: want %q, got %v", "foreground", src)
	}
}

func TestQuery_EndTurn_NoStopHooks_NoPanic(t *testing.T) {
	// Ensure no panic when stopHooks is nil (the default).
	events := buildEndTurnEvents("done")
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(events...), nil
		},
	}

	eng := New(Config{
		Client:   client,
		Registry: tools.NewRegistry(),
		Model:    "test",
		// StopHooks intentionally nil.
	})

	params := QueryParams{
		Messages:       []types.Message{{Role: types.RoleUser}},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	ch, err := eng.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	drainMsgs(ch) // must not panic
}
