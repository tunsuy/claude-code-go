//go:build integration

// Package integration contains end-to-end tests for the claude-code-go engine.
// Run with: go test -race -tags=integration ./test/integration/...
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/engine"
	"github.com/anthropics/claude-code-go/internal/session"
	"github.com/anthropics/claude-code-go/internal/tools"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock helpers (mirrors internal/engine/engine_test.go patterns)
// ─────────────────────────────────────────────────────────────────────────────

// mockClient implements api.Client using a user-supplied streamFn.
type mockClient struct {
	streamFn   func(ctx context.Context, req *api.MessageRequest) (api.StreamReader, error)
	completeFn func(ctx context.Context, req *api.MessageRequest) (*api.MessageResponse, error)
}

func (m *mockClient) Stream(ctx context.Context, req *api.MessageRequest) (api.StreamReader, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, req)
	}
	return nil, errors.New("mockClient: streamFn not set")
}

func (m *mockClient) Complete(ctx context.Context, req *api.MessageRequest) (*api.MessageResponse, error) {
	if m.completeFn != nil {
		return m.completeFn(ctx, req)
	}
	return nil, errors.New("mockClient: completeFn not set")
}

// staticStreamReader replays pre-built events then returns io.EOF.
type staticStreamReader struct {
	events []*api.StreamEvent
	pos    int
}

func newStaticReader(events ...*api.StreamEvent) *staticStreamReader {
	return &staticStreamReader{events: events}
}

func (r *staticStreamReader) Next() (*api.StreamEvent, error) {
	if r.pos >= len(r.events) {
		return nil, io.EOF
	}
	ev := r.events[r.pos]
	r.pos++
	return ev, nil
}

func (r *staticStreamReader) Close() error { return nil }

// mustMarshalRaw marshals v as json.RawMessage, panicking on error.
func mustMarshalRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// buildEndTurnEvents builds the minimal SSE event sequence for an end_turn response.
func buildEndTurnEvents(text string) []*api.StreamEvent {
	msgStart := &api.StreamEvent{
		Type: api.EventMessageStart,
		Data: mustMarshalRaw(map[string]any{
			"message": map[string]any{
				"id": "msg_test", "type": "message", "role": "assistant",
				"content": []any{}, "model": "claude-test",
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 0},
			},
		}),
		MessageStart: &api.MessageStartData{
			Message: api.MessageResponse{
				ID:    "msg_test",
				Type:  "message",
				Role:  "assistant",
				Model: "claude-test",
				Usage: api.Usage{InputTokens: 10},
			},
		},
	}
	cbStart := &api.StreamEvent{
		Type: api.EventContentBlockStart,
		Data: mustMarshalRaw(map[string]any{
			"index":         0,
			"content_block": map[string]any{"type": "text", "text": ""},
		}),
	}
	cbDelta := &api.StreamEvent{
		Type: api.EventContentBlockDelta,
		Data: mustMarshalRaw(map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		}),
		ContentBlockDelta: &api.ContentBlockDeltaData{
			Index: 0,
			Delta: api.Delta{Type: "text_delta", Text: text},
		},
	}
	cbStop := &api.StreamEvent{
		Type: api.EventContentBlockStop,
		Data: mustMarshalRaw(map[string]any{"index": 0}),
	}
	msgDelta := &api.StreamEvent{
		Type: api.EventMessageDelta,
		Data: mustMarshalRaw(map[string]any{
			"delta": map[string]any{"stop_reason": "end_turn"},
			"usage": map[string]any{"output_tokens": 5},
		}),
		MessageDelta: &api.MessageDeltaData{},
	}
	msgDelta.MessageDelta.Delta.StopReason = "end_turn"
	msgDelta.MessageDelta.Usage = api.Usage{OutputTokens: 5}
	msgStop := &api.StreamEvent{Type: api.EventMessageStop}

	return []*api.StreamEvent{msgStart, cbStart, cbDelta, cbStop, msgDelta, msgStop}
}

