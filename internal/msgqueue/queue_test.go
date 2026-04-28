package msgqueue

import (
	"sync"
	"testing"
	"time"
)

func TestQueue_EnqueueDequeue_FIFO(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	q.Enqueue(NewCommand("first", ModePrompt, PriorityNext))
	q.Enqueue(NewCommand("second", ModePrompt, PriorityNext))
	q.Enqueue(NewCommand("third", ModePrompt, PriorityNext))

	if q.Len() != 3 {
		t.Fatalf("expected 3, got %d", q.Len())
	}

	cmd, ok := q.Dequeue("")
	if !ok || cmd.Value != "first" {
		t.Fatalf("expected first, got %q (ok=%v)", cmd.Value, ok)
	}
	cmd, ok = q.Dequeue("")
	if !ok || cmd.Value != "second" {
		t.Fatalf("expected second, got %q", cmd.Value)
	}
	cmd, ok = q.Dequeue("")
	if !ok || cmd.Value != "third" {
		t.Fatalf("expected third, got %q", cmd.Value)
	}
	_, ok = q.Dequeue("")
	if ok {
		t.Fatal("expected empty queue")
	}
}

func TestQueue_PriorityOrdering(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	q.Enqueue(NewCommand("later", ModePrompt, PriorityLater))
	q.Enqueue(NewCommand("next", ModePrompt, PriorityNext))
	q.Enqueue(NewCommand("now", ModePrompt, PriorityNow))

	cmd, _ := q.Dequeue("")
	if cmd.Value != "now" {
		t.Fatalf("expected 'now' first, got %q", cmd.Value)
	}
	cmd, _ = q.Dequeue("")
	if cmd.Value != "next" {
		t.Fatalf("expected 'next' second, got %q", cmd.Value)
	}
	cmd, _ = q.Dequeue("")
	if cmd.Value != "later" {
		t.Fatalf("expected 'later' third, got %q", cmd.Value)
	}
}

func TestQueue_PriorityWithFIFOWithinTier(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	q.Enqueue(NewCommand("next-1", ModePrompt, PriorityNext))
	q.Enqueue(NewCommand("later-1", ModePrompt, PriorityLater))
	q.Enqueue(NewCommand("next-2", ModePrompt, PriorityNext))
	q.Enqueue(NewCommand("now-1", ModePrompt, PriorityNow))

	expected := []string{"now-1", "next-1", "next-2", "later-1"}
	for i, exp := range expected {
		cmd, ok := q.Dequeue("")
		if !ok {
			t.Fatalf("step %d: unexpected empty queue", i)
		}
		if cmd.Value != exp {
			t.Fatalf("step %d: expected %q, got %q", i, exp, cmd.Value)
		}
	}
}

func TestQueue_DequeueFilterByAgentID(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	q.Enqueue(NewCommandWithAgent("main-msg", ModePrompt, PriorityNext, ""))
	q.Enqueue(NewCommandWithAgent("agent-msg", ModePrompt, PriorityNext, "agent-1"))

	// Dequeue for main session should only get main-msg.
	cmd, ok := q.Dequeue("")
	if !ok || cmd.Value != "main-msg" {
		t.Fatalf("expected main-msg, got %q (ok=%v)", cmd.Value, ok)
	}

	// Main session queue is now empty.
	_, ok = q.Dequeue("")
	if ok {
		t.Fatal("main session should be empty")
	}

	// Agent queue still has one.
	cmd, ok = q.Dequeue("agent-1")
	if !ok || cmd.Value != "agent-msg" {
		t.Fatalf("expected agent-msg, got %q", cmd.Value)
	}
}

func TestQueue_DequeueEmpty(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()
	_, ok := q.Dequeue("")
	if ok {
		t.Fatal("expected false from empty queue")
	}
}

func TestQueue_DequeueAll(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	q.Enqueue(NewCommandWithAgent("m1", ModePrompt, PriorityNext, ""))
	q.Enqueue(NewCommandWithAgent("a1", ModePrompt, PriorityNext, "agent-1"))
	q.Enqueue(NewCommandWithAgent("m2", ModePrompt, PriorityLater, ""))
	q.Enqueue(NewCommandWithAgent("a2", ModePrompt, PriorityNow, "agent-1"))

	all := q.DequeueAll("")
	if len(all) != 2 {
		t.Fatalf("expected 2 main commands, got %d", len(all))
	}
	if all[0].Value != "m1" || all[1].Value != "m2" {
		t.Fatalf("unexpected order: %v", all)
	}

	// Agent commands still in queue.
	if q.Len() != 2 {
		t.Fatalf("expected 2 remaining, got %d", q.Len())
	}
}

