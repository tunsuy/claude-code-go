package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/claude-code-go/internal/tools"
)

// ── Shared types ──────────────────────────────────────────────────────────────

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusStopped   TaskStatus = "stopped"
)

// Task is the canonical representation of a task record.
type Task struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at,omitempty"`
}

// ── TaskCreate ────────────────────────────────────────────────────────────────

// TaskCreateInput is the input schema for TaskCreate.
type TaskCreateInput struct {
	Description string   `json:"description"`
	// Tools is an optional list of tool names available to this task (optional).
	Tools []string `json:"tools,omitempty"`
	// Priority is an optional integer priority for the task (optional).
	Priority *int `json:"priority,omitempty"`
}

// TaskCreateTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core TaskManager.
var TaskCreateTool tools.Tool = &taskCreateTool{}

type taskCreateTool struct{ tools.BaseTool }

func (t *taskCreateTool) Name() string { return "TaskCreate" }

func (t *taskCreateTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return "Creates a new task and returns its ID."
}

func (t *taskCreateTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"description": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "A human-readable description of the task",
			}),
			"tools": tools.PropSchema(map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional list of tool names available to this task",
			}),
			"priority": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": "Optional task priority (higher number = higher priority)",
			}),
		},
		[]string{"description"},
	)
}

func (t *taskCreateTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *taskCreateTool) IsReadOnly(_ tools.Input) bool        { return false }

func (t *taskCreateTool) UserFacingName(input tools.Input) string {
	var in TaskCreateInput
	if json.Unmarshal(input, &in) == nil && in.Description != "" {
		desc := in.Description
		if len(desc) > 50 {
			desc = desc[:50] + "…"
		}
		return fmt.Sprintf("TaskCreate(%s)", desc)
	}
	return "TaskCreate"
}

func (t *taskCreateTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core TaskManager.
	return &tools.Result{IsError: true, Content: "TaskCreate not yet implemented (TODO(dep))"}, nil
}

// ── TaskGet ───────────────────────────────────────────────────────────────────

// TaskGetInput is the input schema for TaskGet.
type TaskGetInput struct {
	ID string `json:"id"`
}

// TaskGetTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core TaskManager.
var TaskGetTool tools.Tool = &taskGetTool{}

type taskGetTool struct{ tools.BaseTool }

func (t *taskGetTool) Name() string { return "TaskGet" }

func (t *taskGetTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return "Returns the current state of a task by ID."
}

func (t *taskGetTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"id": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The task ID to retrieve",
			}),
		},
		[]string{"id"},
	)
}

func (t *taskGetTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *taskGetTool) IsReadOnly(_ tools.Input) bool        { return true }

func (t *taskGetTool) UserFacingName(input tools.Input) string {
	var in TaskGetInput
	if json.Unmarshal(input, &in) == nil && in.ID != "" {
		return fmt.Sprintf("TaskGet(%s)", in.ID)
	}
	return "TaskGet"
}

func (t *taskGetTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core TaskManager.
	return &tools.Result{IsError: true, Content: "TaskGet not yet implemented (TODO(dep))"}, nil
}

// ── TaskList ──────────────────────────────────────────────────────────────────

// TaskListInput is the input schema for TaskList.
type TaskListInput struct {
	Status string `json:"status,omitempty"`
}

// TaskListTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core TaskManager.
var TaskListTool tools.Tool = &taskListTool{}

type taskListTool struct{ tools.BaseTool }

func (t *taskListTool) Name() string { return "TaskList" }

func (t *taskListTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return "Lists all tasks, optionally filtered by status."
}

func (t *taskListTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"status": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional status filter (pending, running, completed, failed, stopped)",
				"enum":        []string{"pending", "running", "completed", "failed", "stopped"},
			}),
		},
		[]string{},
	)
}

func (t *taskListTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *taskListTool) IsReadOnly(_ tools.Input) bool        { return true }
func (t *taskListTool) UserFacingName(_ tools.Input) string  { return "TaskList" }

func (t *taskListTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core TaskManager.
	return &tools.Result{IsError: true, Content: "TaskList not yet implemented (TODO(dep))"}, nil
}

