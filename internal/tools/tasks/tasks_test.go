package tasks_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/tasks"
)

// mockCoordinator is a test double for tools.AgentCoordinator.
type mockCoordinator struct {
	spawnFn   func(ctx context.Context, req tools.AgentSpawnRequest) (string, error)
	sendMsgFn func(ctx context.Context, agentID string, message string) error
	stopFn    func(ctx context.Context, agentID string) error
	statusFn  func(ctx context.Context, agentID string) (string, error)
	resultFn  func(ctx context.Context, agentID string) (string, string, error)
	listFn    func() map[string]string
	waitFn    func(ctx context.Context, agentID string) (string, error)
}

func (m *mockCoordinator) SpawnAgent(ctx context.Context, req tools.AgentSpawnRequest) (string, error) {
	if m.spawnFn != nil {
		return m.spawnFn(ctx, req)
	}
	return "mock-task-1", nil
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

// newTestContext creates a UseContext with the given mock coordinator.
func newTestContext(mock *mockCoordinator) *tools.UseContext {
	return &tools.UseContext{
		Ctx:         context.Background(),
		Coordinator: mock,
	}
}

// ── TaskCreate ────────────────────────────────────────────────────────────────

func TestTaskCreateTool_Name(t *testing.T) {
	if tasks.TaskCreateTool.Name() != "TaskCreate" {
		t.Errorf("expected TaskCreate, got %q", tasks.TaskCreateTool.Name())
	}
}

func TestTaskCreateTool_IsConcurrencySafe_True(t *testing.T) {
	// TaskCreate spawns agents via the coordinator which is concurrency-safe,
	// allowing multiple tasks to be created in parallel within one LLM turn.
	if !tasks.TaskCreateTool.IsConcurrencySafe(nil) {
		t.Error("TaskCreate must be concurrency-safe to enable parallel task spawning")
	}
}

func TestTaskCreateTool_IsReadOnly_False(t *testing.T) {
	if tasks.TaskCreateTool.IsReadOnly(nil) {
		t.Error("TaskCreate must not be read-only")
	}
}

func TestTaskCreateTool_InputSchema_HasTools(t *testing.T) {
	// P1-4: schema must have "tools" field
	schema := tasks.TaskCreateTool.InputSchema()
	if _, ok := schema.Properties["tools"]; !ok {
		t.Error("TaskCreate schema missing 'tools'")
	}
}

func TestTaskCreateTool_InputSchema_HasPriority(t *testing.T) {
	// P1-4: schema must have "priority" field
	schema := tasks.TaskCreateTool.InputSchema()
	if _, ok := schema.Properties["priority"]; !ok {
		t.Error("TaskCreate schema missing 'priority'")
	}
}

func TestTaskCreateTool_InputSchema_Required(t *testing.T) {
	schema := tasks.TaskCreateTool.InputSchema()
	if len(schema.Required) != 1 || schema.Required[0] != "description" {
		t.Errorf("expected Required=[description], got %v", schema.Required)
	}
}

func TestTaskCreateInput_ToolsJsonTag(t *testing.T) {
	// P1-4: JSON round-trip for Tools field
	in := tasks.TaskCreateInput{
		Description: "my task",
		Tools:       []string{"Bash", "Read"},
	}
	data, _ := json.Marshal(in)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["tools"]; !ok {
		t.Error("TaskCreateInput.Tools must serialize as 'tools'")
	}
}

func TestTaskCreateInput_PriorityJsonTag(t *testing.T) {
	// P1-4: JSON round-trip for Priority field
	prio := 5
	in := tasks.TaskCreateInput{Description: "my task", Priority: &prio}
	data, _ := json.Marshal(in)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["priority"]; !ok {
		t.Error("TaskCreateInput.Priority must serialize as 'priority'")
	}
}

func TestTaskCreateTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskCreateInput{Description: "test task"})
	name := tasks.TaskCreateTool.UserFacingName(in)
	if name != "TaskCreate(test task)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestTaskCreateTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskCreateInput{Description: "something"})
	result, err := tasks.TaskCreateTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── TaskGet ───────────────────────────────────────────────────────────────────

func TestTaskGetTool_Name(t *testing.T) {
	if tasks.TaskGetTool.Name() != "TaskGet" {
		t.Errorf("expected TaskGet, got %q", tasks.TaskGetTool.Name())
	}
}

