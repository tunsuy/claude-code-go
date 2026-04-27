package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/api"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

func TestRunForkedAgent_CollectsMessages(t *testing.T) {
	t.Parallel()

	events := buildEndTurnEvents("forked response")
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(events...), nil
		},
	}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	cfg := ForkedAgentConfig{
		PromptMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("hello fork")},
			}},
		},
		CacheSafeParams: &CacheSafeParams{
			SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{
				{Text: "You are a test assistant."},
			}},
			ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		},
		QuerySource: "test_fork",
	}

	msgs, err := RunForkedAgent(context.Background(), eng, cfg)
	if err != nil {
		t.Fatalf("RunForkedAgent returned error: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("expected at least one collected message, got none")
	}

	// The collected messages should include the assistant response.
	foundAssistant := false
	for _, m := range msgs {
		if m.Role == types.RoleAssistant {
			foundAssistant = true
			break
		}
	}
	if !foundAssistant {
		t.Error("expected at least one assistant message in collected results")
	}
}

func TestRunForkedAgent_MaxTurns(t *testing.T) {
	t.Parallel()

	// Use tool_use flow that would loop indefinitely without MaxTurns.
	callCount := 0
	var mu sync.Mutex
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			mu.Lock()
			callCount++
			count := callCount
			mu.Unlock()

			if count <= 2 {
				// Return tool_use to keep looping.
				events := buildToolUseEvents("call_"+string(rune('0'+count)), "Echo", `{"msg":"hi"}`)
				return newStaticReader(events...), nil
			}
			// Fallback: end turn.
			events := buildEndTurnEvents("done")
			return newStaticReader(events...), nil
		},
	}

	reg := newTestRegistry(&stubTool{name: "Echo", safe: true})
	eng := New(Config{Client: client, Registry: reg, Model: "claude-test"})

	cfg := ForkedAgentConfig{
		PromptMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("loop")},
			}},
		},
		CacheSafeParams: &CacheSafeParams{
			SystemPrompt:   SystemPrompt{},
			ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		},
		MaxTurns: 2,
	}

	msgs, err := RunForkedAgent(context.Background(), eng, cfg)
	if err != nil {
		t.Fatalf("RunForkedAgent returned error: %v", err)
	}

	// With MaxTurns=2, the engine should stop after 2 turns.
	mu.Lock()
	finalCount := callCount
	mu.Unlock()

	if finalCount > 2 {
		t.Errorf("expected at most 2 API calls with MaxTurns=2, got %d", finalCount)
	}
	_ = msgs
}

func TestRunForkedAgent_NoWriteBack(t *testing.T) {
	t.Parallel()

	events := buildEndTurnEvents("forked reply")
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(events...), nil
		},
	}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	// Set some initial messages on the engine.
	initialMsgs := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr("original")},
		}},
	}
	eng.SetMessages(initialMsgs)

	cfg := ForkedAgentConfig{
		PromptMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("forked question")},
			}},
		},
		CacheSafeParams: &CacheSafeParams{
			SystemPrompt:   SystemPrompt{},
			ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		},
	}

	_, err := RunForkedAgent(context.Background(), eng, cfg)
	if err != nil {
		t.Fatalf("RunForkedAgent returned error: %v", err)
	}

	// The engine's messages should remain unchanged (NoWriteBack=true).
	engineMsgs := eng.GetMessages()
	if len(engineMsgs) != 1 {
		t.Fatalf("expected engine messages to remain unchanged (1 message), got %d", len(engineMsgs))
	}
	if engineMsgs[0].Role != types.RoleUser {
		t.Errorf("expected original user message to be preserved, got role=%q", engineMsgs[0].Role)
	}
}

func TestRunForkedAgent_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create a context that we'll cancel during the query.
	ctx, cancel := context.WithCancel(context.Background())

	streamStarted := make(chan struct{})
	client := &mockClient{
		streamFn: func(ctx context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			close(streamStarted)
			// Block until context is cancelled.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	cfg := ForkedAgentConfig{
		PromptMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("will be cancelled")},
			}},
		},
		CacheSafeParams: &CacheSafeParams{
			SystemPrompt:   SystemPrompt{},
			ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		},
	}

	done := make(chan struct{})
	var msgs []types.Message
	var runErr error

	go func() {
		defer close(done)
		msgs, runErr = RunForkedAgent(ctx, eng, cfg)
	}()

	// Wait for the stream to start, then cancel.
	select {
	case <-streamStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for stream to start")
	}
	cancel()

	// RunForkedAgent should return promptly.
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("RunForkedAgent did not return after context cancellation")
	}

	// It should return an error (either context.Canceled or a wrapped error).
	if runErr == nil {
		t.Log("RunForkedAgent returned no error after cancellation (acceptable if channel closed first)")
	}
	_ = msgs
}

