package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/agent"
)

// mockCoordinator is a test double for tools.AgentCoordinator.
type mockCoordinator struct {
	spawnFn     func(ctx context.Context, req tools.AgentSpawnRequest) (string, error)
	sendMsgFn   func(ctx context.Context, agentID string, message string) error
	stopFn      func(ctx context.Context, agentID string) error
	statusFn    func(ctx context.Context, agentID string) (string, error)
	resultFn    func(ctx context.Context, agentID string) (string, string, error)
	listFn      func() map[string]string
	waitFn      func(ctx context.Context, agentID string) (string, error)
}

func (m *mockCoordinator) SpawnAgent(ctx context.Context, req tools.AgentSpawnRequest) (string, error) {
	if m.spawnFn != nil {
		return m.spawnFn(ctx, req)
	}
	return "mock-agent-1", nil
}

func (m *mockCoordinator) SendMessage(ctx context.Context, agentID string, message string) error {
	if m.sendMsgFn != nil {
		return m.sendMsgFn(ctx, agentID, message)
	}
	return nil
}

func (m *mockCoordinator) StopAgent(ctx context.Context, agentID string) error {
	if m.stopFn != nil {
		return m.stopFn(ctx, agentID)
	}
	return nil
}

func (m *mockCoordinator) GetAgentStatus(ctx context.Context, agentID string) (string, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, agentID)
	}
	return "running", nil
}

func (m *mockCoordinator) GetAgentResult(ctx context.Context, agentID string) (string, string, error) {
	if m.resultFn != nil {
		return m.resultFn(ctx, agentID)
	}
	return "result", "completed", nil
}

func (m *mockCoordinator) ListAgents() map[string]string {
	if m.listFn != nil {
		return m.listFn()
	}
	return map[string]string{}
}

func (m *mockCoordinator) WaitForAgent(ctx context.Context, agentID string) (string, error) {
	if m.waitFn != nil {
		return m.waitFn(ctx, agentID)
	}
	return "done", nil
}

// ── AgentTool ─────────────────────────────────────────────────────────────────

func TestAgentTool_Name(t *testing.T) {
	if agent.AgentTool.Name() != "Agent" {
		t.Errorf("expected Agent, got %q", agent.AgentTool.Name())
	}
}

func TestAgentTool_IsConcurrencySafe_False(t *testing.T) {
	// P1-1: must be false (state-mutating tool)
	if agent.AgentTool.IsConcurrencySafe(nil) {
		t.Error("AgentTool.IsConcurrencySafe must return false")
	}
}

func TestAgentTool_IsReadOnly_False(t *testing.T) {
	if agent.AgentTool.IsReadOnly(nil) {
		t.Error("AgentTool.IsReadOnly must return false")
	}
}

func TestAgentTool_InputSchema_HasAllowedTools(t *testing.T) {
	// P1-2: schema must contain allowed_tools
	schema := agent.AgentTool.InputSchema()
	if _, ok := schema.Properties["allowed_tools"]; !ok {
		t.Error("AgentTool schema missing 'allowed_tools'")
	}
}

func TestAgentTool_InputSchema_HasMaxTurns(t *testing.T) {
	// P1-2: schema must contain max_turns
	schema := agent.AgentTool.InputSchema()
	if _, ok := schema.Properties["max_turns"]; !ok {
		t.Error("AgentTool schema missing 'max_turns'")
	}
}

func TestAgentTool_InputSchema_RequiredIsPrompt(t *testing.T) {
	schema := agent.AgentTool.InputSchema()
	if len(schema.Required) != 1 || schema.Required[0] != "prompt" {
		t.Errorf("expected Required=[prompt], got %v", schema.Required)
	}
}

func TestAgentTool_UserFacingName_WithPrompt(t *testing.T) {
	in, _ := json.Marshal(agent.AgentInput{Prompt: "do something"})
	name := agent.AgentTool.UserFacingName(in)
	if !strings.Contains(name, "do something") {
		t.Errorf("unexpected UserFacingName: %q", name)
	}
}

func TestAgentTool_UserFacingName_LongTruncated(t *testing.T) {
	prompt := strings.Repeat("x", 80)
	in, _ := json.Marshal(agent.AgentInput{Prompt: prompt})
	name := agent.AgentTool.UserFacingName(in)
	// Should be truncated at 60 chars + "…" (3 UTF-8 bytes, 1 rune)
	// Use rune count to handle multi-byte ellipsis correctly.
	runes := []rune(name)
	// "Agent(" = 6 runes, 60 content runes, "…" = 1 rune, ")" = 1 rune → max 68 runes
	if len(runes) > 68 {
		t.Errorf("UserFacingName not truncated properly: %q (rune len %d)", name, len(runes))
	}
}

func TestAgentTool_UserFacingName_NoInput(t *testing.T) {
	name := agent.AgentTool.UserFacingName(nil)
	if name != "Agent" {
		t.Errorf("expected Agent, got %q", name)
	}
}

func TestAgentTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(agent.AgentInput{Prompt: "hello"})
	result, err := agent.AgentTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