func TestTaskGetTool_IsConcurrencySafe_True(t *testing.T) {
	if !tasks.TaskGetTool.IsConcurrencySafe(nil) {
		t.Error("TaskGet should be concurrency-safe (read-only)")
	}
}

func TestTaskGetTool_IsReadOnly_True(t *testing.T) {
	if !tasks.TaskGetTool.IsReadOnly(nil) {
		t.Error("TaskGet should be read-only")
	}
}

func TestTaskGetTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskGetInput{ID: "task-123"})
	name := tasks.TaskGetTool.UserFacingName(in)
	if name != "TaskGet(task-123)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestTaskGetTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskGetInput{ID: "x"})
	result, err := tasks.TaskGetTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── TaskList ──────────────────────────────────────────────────────────────────

func TestTaskListTool_Name(t *testing.T) {
	if tasks.TaskListTool.Name() != "TaskList" {
		t.Errorf("expected TaskList, got %q", tasks.TaskListTool.Name())
	}
}

func TestTaskListTool_IsConcurrencySafe_True(t *testing.T) {
	if !tasks.TaskListTool.IsConcurrencySafe(nil) {
		t.Error("TaskList should be concurrency-safe")
	}
}

func TestTaskListTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskListInput{})
	result, err := tasks.TaskListTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── TaskUpdate ────────────────────────────────────────────────────────────────

func TestTaskUpdateTool_Name(t *testing.T) {
	if tasks.TaskUpdateTool.Name() != "TaskUpdate" {
		t.Errorf("expected TaskUpdate, got %q", tasks.TaskUpdateTool.Name())
	}
}

func TestTaskUpdateTool_IsConcurrencySafe_False(t *testing.T) {
	if tasks.TaskUpdateTool.IsConcurrencySafe(nil) {
		t.Error("TaskUpdate must not be concurrency-safe")
	}
}

func TestTaskUpdateTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskUpdateInput{ID: "t1"})
	name := tasks.TaskUpdateTool.UserFacingName(in)
	if name != "TaskUpdate(t1)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestTaskUpdateTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskUpdateInput{ID: "x"})
	result, err := tasks.TaskUpdateTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── TaskStop ──────────────────────────────────────────────────────────────────

func TestTaskStopTool_Name(t *testing.T) {
	if tasks.TaskStopTool.Name() != "TaskStop" {
		t.Errorf("expected TaskStop, got %q", tasks.TaskStopTool.Name())
	}
}

func TestTaskStopTool_IsDestructive(t *testing.T) {
	if !tasks.TaskStopTool.IsDestructive(nil) {
		t.Error("TaskStop must be destructive")
	}
}

func TestTaskStopTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskStopInput{ID: "t99"})
	name := tasks.TaskStopTool.UserFacingName(in)
	if name != "TaskStop(t99)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestTaskStopTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskStopInput{ID: "x"})
	result, err := tasks.TaskStopTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── TaskOutput ────────────────────────────────────────────────────────────────

func TestTaskOutputTool_Name(t *testing.T) {
	if tasks.TaskOutputTool.Name() != "TaskOutput" {
		t.Errorf("expected TaskOutput, got %q", tasks.TaskOutputTool.Name())
	}
}

func TestTaskOutputTool_IsConcurrencySafe_True(t *testing.T) {
	if !tasks.TaskOutputTool.IsConcurrencySafe(nil) {
		t.Error("TaskOutput should be concurrency-safe")
	}
}

func TestTaskOutputTool_IsReadOnly_True(t *testing.T) {
	if !tasks.TaskOutputTool.IsReadOnly(nil) {
		t.Error("TaskOutput should be read-only")
	}
}

func TestTaskOutputTool_InputSchema_HasSince(t *testing.T) {
	// P1-4: schema must have "since" field
	schema := tasks.TaskOutputTool.InputSchema()
	if _, ok := schema.Properties["since"]; !ok {
		t.Error("TaskOutput schema missing 'since'")
	}
}

func TestTaskOutputInput_SinceJsonTag(t *testing.T) {
	// P1-4: JSON round-trip for Since field
	since := 100
	in := tasks.TaskOutputInput{ID: "t1", Since: &since}
	data, _ := json.Marshal(in)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if _, ok := m["since"]; !ok {
		t.Error("TaskOutputInput.Since must serialize as 'since'")
	}
}

func TestTaskOutputTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskOutputInput{ID: "out-1"})
	name := tasks.TaskOutputTool.UserFacingName(in)
	if name != "TaskOutput(out-1)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestTaskOutputTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(tasks.TaskOutputInput{ID: "x"})
	result, err := tasks.TaskOutputTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestAllTaskTools_ImplementToolInterface(t *testing.T) {
	var _ tools.Tool = tasks.TaskCreateTool
	var _ tools.Tool = tasks.TaskGetTool
	var _ tools.Tool = tasks.TaskListTool
	var _ tools.Tool = tasks.TaskUpdateTool
	var _ tools.Tool = tasks.TaskStopTool
	var _ tools.Tool = tasks.TaskOutputTool
}

// ── TaskCreate Call() with mock coordinator ──────────────────────────────────

func TestTaskCreateTool_Call_Success(t *testing.T) {
	t.Parallel()
	var capturedReq tools.AgentSpawnRequest
	mock := &mockCoordinator{
		spawnFn: func(_ context.Context, req tools.AgentSpawnRequest) (string, error) {
			capturedReq = req
			return "task-100", nil
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskCreateInput{
		Description: "build feature X",
		Tools:       []string{"Read", "Write"},
	})
	result, err := tasks.TaskCreateTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	// Verify the spawned request.
	if capturedReq.Prompt != "build feature X" {
		t.Errorf("expected prompt 'build feature X', got %q", capturedReq.Prompt)
	}
	if len(capturedReq.AllowedTools) != 2 {
		t.Errorf("expected 2 AllowedTools, got %d", len(capturedReq.AllowedTools))
	}

	// Verify output structure.
	// The result now contains JSON followed by a guidance message.
	// Extract the JSON portion (first line before the blank line).
	content := result.Content.(string)
	parts := strings.SplitN(content, "\n\n", 2)
	if len(parts) < 2 {
		t.Fatalf("expected JSON + guidance message, got: %s", content)
	}

	var task tasks.Task
	if err := json.Unmarshal([]byte(parts[0]), &task); err != nil {
		t.Fatalf("failed to unmarshal JSON portion: %v", err)
	}
	if task.ID != "task-100" {
		t.Errorf("expected ID 'task-100', got %q", task.ID)
	}
	if task.Status != tasks.TaskStatusRunning {
		t.Errorf("expected status 'running', got %q", task.Status)
	}
	if task.Description != "build feature X" {
		t.Errorf("expected description 'build feature X', got %q", task.Description)
	}

	// Verify the guidance message mentions TaskGet.
	if !strings.Contains(parts[1], "TaskGet") {
		t.Errorf("expected guidance to mention TaskGet, got: %s", parts[1])
	}
	if !strings.Contains(parts[1], "Do NOT create another task") {
		t.Errorf("expected guidance to warn against duplicate creation, got: %s", parts[1])
	}
}

func TestTaskCreateTool_Call_EmptyDescription(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskCreateInput{Description: ""})
	result, err := tasks.TaskCreateTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty description")
	}
	if !strings.Contains(result.Content.(string), "description is required") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

func TestTaskCreateTool_Call_InvalidJSON(t *testing.T) {
	t.Parallel()
	ctx := newTestContext(&mockCoordinator{})

	result, err := tasks.TaskCreateTool.Call([]byte(`{bad`), ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON")
	}
}

func TestTaskCreateTool_Call_SpawnError(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		spawnFn: func(_ context.Context, _ tools.AgentSpawnRequest) (string, error) {
			return "", errors.New("resource exhausted")
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskCreateInput{Description: "some task"})
	result, err := tasks.TaskCreateTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for spawn error")
	}
	if !strings.Contains(result.Content.(string), "failed to create task") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

// ── TaskGet Call() with mock coordinator ─────────────────────────────────────

func TestTaskGetTool_Call_Success(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		resultFn: func(_ context.Context, agentID string) (string, string, error) {
			if agentID != "task-42" {
				t.Errorf("unexpected agentID: %q", agentID)
			}
			return "analysis done", "completed", nil
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskGetInput{ID: "task-42"})
	result, err := tasks.TaskGetTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	var task tasks.Task
	if err := json.Unmarshal([]byte(result.Content.(string)), &task); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if task.ID != "task-42" {
		t.Errorf("expected ID 'task-42', got %q", task.ID)
	}
	if task.Status != tasks.TaskStatusCompleted {
		t.Errorf("expected status 'completed', got %q", task.Status)
	}
	if task.Description != "analysis done" {
		t.Errorf("expected description 'analysis done', got %q", task.Description)
	}
}