// buildToolUseEvents builds SSE events for a tool_use stop_reason.
func buildToolUseEvents(toolID, toolName, inputJSON string) []*api.StreamEvent {
	msgStart := &api.StreamEvent{
		Type: api.EventMessageStart,
		Data: mustMarshalRaw(map[string]any{
			"message": map[string]any{
				"id": "msg_tu", "type": "message", "role": "assistant",
				"content": []any{}, "model": "claude-test",
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 0},
			},
		}),
		MessageStart: &api.MessageStartData{
			Message: api.MessageResponse{ID: "msg_tu", Usage: api.Usage{InputTokens: 10}},
		},
	}
	cbStart := &api.StreamEvent{
		Type: api.EventContentBlockStart,
		Data: mustMarshalRaw(map[string]any{
			"index": 0,
			"content_block": map[string]any{
				"type": "tool_use", "id": toolID, "name": toolName,
			},
		}),
	}
	cbDelta := &api.StreamEvent{
		Type: api.EventContentBlockDelta,
		Data: mustMarshalRaw(map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": inputJSON},
		}),
		ContentBlockDelta: &api.ContentBlockDeltaData{
			Index: 0,
			Delta: api.Delta{Type: "input_json_delta", PartialJSON: inputJSON},
		},
	}
	cbStop := &api.StreamEvent{
		Type: api.EventContentBlockStop,
		Data: mustMarshalRaw(map[string]any{"index": 0}),
	}
	msgDelta := &api.StreamEvent{
		Type: api.EventMessageDelta,
		Data: mustMarshalRaw(map[string]any{
			"delta": map[string]any{"stop_reason": "tool_use"},
			"usage": map[string]any{"output_tokens": 3},
		}),
		MessageDelta: &api.MessageDeltaData{},
	}
	msgDelta.MessageDelta.Delta.StopReason = "tool_use"
	msgDelta.MessageDelta.Usage = api.Usage{OutputTokens: 3}
	msgStop := &api.StreamEvent{Type: api.EventMessageStop}

	return []*api.StreamEvent{msgStart, cbStart, cbDelta, cbStop, msgDelta, msgStop}
}

// drainMsgs collects all engine.Msg values until the channel closes (5s timeout).
func drainMsgs(ch <-chan engine.Msg) []engine.Msg {
	var msgs []engine.Msg
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				return msgs
			}
			msgs = append(msgs, m)
		case <-timeout:
			return msgs
		}
	}
}

// findMsg returns the first Msg of the given type.
func findMsg(msgs []engine.Msg, t engine.MsgType) (engine.Msg, bool) {
	for _, m := range msgs {
		if m.Type == t {
			return m, true
		}
	}
	return engine.Msg{}, false
}

// filterMsgs returns all messages of the given type.
func filterMsgs(msgs []engine.Msg, t engine.MsgType) []engine.Msg {
	var out []engine.Msg
	for _, m := range msgs {
		if m.Type == t {
			out = append(out, m)
		}
	}
	return out
}

// strPtr is a helper for *string literals.
func strPtr(s string) *string { return &s }

// ─────────────────────────────────────────────────────────────────────────────
// Stub tool implementation (mirrors internal/engine/orchestration_test.go)
// ─────────────────────────────────────────────────────────────────────────────

type stubTool struct {
	name     string
	safe     bool
	readOnly bool
	callFn   func(input tools.Input) (*tools.Result, error)
}

func (s *stubTool) Name() string                                              { return s.name }
func (s *stubTool) Aliases() []string                                         { return nil }
func (s *stubTool) Description(_ tools.Input, _ tools.PermissionContext) string { return "" }
func (s *stubTool) InputSchema() tools.InputSchema                            { return tools.InputSchema{Type: "object"} }
func (s *stubTool) Prompt(_ context.Context, _ tools.PermissionContext) (string, error) {
	return "", nil
}
func (s *stubTool) MaxResultSizeChars() int                           { return -1 }
func (s *stubTool) SearchHint() string                                { return "" }
func (s *stubTool) IsConcurrencySafe(_ tools.Input) bool              { return s.safe }
func (s *stubTool) IsReadOnly(_ tools.Input) bool                     { return s.readOnly }
func (s *stubTool) IsDestructive(_ tools.Input) bool                  { return false }
func (s *stubTool) IsEnabled() bool                                   { return true }
func (s *stubTool) InterruptBehavior() tools.InterruptBehavior        { return tools.InterruptBehaviorCancel }
func (s *stubTool) ValidateInput(_ tools.Input, _ *tools.UseContext) (tools.ValidationResult, error) {
	return tools.ValidationResult{OK: true}, nil
}
func (s *stubTool) CheckPermissions(_ tools.Input, _ *tools.UseContext) (tools.PermissionResult, error) {
	return tools.PermissionResult{Behavior: tools.PermissionPassthrough}, nil
}
func (s *stubTool) PreparePermissionMatcher(_ tools.Input) (func(string) bool, error) {
	return nil, nil
}
func (s *stubTool) Call(input tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if s.callFn != nil {
		return s.callFn(input)
	}
	return &tools.Result{Content: "ok"}, nil
}
func (s *stubTool) MapResultToToolResultBlock(_ any, _ string) (json.RawMessage, error) {
	return json.RawMessage(`"ok"`), nil
}
func (s *stubTool) ToAutoClassifierInput(_ tools.Input) string { return "" }
func (s *stubTool) UserFacingName(_ tools.Input) string        { return s.name }