func TestAgentTool_AgentInput_AllowedToolsField(t *testing.T) {
	// P1-2: JSON round-trip for AllowedTools
	in := agent.AgentInput{
		Prompt:       "do it",
		AllowedTools: []string{"Bash", "Read"},
	}
	data, _ := json.Marshal(in)
	var out agent.AgentInput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.AllowedTools) != 2 || out.AllowedTools[0] != "Bash" {
		t.Errorf("AllowedTools round-trip failed: %v", out.AllowedTools)
	}
}

func TestAgentTool_AgentInput_MaxTurnsField(t *testing.T) {
	// P1-2: JSON round-trip for MaxTurns
	maxTurns := 5
	in := agent.AgentInput{Prompt: "do it", MaxTurns: &maxTurns}
	data, _ := json.Marshal(in)
	var out agent.AgentInput
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.MaxTurns == nil || *out.MaxTurns != 5 {
		t.Errorf("MaxTurns round-trip failed: %v", out.MaxTurns)
	}
}

func TestAgentTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = agent.AgentTool
}

// ── SendMessageTool ───────────────────────────────────────────────────────────

func TestSendMessageTool_Name(t *testing.T) {
	if agent.SendMessageTool.Name() != "SendMessage" {
		t.Errorf("expected SendMessage, got %q", agent.SendMessageTool.Name())
	}
}

func TestSendMessageTool_IsConcurrencySafe_False(t *testing.T) {
	// P1-1: must be false
	if agent.SendMessageTool.IsConcurrencySafe(nil) {
		t.Error("SendMessageTool.IsConcurrencySafe must return false")
	}
}

func TestSendMessageTool_IsReadOnly_False(t *testing.T) {
	if agent.SendMessageTool.IsReadOnly(nil) {
		t.Error("SendMessageTool.IsReadOnly must return false")
	}
}

func TestSendMessageTool_InputSchema_ContentField(t *testing.T) {
	// P1-3: schema must use "content" not "message"
	schema := agent.SendMessageTool.InputSchema()
	if _, ok := schema.Properties["content"]; !ok {
		t.Error("SendMessageTool schema missing 'content' field")
	}
	if _, ok := schema.Properties["message"]; ok {
		t.Error("SendMessageTool schema must not have 'message' field (should be 'content')")
	}
}

func TestSendMessageTool_InputSchema_Required(t *testing.T) {
	schema := agent.SendMessageTool.InputSchema()
	reqMap := make(map[string]bool)
	for _, r := range schema.Required {
		reqMap[r] = true
	}
	if !reqMap["agent_id"] {
		t.Error("SendMessageTool schema must require 'agent_id'")
	}
	if !reqMap["content"] {
		t.Error("SendMessageTool schema must require 'content'")
	}
}

func TestSendMessageInput_ContentJsonTag(t *testing.T) {
	// P1-3: JSON tag must be "content"
	in := agent.SendMessageInput{AgentID: "a1", Content: "hello"}
	data, _ := json.Marshal(in)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["content"]; !ok {
		t.Error("SendMessageInput.Content must serialize as 'content'")
	}
	if _, ok := m["message"]; ok {
		t.Error("SendMessageInput must not serialize as 'message'")
	}
}

func TestSendMessageTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(agent.SendMessageInput{AgentID: "bot1", Content: "hi"})
	name := agent.SendMessageTool.UserFacingName(in)
	if !strings.Contains(name, "bot1") {
		t.Errorf("UserFacingName should contain agent_id: %q", name)
	}
}

func TestSendMessageTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(agent.SendMessageInput{AgentID: "a1", Content: "hello"})
	result, err := agent.SendMessageTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

func TestSendMessageTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = agent.SendMessageTool
}

// ── AgentTool Call() with mock coordinator ───────────────────────────────────

func TestAgentTool_Call_Success(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		spawnFn: func(_ context.Context, req tools.AgentSpawnRequest) (string, error) {
			if req.Prompt != "analyze code" {
				t.Errorf("unexpected prompt: %q", req.Prompt)
			}
			return "agent-42", nil
		},
		waitFn: func(_ context.Context, agentID string) (string, error) {
			if agentID != "agent-42" {
				t.Errorf("unexpected agentID: %q", agentID)
			}
			return "analysis complete", nil
		},
	}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	input, _ := json.Marshal(agent.AgentInput{Prompt: "analyze code"})
	result, err := agent.AgentTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %s", result.Content)
	}

	var out agent.AgentOutput
	if err := json.Unmarshal([]byte(result.Content.(string)), &out); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if out.Response != "analysis complete" {
		t.Errorf("expected 'analysis complete', got %q", out.Response)
	}
}

func TestAgentTool_Call_WithMaxTurns(t *testing.T) {
	t.Parallel()
	var capturedReq tools.AgentSpawnRequest
	mock := &mockCoordinator{
		spawnFn: func(_ context.Context, req tools.AgentSpawnRequest) (string, error) {
			capturedReq = req
			return "agent-mt", nil
		},
		waitFn: func(_ context.Context, _ string) (string, error) {
			return "ok", nil
		},
	}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	maxTurns := 10
	input, _ := json.Marshal(agent.AgentInput{
		Prompt:       "task with limits",
		AllowedTools: []string{"Read", "Bash"},
		MaxTurns:     &maxTurns,
	})
	result, err := agent.AgentTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %s", result.Content)
	}
	if capturedReq.MaxTurns != 10 {
		t.Errorf("expected MaxTurns=10, got %d", capturedReq.MaxTurns)
	}
	if len(capturedReq.AllowedTools) != 2 {
		t.Errorf("expected 2 AllowedTools, got %d", len(capturedReq.AllowedTools))
	}
}