func TestTaskGetTool_Call_EmptyID(t *testing.T) {
	t.Parallel()
	ctx := newTestContext(&mockCoordinator{})

	input, _ := json.Marshal(tasks.TaskGetInput{ID: ""})
	result, err := tasks.TaskGetTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty ID")
	}
}

func TestTaskGetTool_Call_NotFound(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		resultFn: func(_ context.Context, _ string) (string, string, error) {
			return "", "", errors.New("unknown agent")
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskGetInput{ID: "nonexistent"})
	result, err := tasks.TaskGetTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for not found")
	}
	if !strings.Contains(result.Content.(string), "task not found") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

// ── TaskList Call() with mock coordinator ────────────────────────────────────

func TestTaskListTool_Call_Success(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		listFn: func() map[string]string {
			return map[string]string{
				"task-1": "running",
				"task-2": "completed",
				"task-3": "running",
			}
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskListInput{})
	result, err := tasks.TaskListTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	var taskList []tasks.Task
	if err := json.Unmarshal([]byte(result.Content.(string)), &taskList); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(taskList) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(taskList))
	}
}

func TestTaskListTool_Call_FilterByStatus(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		listFn: func() map[string]string {
			return map[string]string{
				"task-1": "running",
				"task-2": "completed",
				"task-3": "running",
			}
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskListInput{Status: "running"})
	result, err := tasks.TaskListTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	var taskList []tasks.Task
	if err := json.Unmarshal([]byte(result.Content.(string)), &taskList); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(taskList) != 2 {
		t.Errorf("expected 2 running tasks, got %d", len(taskList))
	}
	for _, task := range taskList {
		if task.Status != tasks.TaskStatusRunning {
			t.Errorf("expected 'running', got %q", task.Status)
		}
	}
}

func TestTaskListTool_Call_EmptyList(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		listFn: func() map[string]string {
			return map[string]string{}
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskListInput{})
	result, err := tasks.TaskListTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	// Empty list serializes as "null" or "[]".
	content := result.Content.(string)
	if content != "null" && content != "[]" {
		t.Errorf("expected null or [], got %q", content)
	}
}

// ── TaskUpdate Call() with mock coordinator ──────────────────────────────────

func TestTaskUpdateTool_Call_Success(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		resultFn: func(_ context.Context, _ string) (string, string, error) {
			return "some result", "running", nil
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskUpdateInput{
		ID:          "task-10",
		Description: "updated desc",
	})
	result, err := tasks.TaskUpdateTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	var task tasks.Task
	if err := json.Unmarshal([]byte(result.Content.(string)), &task); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if task.ID != "task-10" {
		t.Errorf("expected ID 'task-10', got %q", task.ID)
	}
	if task.Description != "updated desc" {
		t.Errorf("expected description 'updated desc', got %q", task.Description)
	}
}

func TestTaskUpdateTool_Call_StopStatus(t *testing.T) {
	t.Parallel()
	var stopCalled bool
	mock := &mockCoordinator{
		stopFn: func(_ context.Context, agentID string) error {
			stopCalled = true
			if agentID != "task-10" {
				t.Errorf("unexpected agentID: %q", agentID)
			}
			return nil
		},
		resultFn: func(_ context.Context, _ string) (string, string, error) {
			return "", "stopped", nil
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskUpdateInput{
		ID:     "task-10",
		Status: "stopped",
	})
	result, err := tasks.TaskUpdateTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}
	if !stopCalled {
		t.Error("expected StopAgent to be called")
	}
}

func TestTaskUpdateTool_Call_EmptyID(t *testing.T) {
	t.Parallel()
	ctx := newTestContext(&mockCoordinator{})

	input, _ := json.Marshal(tasks.TaskUpdateInput{ID: ""})
	result, err := tasks.TaskUpdateTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty ID")
	}
}

func TestTaskUpdateTool_Call_StopError(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		stopFn: func(_ context.Context, _ string) error {
			return errors.New("already stopped")
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskUpdateInput{ID: "t1", Status: "stopped"})
	result, err := tasks.TaskUpdateTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stop error")
	}
	if !strings.Contains(result.Content.(string), "failed to stop task") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

func TestTaskUpdateTool_Call_NotFound(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		resultFn: func(_ context.Context, _ string) (string, string, error) {
			return "", "", errors.New("unknown agent")
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskUpdateInput{ID: "ghost", Description: "new"})
	result, err := tasks.TaskUpdateTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for not found")
	}
}