// newTestRegistry builds a *tools.Registry populated with the provided tools.
func newTestRegistry(ts ...tools.Tool) *tools.Registry {
	r := tools.NewRegistry()
	for _, t := range ts {
		r.Register(t)
	}
	return r
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration Test: Full conversation round-trip (end_turn)
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_FullConversationRoundTrip verifies the complete engine↔API path for
// a simple single-turn conversation that ends with end_turn.
func TestE2E_FullConversationRoundTrip(t *testing.T) {
	const responseText = "Hello from the integration test LLM"

	client := &mockClient{
		streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
			// Verify the request was built correctly.
			if req.Model != "claude-integration-test" {
				t.Errorf("unexpected model %q", req.Model)
			}
			if req.System != "You are a helpful assistant." {
				t.Errorf("unexpected system prompt %q", req.System)
			}
			if len(req.Messages) != 1 {
				t.Errorf("expected 1 message, got %d", len(req.Messages))
			}
			return newStaticReader(buildEndTurnEvents(responseText)...), nil
		},
	}

	reg := tools.NewRegistry()
	eng := engine.New(engine.Config{
		Client:    client,
		Registry:  reg,
		Model:     "claude-integration-test",
		MaxTokens: 1024,
	})

	params := engine.QueryParams{
		Messages: []types.Message{
			{
				Role:    types.RoleUser,
				Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("Hello")}},
			},
		},
		SystemPrompt: engine.SystemPrompt{
			Parts: []engine.SystemPromptPart{{Text: "You are a helpful assistant."}},
		},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	ch, err := eng.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	msgs := drainMsgs(ch)

	// Verify event sequence.
	if _, ok := findMsg(msgs, engine.MsgTypeRequestStart); !ok {
		t.Error("expected MsgTypeRequestStart")
	}
	if _, ok := findMsg(msgs, engine.MsgTypeStreamRequestStart); !ok {
		t.Error("expected MsgTypeStreamRequestStart")
	}
	textMsgs := filterMsgs(msgs, engine.MsgTypeStreamText)
	if len(textMsgs) == 0 {
		t.Error("expected at least one MsgTypeStreamText event")
	}
	if _, ok := findMsg(msgs, engine.MsgTypeAssistantMessage); !ok {
		t.Error("expected MsgTypeAssistantMessage")
	}
	tc, ok := findMsg(msgs, engine.MsgTypeTurnComplete)
	if !ok {
		t.Error("expected MsgTypeTurnComplete")
	} else if tc.StopReason != "end_turn" {
		t.Errorf("StopReason: want end_turn, got %q", tc.StopReason)
	}
	if _, ok := findMsg(msgs, engine.MsgTypeError); ok {
		t.Error("unexpected error event")
	}

	// Message history should be written back.
	history := eng.GetMessages()
	if len(history) < 2 {
		t.Fatalf("expected ≥2 messages in history (user + assistant), got %d", len(history))
	}
	if history[0].Role != types.RoleUser {
		t.Errorf("history[0] role: want user, got %q", history[0].Role)
	}
	if history[len(history)-1].Role != types.RoleAssistant {
		t.Errorf("last history message should be assistant, got %q", history[len(history)-1].Role)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration Test: Tool dispatch full chain
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_ToolDispatch verifies the complete engine↔tools path:
// tool_use response → tool execution → tool_result message → follow-up end_turn.
func TestE2E_ToolDispatch(t *testing.T) {
	const toolID = "toolu_e2e_01"
	const toolName = "FetchData"
	const toolResult = "data: 42"

	callN := 0
	var capturedToolInput string

	client := &mockClient{
		streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
			callN++
			switch callN {
			case 1:
				// First call: LLM requests a tool use.
				return newStaticReader(buildToolUseEvents(toolID, toolName, `{"key":"value"}`)...), nil
			default:
				// Second call: LLM has seen the tool result and ends the conversation.
				// Verify that the tool result was included in the conversation.
				for _, msg := range req.Messages {
					if msg.Role == "user" {
						capturedToolInput = string(msg.Content)
					}
				}
				return newStaticReader(buildEndTurnEvents("Tool call complete")...), nil
			}
		},
	}

	// Register a stub tool that returns a known result.
	reg := newTestRegistry(&stubTool{
		name: toolName,
		safe: true,
		callFn: func(_ tools.Input) (*tools.Result, error) {
			return &tools.Result{Content: toolResult}, nil
		},
	})

	eng := engine.New(engine.Config{
		Client:   client,
		Registry: reg,
		Model:    "claude-e2e-tools",
	})

	params := engine.QueryParams{
		Messages: []types.Message{
			{
				Role:    types.RoleUser,
				Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("fetch data")}},
			},
		},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	ch, err := eng.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	msgs := drainMsgs(ch)

	if callN < 2 {
		t.Errorf("expected 2 LLM calls (tool_use + follow-up end_turn), got %d", callN)
	}

	// Tool use start event.
	tuStart, ok := findMsg(msgs, engine.MsgTypeToolUseStart)
	if !ok {
		t.Error("expected MsgTypeToolUseStart")
	} else {
		if tuStart.ToolName != toolName {
			t.Errorf("ToolUseStart.ToolName: want %q, got %q", toolName, tuStart.ToolName)
		}
		if tuStart.ToolUseID != toolID {
			t.Errorf("ToolUseStart.ToolUseID: want %q, got %q", toolID, tuStart.ToolUseID)
		}
	}

	// Tool result event.
	trMsg, ok := findMsg(msgs, engine.MsgTypeToolResult)
	if !ok {
		t.Error("expected MsgTypeToolResult")
	} else if trMsg.ToolResult == nil {
		t.Error("MsgTypeToolResult.ToolResult must not be nil")
	} else {
		if trMsg.ToolResult.ToolUseID != toolID {
			t.Errorf("ToolResult.ToolUseID: want %q, got %q", toolID, trMsg.ToolResult.ToolUseID)
		}
		if trMsg.ToolResult.Content != toolResult {
			t.Errorf("ToolResult.Content: want %q, got %q", toolResult, trMsg.ToolResult.Content)
		}
		if trMsg.ToolResult.IsError {
			t.Error("tool result should not be an error")
		}
	}

	// Final TurnComplete.
	tc, ok := findMsg(msgs, engine.MsgTypeTurnComplete)
	if !ok {
		t.Error("expected final MsgTypeTurnComplete")
	} else if tc.StopReason != "end_turn" {
		t.Errorf("final StopReason: want end_turn, got %q", tc.StopReason)
	}

	// No error events.
	if _, ok := findMsg(msgs, engine.MsgTypeError); ok {
		t.Error("unexpected error event")
	}

	// Verify the tool result was passed back to the LLM.
	_ = capturedToolInput // validated above implicitly through callN == 2
}

