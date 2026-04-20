package coordinator_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/coordinator"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// newCoordinator creates a coordinator backed by the supplied RunAgentFn.
// If fn is nil the noop stub inside New() is used.
func newCoordinator(fn coordinator.RunAgentFn) coordinator.Coordinator {
	return coordinator.New(coordinator.Config{
		CoordinatorMode: true,
		RunAgent:        fn,
	})
}

// immediateAgent completes instantly with the given result.
func immediateAgent(result string) coordinator.RunAgentFn {
	return func(_ context.Context, _ coordinator.SpawnRequest, _ <-chan string) (string, error) {
		return result, nil
	}
}

// blockedAgent blocks until ctx is cancelled, then returns the given result.
func blockedAgent(result string) coordinator.RunAgentFn {
	return func(ctx context.Context, _ coordinator.SpawnRequest, _ <-chan string) (string, error) {
		<-ctx.Done()
		return result, nil
	}
}

// failingAgent always returns an error.
func failingAgent(errMsg string) coordinator.RunAgentFn {
	return func(_ context.Context, _ coordinator.SpawnRequest, _ <-chan string) (string, error) {
		return "", errors.New(errMsg)
	}
}

// inboxCapturingAgent reads all inbox messages then completes, returning them
// concatenated with "|".
func inboxCapturingAgent(readDeadline time.Duration) coordinator.RunAgentFn {
	return func(_ context.Context, _ coordinator.SpawnRequest, inbox <-chan string) (string, error) {
		var msgs []string
		deadline := time.After(readDeadline)
		for {
			select {
			case m, ok := <-inbox:
				if !ok {
					return strings.Join(msgs, "|"), nil
				}
				msgs = append(msgs, m)
			case <-deadline:
				return strings.Join(msgs, "|"), nil
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SpawnAgent
// ─────────────────────────────────────────────────────────────────────────────

func TestSpawnAgent_ReturnsUniqueIDs(t *testing.T) {
	c := newCoordinator(nil)
	ctx := context.Background()

	req := coordinator.SpawnRequest{Prompt: "do something"}
	id1, err1 := c.SpawnAgent(ctx, req)
	id2, err2 := c.SpawnAgent(ctx, req)

	if err1 != nil || err2 != nil {
		t.Fatalf("SpawnAgent failed: %v / %v", err1, err2)
	}
	if id1 == id2 {
		t.Errorf("expected distinct agent IDs, got %q twice", id1)
	}
}

func TestSpawnAgent_EmptyPromptReturnsError(t *testing.T) {
	c := newCoordinator(nil)
	_, err := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: ""})
	if err == nil {
		t.Fatal("expected error for empty prompt, got nil")
	}
}

func TestSpawnAgent_AgentBecomesRunning(t *testing.T) {
	// Use a blocked agent so we can query status while it is still running.
	c := newCoordinator(blockedAgent(""))

	id, err := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}

	status, err := c.GetAgentStatus(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if status != coordinator.AgentStatusRunning {
		t.Errorf("expected running, got %s", status)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GetAgentStatus
// ─────────────────────────────────────────────────────────────────────────────

func TestGetAgentStatus_CompletedAfterFinish(t *testing.T) {
	c := newCoordinator(immediateAgent("done"))

	id, err := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the agent to finish.
	ch, err := c.Subscribe(id)
	if err != nil {
		t.Fatal(err)
	}
	<-ch

	status, err := c.GetAgentStatus(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if status != coordinator.AgentStatusCompleted {
		t.Errorf("expected completed, got %s", status)
	}
}

func TestGetAgentStatus_FailedAfterAgentError(t *testing.T) {
	c := newCoordinator(failingAgent("boom"))

	id, err := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}

	ch, err := c.Subscribe(id)
	if err != nil {
		t.Fatal(err)
	}
	notif := <-ch

	if notif.Status != coordinator.AgentStatusFailed {
		t.Errorf("expected failed, got %s", notif.Status)
	}

	status, err := c.GetAgentStatus(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if status != coordinator.AgentStatusFailed {
		t.Errorf("expected failed via GetAgentStatus, got %s", status)
	}
}

func TestGetAgentStatus_UnknownIDReturnsError(t *testing.T) {
	c := newCoordinator(nil)
	_, err := c.GetAgentStatus(context.Background(), coordinator.AgentID("a-deadbeef00000000"))
	if err == nil {
		t.Fatal("expected error for unknown agent ID")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// StopAgent
// ─────────────────────────────────────────────────────────────────────────────

func TestStopAgent_TransitionsToStopped(t *testing.T) {
	c := newCoordinator(blockedAgent(""))

	id, err := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.StopAgent(context.Background(), id); err != nil {
		t.Fatalf("StopAgent error: %v", err)
	}

	// The goroutine may still be shutting down; poll until status reflects stop.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status, _ := c.GetAgentStatus(context.Background(), id)
		if status == coordinator.AgentStatusStopped || status == coordinator.AgentStatusCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("agent did not reach stopped/completed within deadline")
}

func TestStopAgent_UnknownIDReturnsError(t *testing.T) {
	c := newCoordinator(nil)
	err := c.StopAgent(context.Background(), coordinator.AgentID("a-deadbeef00000000"))
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestStopAgent_AlreadyStoppedReturnsError(t *testing.T) {
	c := newCoordinator(blockedAgent(""))

	id, _ := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})
	_ = c.StopAgent(context.Background(), id)
	// Second stop should fail.
	err := c.StopAgent(context.Background(), id)
	if err == nil {
		t.Fatal("expected error when stopping an already-stopped agent")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SendMessage
// ─────────────────────────────────────────────────────────────────────────────

func TestSendMessage_DeliveredToAgent(t *testing.T) {
	// Agent that reads one inbox message then finishes.
	received := make(chan string, 1)
	fn := func(_ context.Context, _ coordinator.SpawnRequest, inbox <-chan string) (string, error) {
		select {
		case msg := <-inbox:
			received <- msg
		case <-time.After(2 * time.Second):
		}
		return "", nil
	}

	c := newCoordinator(fn)
	id, _ := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})

	if err := c.SendMessage(context.Background(), id, "hello"); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-received:
		if msg != "hello" {
			t.Errorf("expected %q, got %q", "hello", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for agent to receive message")
	}
}

func TestSendMessage_UnknownIDReturnsError(t *testing.T) {
	c := newCoordinator(nil)
	err := c.SendMessage(context.Background(), coordinator.AgentID("a-deadbeef00000000"), "hi")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestSendMessage_FinishedAgentReturnsError(t *testing.T) {
	c := newCoordinator(immediateAgent("done"))
	id, _ := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})

	// Wait for completion.
	ch, _ := c.Subscribe(id)
	<-ch

	err := c.SendMessage(context.Background(), id, "too late")
	if err == nil {
		t.Fatal("expected error when messaging a finished agent")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Subscribe
// ─────────────────────────────────────────────────────────────────────────────

func TestSubscribe_ReceivesNotification(t *testing.T) {
	c := newCoordinator(immediateAgent("result-text"))
	id, _ := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})

	ch, err := c.Subscribe(id)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case notif := <-ch:
		if notif.TaskID != id {
			t.Errorf("expected TaskID %s, got %s", id, notif.TaskID)
		}
		if notif.Status != coordinator.AgentStatusCompleted {
			t.Errorf("expected completed, got %s", notif.Status)
		}
		if notif.Result != "result-text" {
			t.Errorf("expected result %q, got %q", "result-text", notif.Result)
		}
		if notif.Usage == nil {
			t.Error("expected non-nil Usage")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestSubscribe_MultipleSubscribers(t *testing.T) {
	c := newCoordinator(immediateAgent("multi"))
	id, _ := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})

	const n = 3
	channels := make([]<-chan coordinator.TaskNotification, n)
	for i := range channels {
		ch, err := c.Subscribe(id)
		if err != nil {
			t.Fatal(err)
		}
		channels[i] = ch
	}

	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func(ch <-chan coordinator.TaskNotification) {
			defer wg.Done()
			select {
			case notif := <-ch:
				if notif.Status != coordinator.AgentStatusCompleted {
					t.Errorf("expected completed, got %s", notif.Status)
				}
			case <-time.After(3 * time.Second):
				t.Errorf("timed out waiting for notification")
			}
		}(ch)
	}
	wg.Wait()
}

func TestSubscribe_AlreadyFinishedDeliverImmediately(t *testing.T) {
	c := newCoordinator(immediateAgent("already-done"))
	id, _ := c.SpawnAgent(context.Background(), coordinator.SpawnRequest{Prompt: "work"})

	// Wait for the agent to finish first.
	time.Sleep(100 * time.Millisecond)

	// Now subscribe — should get notification immediately.
	ch, err := c.Subscribe(id)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case notif := <-ch:
		if notif.TaskID != id {
			t.Errorf("unexpected TaskID: %s", notif.TaskID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for immediate notification on completed agent")
	}
}

func TestSubscribe_UnknownIDReturnsError(t *testing.T) {
	c := newCoordinator(nil)
	_, err := c.Subscribe(coordinator.AgentID("a-deadbeef00000000"))
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// IsCoordinatorMode
// ─────────────────────────────────────────────────────────────────────────────

func TestIsCoordinatorMode_True(t *testing.T) {
	c := coordinator.New(coordinator.Config{CoordinatorMode: true})
	if !c.IsCoordinatorMode() {
		t.Error("expected IsCoordinatorMode() == true")
	}
}

func TestIsCoordinatorMode_False(t *testing.T) {
	c := coordinator.New(coordinator.Config{CoordinatorMode: false})
	if c.IsCoordinatorMode() {
		t.Error("expected IsCoordinatorMode() == false")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Concurrent stress test
// ─────────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────────
// ListAgents (diagnostic helper)
// ─────────────────────────────────────────────────────────────────────────────

func TestListAgents_ReflectsSpawnedAgents(t *testing.T) {
	impl := coordinator.New(coordinator.Config{
		CoordinatorMode: true,
		RunAgent:        blockedAgent(""),
	})
	// coordinatorImpl exposes ListAgents; use a type assertion via a local
	// interface to avoid exporting the method on the Coordinator interface.
	type lister interface {
		ListAgents() map[coordinator.AgentID]coordinator.AgentStatus
	}
	c, ok := impl.(lister)
	if !ok {
		t.Skip("implementation does not expose ListAgents")
	}

	ctx := context.Background()
	id1, _ := impl.SpawnAgent(ctx, coordinator.SpawnRequest{Prompt: "a"})
	id2, _ := impl.SpawnAgent(ctx, coordinator.SpawnRequest{Prompt: "b"})

	agents := c.ListAgents()
	if agents[id1] != coordinator.AgentStatusRunning {
		t.Errorf("expected id1 running, got %s", agents[id1])
	}
	if agents[id2] != coordinator.AgentStatusRunning {
		t.Errorf("expected id2 running, got %s", agents[id2])
	}

	_ = impl.StopAgent(ctx, id1)
	_ = impl.StopAgent(ctx, id2)
}

func TestConcurrentSpawnAndQuery(t *testing.T) {
	c := newCoordinator(immediateAgent("ok"))
	ctx := context.Background()

	const numAgents = 20
	var wg sync.WaitGroup
	ids := make([]coordinator.AgentID, numAgents)
	var mu sync.Mutex

	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, err := c.SpawnAgent(ctx, coordinator.SpawnRequest{Prompt: "concurrent work"})
			if err != nil {
				t.Errorf("SpawnAgent %d: %v", i, err)
				return
			}
			mu.Lock()
			ids[i] = id
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// All agents should be queryable.
	for _, id := range ids {
		if id == "" {
			continue
		}
		_, err := c.GetAgentStatus(ctx, id)
		if err != nil {
			t.Errorf("GetAgentStatus(%s): %v", id, err)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Prompt helpers
// ─────────────────────────────────────────────────────────────────────────────

func TestGetCoordinatorSystemPrompt_Full(t *testing.T) {
	p := coordinator.GetCoordinatorSystemPrompt(false)
	if !strings.Contains(p, "coordinator") {
		t.Error("full prompt should mention coordinator")
	}
	if !strings.Contains(p, "AgentTool") {
		t.Error("full prompt should mention AgentTool")
	}
	if !strings.Contains(p, "task-notification") {
		t.Error("full prompt should describe task-notification protocol")
	}
}

func TestGetCoordinatorSystemPrompt_Simple(t *testing.T) {
	p := coordinator.GetCoordinatorSystemPrompt(true)
	if p == "" {
		t.Error("simple prompt must not be empty")
	}
	// Simple variant should be shorter than the full one.
	full := coordinator.GetCoordinatorSystemPrompt(false)
	if len(p) >= len(full) {
		t.Error("simple prompt should be shorter than full prompt")
	}
}

func TestGetCoordinatorUserContext_NoMCP(t *testing.T) {
	ctx := coordinator.GetCoordinatorUserContext(nil, "/tmp/scratch")
	if _, ok := ctx["worker_tools"]; !ok {
		t.Error("expected worker_tools key in user context")
	}
	if ctx["scratchpad_dir"] != "/tmp/scratch" {
		t.Errorf("unexpected scratchpad_dir: %q", ctx["scratchpad_dir"])
	}
	if _, ok := ctx["mcp_services"]; ok {
		t.Error("mcp_services should be absent when no MCP clients given")
	}
}

func TestGetCoordinatorUserContext_WithMCP(t *testing.T) {
	clients := []coordinator.MCPClientInfo{{Name: "svc-a"}, {Name: "svc-b"}}
	ctx := coordinator.GetCoordinatorUserContext(clients, "")
	if !strings.Contains(ctx["mcp_services"], "svc-a") {
		t.Errorf("mcp_services missing svc-a: %q", ctx["mcp_services"])
	}
	if _, ok := ctx["scratchpad_dir"]; ok {
		t.Error("scratchpad_dir should be absent when empty")
	}
}

func TestFormatTaskNotification(t *testing.T) {
	n := coordinator.TaskNotification{
		TaskID:  coordinator.AgentID("aworker-abcdef1234567890"),
		Status:  coordinator.AgentStatusCompleted,
		Summary: "done",
		Result:  "output <text>",
	}
	xml := coordinator.FormatTaskNotification(n)
	if !strings.Contains(xml, `status="completed"`) {
		t.Errorf("missing status: %s", xml)
	}
	if !strings.Contains(xml, "&lt;text&gt;") {
		t.Errorf("result not XML-escaped: %s", xml)
	}
	if !strings.Contains(xml, "aworker-abcdef1234567890") {
		t.Errorf("missing agent ID: %s", xml)
	}
}