// ── TaskUpdate ────────────────────────────────────────────────────────────────

// TaskUpdateInput is the input schema for TaskUpdate.
type TaskUpdateInput struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
}

// TaskUpdateTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core TaskManager.
var TaskUpdateTool tools.Tool = &taskUpdateTool{}

type taskUpdateTool struct{ tools.BaseTool }

func (t *taskUpdateTool) Name() string { return "TaskUpdate" }

func (t *taskUpdateTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return "Updates the description or status of an existing task."
}

func (t *taskUpdateTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"id": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The task ID to update",
			}),
			"description": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "New description (optional)",
			}),
			"status": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "New status (optional)",
				"enum":        []string{"pending", "running", "completed", "failed", "stopped"},
			}),
		},
		[]string{"id"},
	)
}

func (t *taskUpdateTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *taskUpdateTool) IsReadOnly(_ tools.Input) bool        { return false }

func (t *taskUpdateTool) UserFacingName(input tools.Input) string {
	var in TaskUpdateInput
	if json.Unmarshal(input, &in) == nil && in.ID != "" {
		return fmt.Sprintf("TaskUpdate(%s)", in.ID)
	}
	return "TaskUpdate"
}

func (t *taskUpdateTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core TaskManager.
	return &tools.Result{IsError: true, Content: "TaskUpdate not yet implemented (TODO(dep))"}, nil
}

// ── TaskStop ──────────────────────────────────────────────────────────────────

// TaskStopInput is the input schema for TaskStop.
type TaskStopInput struct {
	ID string `json:"id"`
}

// TaskStopTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core TaskManager.
var TaskStopTool tools.Tool = &taskStopTool{}

type taskStopTool struct{ tools.BaseTool }

func (t *taskStopTool) Name() string { return "TaskStop" }

func (t *taskStopTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return "Stops a running task and sets its status to stopped."
}

func (t *taskStopTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"id": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The task ID to stop",
			}),
		},
		[]string{"id"},
	)
}

func (t *taskStopTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *taskStopTool) IsReadOnly(_ tools.Input) bool        { return false }
func (t *taskStopTool) IsDestructive(_ tools.Input) bool     { return true }

func (t *taskStopTool) UserFacingName(input tools.Input) string {
	var in TaskStopInput
	if json.Unmarshal(input, &in) == nil && in.ID != "" {
		return fmt.Sprintf("TaskStop(%s)", in.ID)
	}
	return "TaskStop"
}

func (t *taskStopTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core TaskManager.
	return &tools.Result{IsError: true, Content: "TaskStop not yet implemented (TODO(dep))"}, nil
}

// ── TaskOutput ────────────────────────────────────────────────────────────────

// TaskOutputInput is the input schema for TaskOutput.
type TaskOutputInput struct {
	ID string `json:"id"`
	// Since is an optional byte offset; only output after this offset is returned (optional).
	Since *int `json:"since,omitempty"`
}

// TaskOutputTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core TaskManager.
var TaskOutputTool tools.Tool = &taskOutputTool{}

type taskOutputTool struct{ tools.BaseTool }

func (t *taskOutputTool) Name() string { return "TaskOutput" }

func (t *taskOutputTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return "Returns the captured output (stdout/stderr) of a task."
}

func (t *taskOutputTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"id": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The task ID whose output to retrieve",
			}),
			"since": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": "Optional byte offset; only output after this position is returned",
			}),
		},
		[]string{"id"},
	)
}

func (t *taskOutputTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *taskOutputTool) IsReadOnly(_ tools.Input) bool        { return true }

func (t *taskOutputTool) UserFacingName(input tools.Input) string {
	var in TaskOutputInput
	if json.Unmarshal(input, &in) == nil && in.ID != "" {
		return fmt.Sprintf("TaskOutput(%s)", in.ID)
	}
	return "TaskOutput"
}

func (t *taskOutputTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core TaskManager.
	return &tools.Result{IsError: true, Content: "TaskOutput not yet implemented (TODO(dep))"}, nil
}