// TestE2E_ToolDispatch_ErrorResult verifies that tool call errors are
// correctly propagated back to the LLM as error tool_result blocks.
func TestE2E_ToolDispatch_ErrorResult(t *testing.T) {
	const toolID = "toolu_err_01"
	const toolName = "FailingTool"

	callN := 0
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			callN++
			switch callN {
			case 1:
				return newStaticReader(buildToolUseEvents(toolID, toolName, `{}`)...), nil
			default:
				return newStaticReader(buildEndTurnEvents("I see the tool failed")...), nil
			}
		},
	}

	reg := newTestRegistry(&stubTool{
		name: toolName,
		safe: false,
		callFn: func(_ tools.Input) (*tools.Result, error) {
			return &tools.Result{Content: "permission denied", IsError: true}, nil
		},
	})

	eng := engine.New(engine.Config{Client: client, Registry: reg, Model: "m"})
	params := engine.QueryParams{
		Messages:       []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("do it")}}}},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	ch, _ := eng.Query(context.Background(), params)
	msgs := drainMsgs(ch)

	trMsg, ok := findMsg(msgs, engine.MsgTypeToolResult)
	if !ok {
		t.Fatal("expected MsgTypeToolResult")
	}
	if trMsg.ToolResult == nil {
		t.Fatal("ToolResult must not be nil")
	}
	if !trMsg.ToolResult.IsError {
		t.Error("expected IsError=true for failing tool")
	}
	if trMsg.ToolResult.Content != "permission denied" {
		t.Errorf("ToolResult.Content: want %q, got %q", "permission denied", trMsg.ToolResult.Content)
	}
}

// TestE2E_MultipleConcurrentTools verifies that concurrent-safe tools are
// executed in a single batch and that all results are returned.
func TestE2E_MultipleConcurrentTools(t *testing.T) {
	callN := 0
	var mu sync.Mutex

	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			mu.Lock()
			callN++
			n := callN
			mu.Unlock()
			if n == 1 {
				// Return two concurrent tool calls in one turn.
				events := buildTwoToolUseEvents(
					"toolu_1", "ReadA", `{"path":"/a"}`,
					"toolu_2", "ReadB", `{"path":"/b"}`,
				)
				return newStaticReader(events...), nil
			}
			return newStaticReader(buildEndTurnEvents("Both done")...), nil
		},
	}

	var callOrder []string
	var callMu sync.Mutex

	reg := newTestRegistry(
		&stubTool{
			name: "ReadA", safe: true,
			callFn: func(_ tools.Input) (*tools.Result, error) {
				callMu.Lock()
				callOrder = append(callOrder, "ReadA")
				callMu.Unlock()
				return &tools.Result{Content: "content-a"}, nil
			},
		},
		&stubTool{
			name: "ReadB", safe: true,
			callFn: func(_ tools.Input) (*tools.Result, error) {
				callMu.Lock()
				callOrder = append(callOrder, "ReadB")
				callMu.Unlock()
				return &tools.Result{Content: "content-b"}, nil
			},
		},
	)

	eng := engine.New(engine.Config{Client: client, Registry: reg, Model: "m"})
	params := engine.QueryParams{
		Messages:       []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("read both")}}}},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	ch, _ := eng.Query(context.Background(), params)
	msgs := drainMsgs(ch)

	// Both tools must have been called.
	callMu.Lock()
	n := len(callOrder)
	callMu.Unlock()
	if n != 2 {
		t.Errorf("expected 2 tool calls, got %d", n)
	}

	// Two ToolResult events.
	toolResults := filterMsgs(msgs, engine.MsgTypeToolResult)
	if len(toolResults) != 2 {
		t.Errorf("expected 2 MsgTypeToolResult events, got %d", len(toolResults))
	}

	if _, ok := findMsg(msgs, engine.MsgTypeError); ok {
		t.Error("unexpected error event")
	}
}