// ── TaskStop Call() with mock coordinator ────────────────────────────────────

func TestTaskStopTool_Call_Success(t *testing.T) {
	t.Parallel()
	var stopCalled bool
	mock := &mockCoordinator{
		stopFn: func(_ context.Context, agentID string) error {
			stopCalled = true
			if agentID != "task-99" {
				t.Errorf("unexpected agentID: %q", agentID)
			}
			return nil
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskStopInput{ID: "task-99"})
	result, err := tasks.TaskStopTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}
	if !stopCalled {
		t.Error("expected StopAgent to be called")
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(result.Content.(string)), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if out["id"] != "task-99" {
		t.Errorf("expected id 'task-99', got %q", out["id"])
	}
	if out["status"] != "stopped" {
		t.Errorf("expected status 'stopped', got %q", out["status"])
	}
}

func TestTaskStopTool_Call_EmptyID(t *testing.T) {
	t.Parallel()
	ctx := newTestContext(&mockCoordinator{})

	input, _ := json.Marshal(tasks.TaskStopInput{ID: ""})
	result, err := tasks.TaskStopTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty ID")
	}
}

func TestTaskStopTool_Call_StopError(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		stopFn: func(_ context.Context, _ string) error {
			return errors.New("agent not found")
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskStopInput{ID: "ghost"})
	result, err := tasks.TaskStopTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stop error")
	}
	if !strings.Contains(result.Content.(string), "failed to stop task") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}

// ── TaskOutput Call() with mock coordinator ──────────────────────────────────

func TestTaskOutputTool_Call_Success(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		resultFn: func(_ context.Context, agentID string) (string, string, error) {
			if agentID != "task-out" {
				t.Errorf("unexpected agentID: %q", agentID)
			}
			return "full output text here", "completed", nil
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskOutputInput{ID: "task-out"})
	result, err := tasks.TaskOutputTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(result.Content.(string)), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if out["id"] != "task-out" {
		t.Errorf("expected id 'task-out', got %q", out["id"])
	}
	if out["output"] != "full output text here" {
		t.Errorf("expected full output, got %q", out["output"])
	}
	if out["status"] != "completed" {
		t.Errorf("expected status 'completed', got %q", out["status"])
	}
}

func TestTaskOutputTool_Call_WithSinceOffset(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		resultFn: func(_ context.Context, _ string) (string, string, error) {
			return "abcdefghij", "running", nil
		},
	}
	ctx := newTestContext(mock)

	since := 5
	input, _ := json.Marshal(tasks.TaskOutputInput{ID: "task-partial", Since: &since})
	result, err := tasks.TaskOutputTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(result.Content.(string)), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if out["output"] != "fghij" {
		t.Errorf("expected 'fghij', got %q", out["output"])
	}
}

func TestTaskOutputTool_Call_SinceBeyondLength(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		resultFn: func(_ context.Context, _ string) (string, string, error) {
			return "short", "completed", nil
		},
	}
	ctx := newTestContext(mock)

	since := 100
	input, _ := json.Marshal(tasks.TaskOutputInput{ID: "task-over", Since: &since})
	result, err := tasks.TaskOutputTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected IsError: %v", result.Content)
	}

	// since >= len(output), so output should be full (since condition not met).
	var out map[string]string
	if err := json.Unmarshal([]byte(result.Content.(string)), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if out["output"] != "short" {
		t.Errorf("expected 'short' (since beyond length), got %q", out["output"])
	}
}

func TestTaskOutputTool_Call_EmptyID(t *testing.T) {
	t.Parallel()
	ctx := newTestContext(&mockCoordinator{})

	input, _ := json.Marshal(tasks.TaskOutputInput{ID: ""})
	result, err := tasks.TaskOutputTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty ID")
	}
}

func TestTaskOutputTool_Call_NotFound(t *testing.T) {
	t.Parallel()
	mock := &mockCoordinator{
		resultFn: func(_ context.Context, _ string) (string, string, error) {
			return "", "", errors.New("unknown agent")
		},
	}
	ctx := newTestContext(mock)

	input, _ := json.Marshal(tasks.TaskOutputInput{ID: "ghost"})
	result, err := tasks.TaskOutputTool.Call(input, ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for not found")
	}
	if !strings.Contains(result.Content.(string), "task not found") {
		t.Errorf("unexpected error message: %v", result.Content)
	}
}
