package msgqueue

import (
	"sync"
	"testing"
	"time"
)

func TestSignal_SubscribeBroadcastReceive(t *testing.T) {
	t.Parallel()
	s := NewSignal()
	ch, id := s.Subscribe()
	defer s.Unsubscribe(id)

	s.Broadcast()

	select {
	case <-ch:
		// OK — received broadcast.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for broadcast")
	}
}

func TestSignal_MultipleSubscribers(t *testing.T) {
	t.Parallel()
	s := NewSignal()

	const n = 5
	chs := make([]<-chan struct{}, n)
	ids := make([]uint64, n)
	for i := 0; i < n; i++ {
		chs[i], ids[i] = s.Subscribe()
	}
	defer func() {
		for _, id := range ids {
			s.Unsubscribe(id)
		}
	}()

	if s.Len() != n {
		t.Fatalf("expected %d subscribers, got %d", n, s.Len())
	}

	s.Broadcast()

	for i, ch := range chs {
		select {
		case <-ch:
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d timed out waiting for broadcast", i)
		}
	}
}

func TestSignal_Coalescing(t *testing.T) {
	t.Parallel()
	s := NewSignal()
	ch, id := s.Subscribe()
	defer s.Unsubscribe(id)

	// Multiple broadcasts without reading should coalesce into one pending value.
	for i := 0; i < 10; i++ {
		s.Broadcast()
	}

	// First read succeeds.
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("first read timed out")
	}

	// Second read should block (no more pending values).
	select {
	case <-ch:
		t.Fatal("unexpected second value — coalescing failed")
	case <-time.After(50 * time.Millisecond):
		// OK — no more values, coalescing works.
	}
}

func TestSignal_Unsubscribe(t *testing.T) {
	t.Parallel()
	s := NewSignal()
	ch, id := s.Subscribe()

	if s.Len() != 1 {
		t.Fatalf("expected 1 subscriber, got %d", s.Len())
	}

	s.Unsubscribe(id)

	if s.Len() != 0 {
		t.Fatalf("expected 0 subscribers after unsubscribe, got %d", s.Len())
	}

	// Channel should be closed — reading returns zero value immediately.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel should be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel should be closed and readable")
	}

	// Double unsubscribe is a no-op.
	s.Unsubscribe(id) // should not panic
}

func TestSignal_UnsubscribeUnknownID(t *testing.T) {
	t.Parallel()
	s := NewSignal()
	s.Unsubscribe(999) // should not panic
}

func TestSignal_ConcurrentBroadcast(t *testing.T) {
	t.Parallel()
	s := NewSignal()
	ch, id := s.Subscribe()
	defer s.Unsubscribe(id)

	var wg sync.WaitGroup
	const goroutines = 20
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			s.Broadcast()
		}()
	}
	wg.Wait()

	// At least one broadcast should have landed.
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no broadcast received from concurrent goroutines")
	}
}

func TestSignal_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	t.Parallel()
	s := NewSignal()

	var wg sync.WaitGroup
	const goroutines = 20
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, subID := s.Subscribe()
			s.Broadcast()
			s.Unsubscribe(subID)
		}()
	}
	wg.Wait()

	if s.Len() != 0 {
		t.Fatalf("expected 0 subscribers after all unsubscribed, got %d", s.Len())
	}
}
