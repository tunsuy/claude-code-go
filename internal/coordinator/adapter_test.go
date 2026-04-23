package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// TestNewAgentCoordinator_NilInput verifies nil returns nil.
func TestNewAgentCoordinator_NilInput(t *testing.T) {
	t.Parallel()
	if got := NewAgentCoordinator(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestNewAgentCoordinator_ValidCoordinator verifies adapter creation.
func TestNewAgentCoordinator_ValidCoordinator(t *testing.T) {
	t.Parallel()
	c := New(Config{CoordinatorMode: true})
	adapter := NewAgentCoordinator(c)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
}

// TestAdapterSpawnAgent tests the SpawnAgent flow through the adapter.
func TestAdapterSpawnAgent(t *testing.T) {
	t.Parallel()
	c := New(Config{
		CoordinatorMode: true,
		RunAgent: func(_ context.Context, _ AgentID, _ SpawnRequest, _ <-chan string) (string, error) {
			return "done", nil
		},
	})
	adapter := NewAgentCoordinator(c)

	ctx := context.Background()
	id, err := adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{
		Description: "test task",
		Prompt:      "do something",
	})
	if err != nil {
		t.Fatalf("SpawnAgent error: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty agent ID")
	}
}

// TestAdapterSpawnAgent_EmptyPrompt tests that empty prompt returns an error.
func TestAdapterSpawnAgent_EmptyPrompt(t *testing.T) {
	t.Parallel()
	c := New(Config{CoordinatorMode: true})
	adapter := NewAgentCoordinator(c)

	_, err := adapter.SpawnAgent(context.Background(), tools.AgentSpawnRequest{
		Prompt: "",
	})
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

// TestAdapterWaitForAgent tests that WaitForAgent blocks until the agent finishes.
func TestAdapterWaitForAgent(t *testing.T) {
	t.Parallel()
	c := New(Config{
		CoordinatorMode: true,
		RunAgent: func(_ context.Context, _ AgentID, _ SpawnRequest, _ <-chan string) (string, error) {
			time.Sleep(50 * time.Millisecond)
			return "agent result", nil
		},
	})
	adapter := NewAgentCoordinator(c)

	ctx := context.Background()
	id, err := adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{
		Prompt: "test prompt",
	})
	if err != nil {
		t.Fatalf("SpawnAgent error: %v", err)
	}

	result, err := adapter.WaitForAgent(ctx, id)
	if err != nil {
		t.Fatalf("WaitForAgent error: %v", err)
	}
	if result != "agent result" {
		t.Errorf("expected 'agent result', got %q", result)
	}
}

// TestAdapterWaitForAgent_Failed tests WaitForAgent when the agent fails.
func TestAdapterWaitForAgent_Failed(t *testing.T) {
	t.Parallel()
	c := New(Config{
		CoordinatorMode: true,
		RunAgent: func(_ context.Context, _ AgentID, _ SpawnRequest, _ <-chan string) (string, error) {
			return "", errors.New("agent error")
		},
	})
	adapter := NewAgentCoordinator(c)

	ctx := context.Background()
	id, err := adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{
		Prompt: "fail task",
	})
	if err != nil {
		t.Fatalf("SpawnAgent error: %v", err)
	}

	_, err = adapter.WaitForAgent(ctx, id)
	if err == nil {
		t.Fatal("expected error for failed agent")
	}
}