func TestAgentTool_Call_EmptyPrompt(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	input, _ := json.Marshal(agent.AgentInput{Prompt: ""})
	result, err := agent.AgentTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty prompt")
	}
	if !strings.Contains(result.Content.(string), "prompt is required") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

func TestAgentTool_Call_InvalidJSON(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	result, err := agent.AgentTool.Call([]byte(`{bad json`), ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON")
	}
	if !strings.Contains(result.Content.(string), "invalid input") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

func TestAgentTool_Call_SpawnError(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		spawnFn: func(_ context.Context, _ tools.AgentSpawnRequest) (string, error) {
			return "", errors.New("spawn limit reached")
		},
	}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	input, _ := json.Marshal(agent.AgentInput{Prompt: "do something"})
	result, err := agent.AgentTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for spawn error")
	}
	if !strings.Contains(result.Content.(string), "failed to spawn agent") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

func TestAgentTool_Call_WaitError(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		spawnFn: func(_ context.Context, _ tools.AgentSpawnRequest) (string, error) {
			return "agent-err", nil
		},
		waitFn: func(_ context.Context, _ string) (string, error) {
			return "", errors.New("agent panicked")
		},
	}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	input, _ := json.Marshal(agent.AgentInput{Prompt: "do something"})
	result, err := agent.AgentTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for wait error")
	}
	if !strings.Contains(result.Content.(string), "failed") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

func TestAgentTool_Call_WithProgress(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		spawnFn: func(_ context.Context, _ tools.AgentSpawnRequest) (string, error) {
			return "agent-prog", nil
		},
		waitFn: func(_ context.Context, _ string) (string, error) {
			return "done", nil
		},
	}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	var progressData any
	onProgress := func(data any) {
		progressData = data
	}

	input, _ := json.Marshal(agent.AgentInput{Prompt: "progress task"})
	result, err := agent.AgentTool.Call(input, ctx, onProgress)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %s", result.Content)
	}

	// Verify progress was reported.
	pm, ok := progressData.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %T", progressData)
	}
	if pm["agent_id"] != "agent-prog" {
		t.Errorf("expected agent_id 'agent-prog', got %q", pm["agent_id"])
	}
	if pm["status"] != "spawned" {
		t.Errorf("expected status 'spawned', got %q", pm["status"])
	}
}

// ── SendMessageTool Call() with mock coordinator ─────────────────────────────

func TestSendMessageTool_Call_Success(t *testing.T) {
	t.Parallel()
	var capturedID, capturedMsg string
	mock := &mockCoordinator{
		sendMsgFn: func(_ context.Context, agentID string, message string) error {
			capturedID = agentID
			capturedMsg = message
			return nil
		},
	}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	input, _ := json.Marshal(agent.SendMessageInput{AgentID: "bot1", Content: "hello agent"})
	result, err := agent.SendMessageTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %s", result.Content)
	}
	if capturedID != "bot1" {
		t.Errorf("expected agentID 'bot1', got %q", capturedID)
	}
	if capturedMsg != "hello agent" {
		t.Errorf("expected message 'hello agent', got %q", capturedMsg)
	}

	// Verify output contains delivery confirmation.
	var out agent.SendMessageOutput
	if err := json.Unmarshal([]byte(result.Content.(string)), &out); err != nil {
		t.Fatalf("failed to unmarshal output: %v", err)
	}
	if !strings.Contains(out.Response, "bot1") {
		t.Errorf("response should mention agent ID: %q", out.Response)
	}
}

func TestSendMessageTool_Call_EmptyAgentID(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	input, _ := json.Marshal(agent.SendMessageInput{AgentID: "", Content: "hello"})
	result, err := agent.SendMessageTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty agent_id")
	}
	if !strings.Contains(result.Content.(string), "agent_id is required") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

func TestSendMessageTool_Call_EmptyContent(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	input, _ := json.Marshal(agent.SendMessageInput{AgentID: "bot1", Content: ""})
	result, err := agent.SendMessageTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty content")
	}
	if !strings.Contains(result.Content.(string), "content is required") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

func TestSendMessageTool_Call_InvalidJSON(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	result, err := agent.SendMessageTool.Call([]byte(`{bad`), ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON")
	}
}

func TestSendMessageTool_Call_SendError(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		sendMsgFn: func(_ context.Context, _ string, _ string) error {
			return errors.New("agent not found")
		},
	}
	ctx := &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}

	input, _ := json.Marshal(agent.SendMessageInput{AgentID: "ghost", Content: "hello"})
	result, err := agent.SendMessageTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for send error")
	}
	if !strings.Contains(result.Content.(string), "failed to send message") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}