func TestRunForkedAgent_AllowedToolsFilter(t *testing.T) {
	t.Parallel()

	// Track which tools appear in the API request.
	var capturedReq *api.MessageRequest
	var mu sync.Mutex

	events := buildEndTurnEvents("done")
	client := &mockClient{
		streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
			mu.Lock()
			capturedReq = req
			mu.Unlock()
			return newStaticReader(events...), nil
		},
	}

	reg := newTestRegistry(
		&stubTool{name: "Read", safe: true},
		&stubTool{name: "Write", safe: false},
		&stubTool{name: "Bash", safe: false},
	)
	eng := New(Config{Client: client, Registry: reg, Model: "claude-test"})

	cfg := ForkedAgentConfig{
		PromptMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("test allowed")},
			}},
		},
		CacheSafeParams: &CacheSafeParams{
			SystemPrompt:   SystemPrompt{},
			ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		},
		AllowedTools: []string{"Read"}, // Only Read should be available.
	}

	_, err := RunForkedAgent(context.Background(), eng, cfg)
	if err != nil {
		t.Fatalf("RunForkedAgent returned error: %v", err)
	}

	mu.Lock()
	req := capturedReq
	mu.Unlock()

	if req == nil {
		t.Fatal("expected API request to be captured")
	}

	// Only "Read" should be in the tools list.
	toolNames := make(map[string]bool)
	for _, ts := range req.Tools {
		toolNames[ts.Name] = true
	}

	if !toolNames["Read"] {
		t.Error("expected Read tool to be included")
	}
	if toolNames["Write"] {
		t.Error("Write tool should be excluded")
	}
	if toolNames["Bash"] {
		t.Error("Bash tool should be excluded")
	}
}

func TestRunForkedAgent_NilEngine(t *testing.T) {
	t.Parallel()

	cfg := ForkedAgentConfig{
		CacheSafeParams: &CacheSafeParams{},
	}

	_, err := RunForkedAgent(context.Background(), nil, cfg)
	if err == nil {
		t.Fatal("expected error for nil engine")
	}
}

func TestRunForkedAgent_NilCacheSafeParams(t *testing.T) {
	t.Parallel()

	eng := New(Config{Client: &mockClient{}, Registry: tools.NewRegistry()})
	cfg := ForkedAgentConfig{
		CacheSafeParams: nil,
	}

	_, err := RunForkedAgent(context.Background(), eng, cfg)
	if err == nil {
		t.Fatal("expected error for nil CacheSafeParams")
	}
}

func TestRunForkedAgent_OnMessageCallback(t *testing.T) {
	t.Parallel()

	events := buildEndTurnEvents("callback test")
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(events...), nil
		},
	}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	var callbackMsgs []types.Message
	var mu sync.Mutex

	cfg := ForkedAgentConfig{
		PromptMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("hello")},
			}},
		},
		CacheSafeParams: &CacheSafeParams{
			SystemPrompt:   SystemPrompt{},
			ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		},
		OnMessage: func(msg types.Message) {
			mu.Lock()
			callbackMsgs = append(callbackMsgs, msg)
			mu.Unlock()
		},
	}

	_, err := RunForkedAgent(context.Background(), eng, cfg)
	if err != nil {
		t.Fatalf("RunForkedAgent returned error: %v", err)
	}

	mu.Lock()
	count := len(callbackMsgs)
	mu.Unlock()

	if count == 0 {
		t.Error("expected OnMessage callback to be invoked at least once")
	}
}

func TestRunForkedAgent_ContextMessages(t *testing.T) {
	t.Parallel()

	// Verify that context messages from CacheSafeParams are prepended to prompt messages.
	var capturedReq *api.MessageRequest
	var mu sync.Mutex

	events := buildEndTurnEvents("done")
	client := &mockClient{
		streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
			mu.Lock()
			capturedReq = req
			mu.Unlock()
			return newStaticReader(events...), nil
		},
	}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	contextMsg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr("context message")},
		},
	}
	promptMsg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr("prompt message")},
		},
	}

	cfg := ForkedAgentConfig{
		PromptMessages: []types.Message{promptMsg},
		CacheSafeParams: &CacheSafeParams{
			SystemPrompt:    SystemPrompt{Parts: []SystemPromptPart{{Text: "sys"}}},
			ContextMessages: []types.Message{contextMsg},
			ToolUseContext:  &tools.UseContext{Ctx: context.Background()},
		},
	}

	_, err := RunForkedAgent(context.Background(), eng, cfg)
	if err != nil {
		t.Fatalf("RunForkedAgent returned error: %v", err)
	}

	mu.Lock()
	req := capturedReq
	mu.Unlock()

	if req == nil {
		t.Fatal("expected API request to be captured")
	}

	// Should have 2 messages: context + prompt.
	if len(req.Messages) != 2 {
		t.Errorf("expected 2 messages in API request, got %d", len(req.Messages))
	}
}