func TestQueue_GetByMaxPriority(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	q.Enqueue(NewCommand("now", ModePrompt, PriorityNow))
	q.Enqueue(NewCommand("next", ModePrompt, PriorityNext))
	q.Enqueue(NewCommand("later", ModePrompt, PriorityLater))

	// Only PriorityNext and above.
	result := q.GetByMaxPriority(PriorityNext, "")
	if len(result) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(result))
	}
	// Should not modify the queue.
	if q.Len() != 3 {
		t.Fatalf("peek should not remove: expected 3, got %d", q.Len())
	}
}

func TestQueue_RemoveByIDs(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	c1 := NewCommand("a", ModePrompt, PriorityNext)
	c2 := NewCommand("b", ModePrompt, PriorityNext)
	c3 := NewCommand("c", ModePrompt, PriorityNext)
	q.Enqueue(c1)
	q.Enqueue(c2)
	q.Enqueue(c3)

	removed := q.RemoveByIDs([]string{c1.ID, c3.ID})
	if removed != 2 {
		t.Fatalf("expected 2 removed, got %d", removed)
	}
	if q.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", q.Len())
	}
	cmd, _ := q.Dequeue("")
	if cmd.Value != "b" {
		t.Fatalf("expected 'b', got %q", cmd.Value)
	}
}

func TestQueue_RemoveByIDs_Empty(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()
	removed := q.RemoveByIDs(nil)
	if removed != 0 {
		t.Fatalf("expected 0, got %d", removed)
	}
}

func TestQueue_PopAllEditable(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	q.Enqueue(NewCommand("now-cmd", ModePrompt, PriorityNow))
	q.Enqueue(NewCommand("next-cmd", ModePrompt, PriorityNext))
	q.Enqueue(NewCommand("later-cmd", ModePrompt, PriorityLater))

	popped := q.PopAllEditable("")
	if len(popped) != 2 {
		t.Fatalf("expected 2 editable, got %d", len(popped))
	}
	// PriorityNow should remain.
	if q.Len() != 1 {
		t.Fatalf("expected 1 remaining (now), got %d", q.Len())
	}
}

func TestQueue_Snapshot(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	q.Enqueue(NewCommand("a", ModePrompt, PriorityNext))
	q.Enqueue(NewCommand("b", ModePrompt, PriorityNext))

	snap := q.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2, got %d", len(snap))
	}

	// Modifying snapshot should not affect queue.
	snap[0].Value = "mutated"
	cmd, _ := q.Dequeue("")
	if cmd.Value == "mutated" {
		t.Fatal("snapshot modification affected queue")
	}
}

func TestQueue_SubscribeBroadcastOnMutation(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()
	ch, id := q.Subscribe()
	defer q.Unsubscribe(id)

	// Enqueue should broadcast.
	q.Enqueue(NewCommand("test", ModePrompt, PriorityNext))

	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("enqueue did not broadcast")
	}

	// Dequeue should broadcast.
	q.Dequeue("")
	select {
	case <-ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("dequeue did not broadcast")
	}
}

func TestQueue_ConcurrentEnqueueDequeue(t *testing.T) {
	t.Parallel()
	q := NewMessageQueue()

	var wg sync.WaitGroup
	const producers = 30
	const consumers = 30

	wg.Add(producers + consumers)

	for i := 0; i < producers; i++ {
		go func(n int) {
			defer wg.Done()
			cmd := NewCommand("msg", ModePrompt, PriorityNext)
			q.Enqueue(cmd)
		}(i)
	}

	dequeued := make(chan QueuedCommand, producers)
	for i := 0; i < consumers; i++ {
		go func() {
			defer wg.Done()
			if cmd, ok := q.Dequeue(""); ok {
				dequeued <- cmd
			}
		}()
	}

	wg.Wait()
	close(dequeued)

	// Count how many were dequeued. Since producers and consumers race,
	// the exact count depends on scheduling; just verify no panics or races.
	count := 0
	for range dequeued {
		count++
	}
	// At least some should have been dequeued (the test mostly validates -race).
	t.Logf("dequeued %d of %d", count, producers)
}
