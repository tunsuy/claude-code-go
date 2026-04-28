package msgqueue

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestGuard_HappyPath(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	// Idle → Dispatching
	gen, err := g.Reserve()
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	assertState(t, g, Dispatching)

	// Dispatching → Running
	if err := g.TryStart(gen); err != nil {
		t.Fatalf("TryStart: %v", err)
	}
	assertState(t, g, Running)

	// Running → Idle
	if err := g.End(gen); err != nil {
		t.Fatalf("End: %v", err)
	}
	assertState(t, g, Idle)
}

func TestGuard_ReserveWhenNotIdle(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	_, err := g.Reserve()
	if err != nil {
		t.Fatalf("first Reserve: %v", err)
	}

	_, err = g.Reserve()
	if !errors.Is(err, ErrNotIdle) {
		t.Fatalf("expected ErrNotIdle, got %v", err)
	}
}

func TestGuard_CancelReservation(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	gen, _ := g.Reserve()
	assertState(t, g, Dispatching)

	if err := g.CancelReservation(gen); err != nil {
		t.Fatalf("CancelReservation: %v", err)
	}
	assertState(t, g, Idle)
}

func TestGuard_CancelReservation_StaleGen(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	gen, _ := g.Reserve()
	g.ForceEnd() // bumps generation

	err := g.CancelReservation(gen)
	if !errors.Is(err, ErrGenerationMismatch) {
		t.Fatalf("expected ErrGenerationMismatch, got %v", err)
	}
}

func TestGuard_TryStart_StaleGen(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	gen, _ := g.Reserve()
	g.ForceEnd()

	// Now try with old gen.
	_, _ = g.Reserve() // get new reservation
	err := g.TryStart(gen)
	if !errors.Is(err, ErrGenerationMismatch) {
		t.Fatalf("expected ErrGenerationMismatch, got %v", err)
	}
}

func TestGuard_End_StaleGen(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	gen, _ := g.Reserve()
	if err := g.TryStart(gen); err != nil {
		t.Fatalf("TryStart: %v", err)
	}
	g.ForceEnd() // bumps generation

	err := g.End(gen)
	if !errors.Is(err, ErrGenerationMismatch) {
		t.Fatalf("expected ErrGenerationMismatch, got %v", err)
	}
}

func TestGuard_ForceEnd_FromAnyState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*QueryGuard)
	}{
		{
			name:  "from idle",
			setup: func(g *QueryGuard) {},
		},
		{
			name: "from dispatching",
			setup: func(g *QueryGuard) {
				_, _ = g.Reserve()
			},
		},
		{
			name: "from running",
			setup: func(g *QueryGuard) {
				gen, _ := g.Reserve()
				_ = g.TryStart(gen)
			},
		},
	}

	for _, tt := range tests {
		tt := tt // capture loop variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewQueryGuard()
			tt.setup(g)

			g.ForceEnd()
			assertState(t, g, Idle)
		})
	}
}

func TestGuard_ForceEnd_InvalidatesOldGen(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	gen, _ := g.Reserve()
	if err := g.TryStart(gen); err != nil {
		t.Fatalf("TryStart: %v", err)
	}

	// Simulate abort — ForceEnd bumps generation.
	g.ForceEnd()

	// Old goroutine's End should fail.
	err := g.End(gen)
	if !errors.Is(err, ErrGenerationMismatch) {
		t.Fatalf("expected ErrGenerationMismatch, got %v", err)
	}
}

func TestGuard_IsActive(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	if g.IsActive() {
		t.Fatal("expected not active in Idle")
	}

	gen, _ := g.Reserve()
	if !g.IsActive() {
		t.Fatal("expected active in Dispatching")
	}

	if err := g.TryStart(gen); err != nil {
		t.Fatalf("TryStart: %v", err)
	}
	if !g.IsActive() {
		t.Fatal("expected active in Running")
	}

	if err := g.End(gen); err != nil {
		t.Fatalf("End: %v", err)
	}
	if g.IsActive() {
		t.Fatal("expected not active after End")
	}
}

func TestGuard_Subscribe(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()
	ch, id := g.Subscribe()
	defer g.Unsubscribe(id)

	_, _ = g.Reserve()

	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no notification on Reserve")
	}
}

func TestGuard_ConcurrentReserve(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	const goroutines = 20
	successes := make(chan uint64, goroutines)
	failures := make(chan error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			gen, err := g.Reserve()
			if err != nil {
				failures <- err
			} else {
				successes <- gen
			}
		}()
	}
	wg.Wait()
	close(successes)
	close(failures)

	successCount := 0
	for range successes {
		successCount++
	}

	if successCount != 1 {
		t.Fatalf("expected exactly 1 successful Reserve, got %d", successCount)
	}
}

func TestGuard_GenerationIncrementsOnEnd(t *testing.T) {
	t.Parallel()
	g := NewQueryGuard()

	_, gen0 := g.State()

	gen, _ := g.Reserve()
	_ = g.TryStart(gen)
	_ = g.End(gen)

	_, gen1 := g.State()
	if gen1 != gen0+1 {
		t.Fatalf("expected generation %d, got %d", gen0+1, gen1)
	}
}

// assertState is a test helper that checks the guard's current state.
func assertState(t *testing.T, g *QueryGuard, expected GuardState) {
	t.Helper()
	state, _ := g.State()
	if state != expected {
		t.Fatalf("expected state %s, got %s", expected, state)
	}
}