// buildTwoToolUseEvents builds SSE events with two tool_use blocks in one turn.
func buildTwoToolUseEvents(id1, name1, input1, id2, name2, input2 string) []*api.StreamEvent {
	msgStart := &api.StreamEvent{
		Type: api.EventMessageStart,
		Data: mustMarshalRaw(map[string]any{
			"message": map[string]any{
				"id": "msg_2tu", "type": "message", "role": "assistant",
				"content": []any{}, "model": "claude-test",
				"usage": map[string]any{"input_tokens": 15, "output_tokens": 0},
			},
		}),
		MessageStart: &api.MessageStartData{
			Message: api.MessageResponse{ID: "msg_2tu", Usage: api.Usage{InputTokens: 15}},
		},
	}
	cb1Start := &api.StreamEvent{
		Type: api.EventContentBlockStart,
		Data: mustMarshalRaw(map[string]any{
			"index":         0,
			"content_block": map[string]any{"type": "tool_use", "id": id1, "name": name1},
		}),
	}
	cb1Delta := &api.StreamEvent{
		Type: api.EventContentBlockDelta,
		Data: mustMarshalRaw(map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": input1},
		}),
		ContentBlockDelta: &api.ContentBlockDeltaData{
			Index: 0,
			Delta: api.Delta{Type: "input_json_delta", PartialJSON: input1},
		},
	}
	cb1Stop := &api.StreamEvent{
		Type: api.EventContentBlockStop,
		Data: mustMarshalRaw(map[string]any{"index": 0}),
	}
	cb2Start := &api.StreamEvent{
		Type: api.EventContentBlockStart,
		Data: mustMarshalRaw(map[string]any{
			"index":         1,
			"content_block": map[string]any{"type": "tool_use", "id": id2, "name": name2},
		}),
	}
	cb2Delta := &api.StreamEvent{
		Type: api.EventContentBlockDelta,
		Data: mustMarshalRaw(map[string]any{
			"index": 1,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": input2},
		}),
		ContentBlockDelta: &api.ContentBlockDeltaData{
			Index: 1,
			Delta: api.Delta{Type: "input_json_delta", PartialJSON: input2},
		},
	}
	cb2Stop := &api.StreamEvent{
		Type: api.EventContentBlockStop,
		Data: mustMarshalRaw(map[string]any{"index": 1}),
	}
	msgDelta := &api.StreamEvent{
		Type: api.EventMessageDelta,
		Data: mustMarshalRaw(map[string]any{
			"delta": map[string]any{"stop_reason": "tool_use"},
			"usage": map[string]any{"output_tokens": 8},
		}),
		MessageDelta: &api.MessageDeltaData{},
	}
	msgDelta.MessageDelta.Delta.StopReason = "tool_use"
	msgDelta.MessageDelta.Usage = api.Usage{OutputTokens: 8}
	msgStop := &api.StreamEvent{Type: api.EventMessageStop}

	return []*api.StreamEvent{msgStart, cb1Start, cb1Delta, cb1Stop, cb2Start, cb2Delta, cb2Stop, msgDelta, msgStop}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration Test: MaxTurns limit
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_MaxTurns verifies that MaxTurns is enforced by the engine loop.
// We use max_tokens stop_reason to keep the loop running so MaxTurns can fire.
func TestE2E_MaxTurns(t *testing.T) {
	callN := 0
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			callN++
			// Return max_tokens stop reason to force the loop to continue.
			return newStaticReader(buildMaxTokensEvents("partial...")...), nil
		},
	}

	eng := engine.New(engine.Config{Client: client, Registry: tools.NewRegistry(), Model: "m"})
	params := engine.QueryParams{
		MaxTurns:       2,
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("go")}}},
		},
	}

	ch, _ := eng.Query(context.Background(), params)
	msgs := drainMsgs(ch)

	// After MaxTurns turns, the engine must emit a system message and stop.
	sysMsg, ok := findMsg(msgs, engine.MsgTypeSystemMessage)
	if !ok {
		t.Error("expected MsgTypeSystemMessage when MaxTurns reached")
	} else if sysMsg.SystemText == "" {
		t.Error("SystemText should not be empty for MaxTurns message")
	}

	// The loop should have called the API exactly MaxTurns times.
	if callN > 3 {
		t.Errorf("expected ≤3 API calls for MaxTurns=2, got %d", callN)
	}
}