// TestAdapterWaitForAgent_ContextCancelled tests WaitForAgent with cancelled context.
func TestAdapterWaitForAgent_ContextCancelled(t *testing.T) {
	t.Parallel()
	c := New(Config{
		CoordinatorMode: true,
		RunAgent: func(ctx context.Context, _ AgentID, _ SpawnRequest, _ <-chan string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	adapter := NewAgentCoordinator(c)

	ctx, cancel := context.WithCancel(context.Background())
	id, err := adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{
		Prompt: "blocked task",
	})
	if err != nil {
		t.Fatalf("SpawnAgent error: %v", err)
	}

	// Cancel context after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err = adapter.WaitForAgent(ctx, id)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// TestAdapterSendMessage tests the SendMessage flow.
func TestAdapterSendMessage(t *testing.T) {
	t.Parallel()
	received := make(chan string, 1)
	c := New(Config{
		CoordinatorMode: true,
		RunAgent: func(ctx context.Context, _ AgentID, _ SpawnRequest, inbox <-chan string) (string, error) {
			select {
			case msg := <-inbox:
				received <- msg
				return "got: " + msg, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	})
	adapter := NewAgentCoordinator(c)

	ctx := context.Background()
	id, err := adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{
		Prompt: "wait for message",
	})
	if err != nil {
		t.Fatalf("SpawnAgent error: %v", err)
	}

	err = adapter.SendMessage(ctx, id, "hello agent")
	if err != nil {
		t.Fatalf("SendMessage error: %v", err)
	}

	select {
	case msg := <-received:
		if msg != "hello agent" {
			t.Errorf("expected 'hello agent', got %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

// TestAdapterSendMessage_UnknownAgent tests SendMessage for an unknown agent.
func TestAdapterSendMessage_UnknownAgent(t *testing.T) {
	t.Parallel()
	c := New(Config{CoordinatorMode: true})
	adapter := NewAgentCoordinator(c)

	err := adapter.SendMessage(context.Background(), "nonexistent", "hello")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

// TestAdapterStopAgent tests the StopAgent flow.
func TestAdapterStopAgent(t *testing.T) {
	t.Parallel()
	c := New(Config{
		CoordinatorMode: true,
		RunAgent: func(ctx context.Context, _ AgentID, _ SpawnRequest, _ <-chan string) (string, error) {
			<-ctx.Done()
			return "stopped", nil
		},
	})
	adapter := NewAgentCoordinator(c)

	ctx := context.Background()
	id, err := adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{
		Prompt: "long task",
	})
	if err != nil {
		t.Fatalf("SpawnAgent error: %v", err)
	}

	// Give the goroutine time to start.
	time.Sleep(20 * time.Millisecond)

	err = adapter.StopAgent(ctx, id)
	if err != nil {
		t.Fatalf("StopAgent error: %v", err)
	}

	// Verify status.
	status, err := adapter.GetAgentStatus(ctx, id)
	if err != nil {
		t.Fatalf("GetAgentStatus error: %v", err)
	}
	if status != string(AgentStatusStopped) {
		t.Errorf("expected 'killed', got %q", status)
	}
}

// TestAdapterGetAgentResult tests GetAgentResult for completed and running agents.
func TestAdapterGetAgentResult(t *testing.T) {
	t.Parallel()
	c := New(Config{
		CoordinatorMode: true,
		RunAgent: func(_ context.Context, _ AgentID, _ SpawnRequest, _ <-chan string) (string, error) {
			return "final result", nil
		},
	})
	adapter := NewAgentCoordinator(c)

	ctx := context.Background()
	id, err := adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{
		Prompt: "task",
	})
	if err != nil {
		t.Fatalf("SpawnAgent error: %v", err)
	}

	// Wait for completion.
	time.Sleep(100 * time.Millisecond)

	result, status, err := adapter.GetAgentResult(ctx, id)
	if err != nil {
		t.Fatalf("GetAgentResult error: %v", err)
	}
	if result != "final result" {
		t.Errorf("expected 'final result', got %q", result)
	}
	if status != string(AgentStatusCompleted) {
		t.Errorf("expected 'completed', got %q", status)
	}
}

// TestAdapterGetAgentResult_Unknown tests GetAgentResult for unknown agent.
func TestAdapterGetAgentResult_Unknown(t *testing.T) {
	t.Parallel()
	c := New(Config{CoordinatorMode: true})
	adapter := NewAgentCoordinator(c)

	_, _, err := adapter.GetAgentResult(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

// TestAdapterListAgents tests ListAgents.
func TestAdapterListAgents(t *testing.T) {
	t.Parallel()
	c := New(Config{
		CoordinatorMode: true,
		RunAgent: func(ctx context.Context, _ AgentID, _ SpawnRequest, _ <-chan string) (string, error) {
			<-ctx.Done()
			return "", nil
		},
	})
	adapter := NewAgentCoordinator(c)

	ctx := context.Background()
	_, err := adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{Prompt: "task1", Description: "task1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.SpawnAgent(ctx, tools.AgentSpawnRequest{Prompt: "task2", Description: "task2"})
	if err != nil {
		t.Fatal(err)
	}

	// Give goroutines time to start.
	time.Sleep(20 * time.Millisecond)

	agents := adapter.ListAgents()
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
	for id, status := range agents {
		if id == "" {
			t.Error("empty agent ID")
		}
		if status != string(AgentStatusRunning) {
			t.Errorf("expected 'running', got %q", status)
		}
	}
}
