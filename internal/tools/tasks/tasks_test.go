package tasks_test

import (
	"encoding/json"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/tasks"
)

// ── TaskCreate ────────────────────────────────────────────────────────────────

func TestTaskCreateTool_Name(t *testing.T) {
	if tasks.TaskCreateTool.Name() != "TaskCreate" {
		t.Errorf("expected TaskCreate, got %q", tasks.TaskCreateTool.Name())
	}
}

func TestTaskCreateTool_IsConcurrencySafe_False(t *testing.T) {
	if tasks.TaskCreateTool.IsConcurrencySafe(nil) {
		t.Error("TaskCreate must not be concurrency-safe")
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
	json.Unmarshal(data, &m)
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
	json.Unmarshal(data, &m)
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
	json.Unmarshal(data, &m)
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
