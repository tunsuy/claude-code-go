package state_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/anthropics/claude-code-go/internal/state"
)

// TestStore_GetSetBasic verifies that SetState updates visible via GetState.
func TestStore_GetSetBasic(t *testing.T) {
	s := state.NewStore(0, nil)
	s.SetState(func(prev int) int { return prev + 1 })
	if got := s.GetState(); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

// TestStore_SubscribeUnsubscribe verifies that Subscribe delivers updates and
// the returned unsubscribe cancels further delivery.
func TestStore_SubscribeUnsubscribe(t *testing.T) {
	s := state.NewStore(0, nil)

	var calls int32
	unsub := s.Subscribe(func(newState, _ int) {
		atomic.AddInt32(&calls, 1)
	})

	s.SetState(func(prev int) int { return prev + 1 }) // calls should be 1
	unsub()
	s.SetState(func(prev int) int { return prev + 1 }) // should NOT increment calls

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 call before unsubscribe, got %d", got)
	}
}

// TestStore_MultipleSubscribers verifies that all active listeners receive the
// notification.
func TestStore_MultipleSubscribers(t *testing.T) {
	s := state.NewStore(0, nil)

	var wg sync.WaitGroup
	n := 5
	wg.Add(n)
	for i := 0; i < n; i++ {
		s.Subscribe(func(_, _ int) { wg.Done() })
	}
	s.SetState(func(prev int) int { return prev + 1 })
	wg.Wait()
}

// TestStore_UnsubscribeIdempotent verifies that calling unsub multiple times
// does not panic.
func TestStore_UnsubscribeIdempotent(t *testing.T) {
	s := state.NewStore(0, nil)
	unsub := s.Subscribe(func(_, _ int) {})
	unsub()
	unsub() // second call must not panic
}

// TestStore_ConcurrentSetState is the race-detector test: multiple goroutines
// calling SetState concurrently must not trigger data-race warnings.
// Run with: go test -race ./internal/state/...
func TestStore_ConcurrentSetState(t *testing.T) {
	s := state.NewStore(0, nil)

	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			s.SetState(func(prev int) int { return prev + 1 })
		}()
	}
	wg.Wait()

	// Value should be exactly n (each SetState is serialised by the write lock).
	if got := s.GetState(); got != n {
		t.Fatalf("expected %d, got %d", n, got)
	}
}

// TestStore_OnChange verifies that the onChange callback supplied to NewStore
// is invoked after every SetState.
func TestStore_OnChange(t *testing.T) {
	var changes int32
	s := state.NewStore(0, func(_, _ int) {
		atomic.AddInt32(&changes, 1)
	})
	s.SetState(func(prev int) int { return prev + 1 })
	s.SetState(func(prev int) int { return prev + 1 })
	if got := atomic.LoadInt32(&changes); got != 2 {
		t.Fatalf("expected onChange to be called 2 times, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// AppState copy-on-write tests
// ---------------------------------------------------------------------------

// TestAppState_CopyOnWrite verifies P0-3: SetState with tasks map must not
// mutate a snapshot held by a concurrent reader.
func TestAppState_CopyOnWrite(t *testing.T) {
	s := state.NewAppStateStore(state.GetDefaultAppState())

	// Take a snapshot before the mutation.
	before := s.GetState()

	// Mutate in SetState using copy-on-write.
	s.SetState(func(prev state.AppState) state.AppState {
		prev.Tasks = state.CloneTasks(prev.Tasks)
		prev.Tasks["t1"] = state.TaskState{Status: "running"}
		return prev
	})

	// The before snapshot must not see the new task (copy-on-write guarantee).
	if _, ok := before.Tasks["t1"]; ok {
		t.Fatal("copy-on-write violation: old snapshot was mutated by SetState")
	}

	// The current state should have the task.
	after := s.GetState()
	if _, ok := after.Tasks["t1"]; !ok {
		t.Fatal("new task not visible in GetState after SetState")
	}
}

// TestGetDefaultAppState verifies all maps are non-nil.
func TestGetDefaultAppState(t *testing.T) {
	a := state.GetDefaultAppState()
	if a.Tasks == nil {
		t.Error("Tasks must be non-nil")
	}
	if a.AgentNameRegistry == nil {
		t.Error("AgentNameRegistry must be non-nil")
	}
	if a.ToolPermissionContext.AdditionalWorkingDirectories == nil {
		t.Error("AdditionalWorkingDirectories must be non-nil")
	}
	if a.ToolPermissionContext.AlwaysAllowRules == nil {
		t.Error("AlwaysAllowRules must be non-nil")
	}
}

// TestSnapshot verifies that Snapshot returns a consistent read-only view.
func TestSnapshot(t *testing.T) {
	initial := state.GetDefaultAppState()
	initial.Verbose = true
	initial.MainLoopModel = state.ModelSetting{ModelID: "claude-opus-4-5"}
	s := state.NewAppStateStore(initial)

	snap := state.Snapshot(s)
	if !snap.GetVerbose() {
		t.Error("expected Verbose=true from snapshot")
	}
	if snap.GetModel() != "claude-opus-4-5" {
		t.Errorf("expected model %q, got %q", "claude-opus-4-5", snap.GetModel())
	}
}