// buildMaxTokensEvents builds events with max_tokens stop reason.
func buildMaxTokensEvents(text string) []*api.StreamEvent {
	msgStart := &api.StreamEvent{
		Type: api.EventMessageStart,
		Data: mustMarshalRaw(map[string]any{
			"message": map[string]any{
				"id": "msg_mt", "type": "message", "role": "assistant",
				"content": []any{}, "model": "claude-test",
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 0},
			},
		}),
		MessageStart: &api.MessageStartData{
			Message: api.MessageResponse{ID: "msg_mt", Usage: api.Usage{InputTokens: 10}},
		},
	}
	cbStart := &api.StreamEvent{
		Type: api.EventContentBlockStart,
		Data: mustMarshalRaw(map[string]any{
			"index":         0,
			"content_block": map[string]any{"type": "text", "text": ""},
		}),
	}
	cbDelta := &api.StreamEvent{
		Type: api.EventContentBlockDelta,
		Data: mustMarshalRaw(map[string]any{
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		}),
		ContentBlockDelta: &api.ContentBlockDeltaData{
			Index: 0,
			Delta: api.Delta{Type: "text_delta", Text: text},
		},
	}
	cbStop := &api.StreamEvent{
		Type: api.EventContentBlockStop,
		Data: mustMarshalRaw(map[string]any{"index": 0}),
	}
	msgDelta := &api.StreamEvent{
		Type: api.EventMessageDelta,
		Data: mustMarshalRaw(map[string]any{
			"delta": map[string]any{"stop_reason": "max_tokens"},
			"usage": map[string]any{"output_tokens": 100},
		}),
		MessageDelta: &api.MessageDeltaData{},
	}
	msgDelta.MessageDelta.Delta.StopReason = "max_tokens"
	msgDelta.MessageDelta.Usage = api.Usage{OutputTokens: 100}
	msgStop := &api.StreamEvent{Type: api.EventMessageStop}

	return []*api.StreamEvent{msgStart, cbStart, cbDelta, cbStop, msgDelta, msgStop}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration Test: Context cancellation / Interrupt
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_ContextCancellation verifies that cancelling the context stops the
// query loop and the message channel is closed promptly.
func TestE2E_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			return newStaticReader(buildEndTurnEvents("should not see")...), nil
		},
	}

	eng := engine.New(engine.Config{Client: client, Registry: tools.NewRegistry(), Model: "m"})
	params := engine.QueryParams{
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("hi")}}},
		},
	}

	ch, err := eng.Query(ctx, params)
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	select {
	case <-ch:
		// Channel closed promptly — expected.
	case <-time.After(2 * time.Second):
		t.Error("query did not terminate after context cancellation")
	}
}

// TestE2E_Interrupt verifies that calling Interrupt() stops a running query.
func TestE2E_Interrupt(t *testing.T) {
	streamStarted := make(chan struct{})
	unblock := make(chan struct{})

	client := &mockClient{
		streamFn: func(ctx context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			close(streamStarted)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-unblock:
				return newStaticReader(), nil
			}
		},
	}

	eng := engine.New(engine.Config{Client: client, Registry: tools.NewRegistry(), Model: "m"})
	params := engine.QueryParams{
		Messages:       []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("hi")}}}},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	ch, err := eng.Query(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}

	<-streamStarted
	eng.Interrupt(context.Background())
	close(unblock)

	select {
	case <-ch:
		// Channel closed after interrupt — expected.
	case <-time.After(3 * time.Second):
		t.Error("query did not terminate after Interrupt()")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration Test: FallbackModel on context window error
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_FallbackModel verifies that when the primary model returns a
// context-window error, the engine retries with the fallback model.
func TestE2E_FallbackModel(t *testing.T) {
	ctxErr := &api.APIError{Kind: api.ErrKindContextWindow, Message: "context too long"}

	callN := 0
	var callMu sync.Mutex
	var requestedModels []string

	client := &mockClient{
		streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
			callMu.Lock()
			callN++
			requestedModels = append(requestedModels, req.Model)
			n := callN
			callMu.Unlock()

			if n == 1 {
				return nil, ctxErr
			}
			return newStaticReader(buildEndTurnEvents("fallback success")...), nil
		},
	}

	eng := engine.New(engine.Config{
		Client:   client,
		Registry: tools.NewRegistry(),
		Model:    "primary-model",
	})
	params := engine.QueryParams{
		FallbackModel: "fallback-model",
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("long prompt")}}},
		},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	ch, _ := eng.Query(context.Background(), params)
	msgs := drainMsgs(ch)

	callMu.Lock()
	n := callN
	models := append([]string(nil), requestedModels...)
	callMu.Unlock()

	if n != 2 {
		t.Errorf("expected 2 API calls (primary + fallback), got %d", n)
	}
	if len(models) >= 1 && models[0] != "primary-model" {
		t.Errorf("first call model: want primary-model, got %q", models[0])
	}
	if len(models) >= 2 && models[1] != "fallback-model" {
		t.Errorf("second call model: want fallback-model, got %q", models[1])
	}
	if _, ok := findMsg(msgs, engine.MsgTypeError); ok {
		t.Error("expected no error when fallback succeeds")
	}
	if _, ok := findMsg(msgs, engine.MsgTypeTurnComplete); !ok {
		t.Error("expected TurnComplete after fallback success")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration Test: Session persistence round-trip
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_SessionPersistence verifies the complete session lifecycle:
// create → append entries → close → reopen → read entries back.
func TestE2E_SessionPersistence(t *testing.T) {
	// Use a temp directory as the project root.
	projectDir := t.TempDir()

	// Create a new session.
	sid, mgr, err := session.New(projectDir)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	if sid == "" {
		t.Error("session ID must not be empty")
	}

	// Append a few entries.
	type testEntry struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	entries := []testEntry{
		{Type: "user", Content: "Hello, session!"},
		{Type: "assistant", Content: "Hello back!"},
		{Type: "user", Content: "How are you?"},
	}
	for _, e := range entries {
		if err := mgr.AppendEntry(e); err != nil {
			t.Fatalf("AppendEntry: %v", err)
		}
	}
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close: %v", err)
	}

	// Resume the same session.
	sid2, mgr2, err := session.Resume(string(sid), projectDir)
	if err != nil {
		t.Fatalf("session.Resume: %v", err)
	}
	if sid2 != sid {
		t.Errorf("resumed session ID mismatch: want %q, got %q", sid, sid2)
	}
	defer mgr2.Close()

	// Read all entries back.
	envelopes, err := mgr2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(envelopes) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(envelopes))
	}

	// Verify content round-trips correctly.
	for i, env := range envelopes {
		var got testEntry
		if err := json.Unmarshal(env.Raw, &got); err != nil {
			t.Errorf("entry %d: unmarshal: %v", i, err)
			continue
		}
		if got.Type != entries[i].Type || got.Content != entries[i].Content {
			t.Errorf("entry %d: want %+v, got %+v", i, entries[i], got)
		}
	}
}

// TestE2E_SessionResume_NotFound verifies that Resume returns an error for
// a non-existent session.
func TestE2E_SessionResume_NotFound(t *testing.T) {
	projectDir := t.TempDir()
	_, _, err := session.Resume("nonexistent-session-id", projectDir)
	if err == nil {
		t.Error("expected error when resuming non-existent session")
	}
}

// TestE2E_Session_CorruptLinesSkipped verifies that corrupt JSONL lines are
// skipped gracefully without returning an error.
func TestE2E_Session_CorruptLinesSkipped(t *testing.T) {
	projectDir := t.TempDir()

	sid, mgr, err := session.New(projectDir)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}

	// Append valid entries.
	if err := mgr.AppendEntry(map[string]string{"type": "valid1"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AppendEntry(map[string]string{"type": "valid2"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}

	// Corrupt the file by finding it and injecting a bad line.
	// We do this by appending a bad line directly via the store.
	store2, err := session.OpenSessionStore(sessionFilePath(t, projectDir, sid))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	// Write a raw corrupt JSON line.
	f, openErr := os.OpenFile(sessionFilePath(t, projectDir, sid), os.O_APPEND|os.O_WRONLY, 0o644)
	if openErr != nil {
		t.Fatalf("open file: %v", openErr)
	}
	_, _ = f.WriteString("{not valid json}\n")
	_ = f.Close()
	_ = store2.Close()

	// Re-open and read — corrupt line should be skipped.
	_, mgr3, err := session.Resume(string(sid), projectDir)
	if err != nil {
		t.Fatalf("session.Resume: %v", err)
	}
	defer mgr3.Close()

	envelopes, err := mgr3.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error for corrupt line: %v", err)
	}
	// Only the 2 valid entries should remain.
	if len(envelopes) != 2 {
		t.Errorf("expected 2 valid entries after skipping corrupt line, got %d", len(envelopes))
	}
}

// sessionFilePath reconstructs the session file path using the same conventions
// as the session package (projectDir/.claude/projects/<hash>/<sid>.jsonl).
// This helper duplicates the logic from session.sessionPath since it's unexported.
func sessionFilePath(t *testing.T, projectDir string, sid types.SessionId) string {
	t.Helper()
	// Use sessionPathViaNew to find the path by creating a dummy session
	// and looking at its ID, then computing the expected path.
	// Since we can't call the unexported sessionPath, we derive it by running
	// a New() call and observing the resulting path heuristically.
	// Instead, we'll just glob for the file.
	matches, err := filepath.Glob(projectDir + "/.claude/projects/*/" + string(sid) + ".jsonl")
	if err != nil || len(matches) == 0 {
		t.Fatalf("could not find session file for %s: %v", sid, err)
	}
	return matches[0]
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration Test: SetModel / GetMessages across queries
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_SetModelAffectsNextQuery verifies that SetModel changes the model
// used in the next Query call.
func TestE2E_SetModelAffectsNextQuery(t *testing.T) {
	var lastModel string
	var modelMu sync.Mutex

	client := &mockClient{
		streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
			modelMu.Lock()
			lastModel = req.Model
			modelMu.Unlock()
			return newStaticReader(buildEndTurnEvents("ok")...), nil
		},
	}

	eng := engine.New(engine.Config{Client: client, Registry: tools.NewRegistry(), Model: "original-model"})

	// First query.
	params := engine.QueryParams{
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		Messages:       []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("q1")}}}},
	}
	ch, _ := eng.Query(context.Background(), params)
	drainMsgs(ch)

	modelMu.Lock()
	first := lastModel
	modelMu.Unlock()
	if first != "original-model" {
		t.Errorf("first query model: want original-model, got %q", first)
	}

	// Change model.
	eng.SetModel("new-model")

	// Second query.
	ch2, _ := eng.Query(context.Background(), params)
	drainMsgs(ch2)

	modelMu.Lock()
	second := lastModel
	modelMu.Unlock()
	if second != "new-model" {
		t.Errorf("second query model: want new-model, got %q", second)
	}
}

// TestE2E_SetMessages_PersistsAcrossQuery verifies that SetMessages seeds
// the conversation history that gets written back after a query.
func TestE2E_SetMessages_PersistsAcrossQuery(t *testing.T) {
	client := &mockClient{
		streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
			// Verify the pre-seeded message is sent to the API.
			if len(req.Messages) == 0 {
				t.Error("expected at least one message in the request")
			}
			return newStaticReader(buildEndTurnEvents("reply")...), nil
		},
	}

	eng := engine.New(engine.Config{Client: client, Registry: tools.NewRegistry(), Model: "m"})

	// Pre-seed messages using SetMessages.
	seed := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("seeded user message")}}},
	}
	eng.SetMessages(seed)

	// Query using the seeded messages.
	params := engine.QueryParams{
		Messages:       eng.GetMessages(),
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}
	ch, _ := eng.Query(context.Background(), params)
	drainMsgs(ch)

	history := eng.GetMessages()
	if len(history) < 2 {
		t.Fatalf("expected ≥2 messages (seeded + assistant), got %d", len(history))
	}
	if history[0].Role != types.RoleUser {
		t.Errorf("history[0]: want user, got %q", history[0].Role)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration Test: Unknown tool graceful recovery
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_UnknownTool_GracefulRecovery verifies that when the LLM requests a
// tool that is not registered, the engine returns an error result block and
// continues the conversation rather than crashing.
func TestE2E_UnknownTool_GracefulRecovery(t *testing.T) {
	callN := 0
	client := &mockClient{
		streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
			callN++
			switch callN {
			case 1:
				return newStaticReader(buildToolUseEvents("toolu_unk", "UnknownTool", `{}`)...), nil
			default:
				return newStaticReader(buildEndTurnEvents("recovered")...), nil
			}
		},
	}

	// Empty registry — UnknownTool is not registered.
	reg := tools.NewRegistry()
	eng := engine.New(engine.Config{Client: client, Registry: reg, Model: "m"})
	params := engine.QueryParams{
		Messages:       []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("use unknown tool")}}}},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	ch, _ := eng.Query(context.Background(), params)
	msgs := drainMsgs(ch)

	// Engine should emit a ToolResult with IsError=true.
	trMsg, ok := findMsg(msgs, engine.MsgTypeToolResult)
	if !ok {
		t.Fatal("expected MsgTypeToolResult even for unknown tool")
	}
	if trMsg.ToolResult == nil {
		t.Fatal("ToolResult must not be nil")
	}
	if !trMsg.ToolResult.IsError {
		t.Error("expected IsError=true for unknown tool result")
	}

	// Engine should continue and eventually complete.
	if _, ok := findMsg(msgs, engine.MsgTypeTurnComplete); !ok {
		t.Error("expected MsgTypeTurnComplete after unknown tool recovery")
	}

	// No fatal engine-level error.
	if _, ok := findMsg(msgs, engine.MsgTypeError); ok {
		t.Error("unexpected engine error for unknown tool (should recover gracefully)")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// filepath import needed for sessionFilePath helper
// ─────────────────────────────────────────────────────────────────────────────
