package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tunsuy/claude-code-go/internal/tools"
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
	Description string `json:"description"`
	// Tools is an optional list of tool names available to this task (optional).
	Tools []string `json:"tools,omitempty"`
	// Priority is an optional integer priority for the task (optional).
	Priority *int `json:"priority,omitempty"`
}

// TaskCreateTool is the exported singleton instance.
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

func (t *taskCreateTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{IsError: true, Content: "TaskCreate is not available: coordinator mode is not enabled"}, nil
	}

	var in TaskCreateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if in.Description == "" {
		return &tools.Result{IsError: true, Content: "description is required"}, nil
	}

	// Spawn a new agent as a "task".
	agentID, err := ctx.Coordinator.SpawnAgent(ctx.Ctx, tools.AgentSpawnRequest{
		Description:  in.Description,
		Prompt:       in.Description,
		AllowedTools: in.Tools,
	})
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("failed to create task: %v", err)}, nil
	}

	task := Task{
		ID:          agentID,
		Description: in.Description,
		Status:      TaskStatusRunning,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	out, _ := json.Marshal(task)

	// Return a structured response that explicitly tells the LLM:
	// 1) The task is running in the background.
	// 2) Use TaskGet to poll for results — do NOT call TaskCreate again.
	result := fmt.Sprintf(
		"%s\n\nTask %s is now running in the background. "+
			"Do NOT create another task with the same description. "+
			"Use TaskGet with id=%q to check its status and retrieve results when it completes.",
		string(out), agentID, agentID,
	)
	return &tools.Result{Content: result}, nil
}

// ── TaskGet ───────────────────────────────────────────────────────────────────

// TaskGetInput is the input schema for TaskGet.
type TaskGetInput struct {
	ID string `json:"id"`
}

// TaskGetTool is the exported singleton instance.
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

func (t *taskGetTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{IsError: true, Content: "TaskGet is not available: coordinator mode is not enabled"}, nil
	}

	var in TaskGetInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if in.ID == "" {
		return &tools.Result{IsError: true, Content: "id is required"}, nil
	}

	result, status, err := ctx.Coordinator.GetAgentResult(ctx.Ctx, in.ID)
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("task not found: %v", err)}, nil
	}

	task := Task{
		ID:     in.ID,
		Status: TaskStatus(status),
	}
	if result != "" {
		task.Description = result
	}

	out, _ := json.Marshal(task)
	return &tools.Result{Content: string(out)}, nil
}

// ── TaskList ──────────────────────────────────────────────────────────────────

// TaskListInput is the input schema for TaskList.
type TaskListInput struct {
	Status string `json:"status,omitempty"`
}

// TaskListTool is the exported singleton instance.
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

func (t *taskListTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{IsError: true, Content: "TaskList is not available: coordinator mode is not enabled"}, nil
	}

	var in TaskListInput
	// Tolerate missing input (all fields optional).
	_ = json.Unmarshal(input, &in)

	agents := ctx.Coordinator.ListAgents()
	var taskList []Task
	for id, status := range agents {
		if in.Status != "" && status != in.Status {
			continue
		}
		taskList = append(taskList, Task{
			ID:     id,
			Status: TaskStatus(status),
		})
	}

	out, _ := json.Marshal(taskList)
	return &tools.Result{Content: string(out)}, nil
}

// ── TaskUpdate ────────────────────────────────────────────────────────────────

// TaskUpdateInput is the input schema for TaskUpdate.
type TaskUpdateInput struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
}

// TaskUpdateTool is the exported singleton instance.
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

func (t *taskUpdateTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{IsError: true, Content: "TaskUpdate is not available: coordinator mode is not enabled"}, nil
	}

	var in TaskUpdateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if in.ID == "" {
		return &tools.Result{IsError: true, Content: "id is required"}, nil
	}

	// If the update requests a "stopped" status, stop the agent.
	if in.Status == "stopped" {
		if err := ctx.Coordinator.StopAgent(ctx.Ctx, in.ID); err != nil {
			return &tools.Result{IsError: true, Content: fmt.Sprintf("failed to stop task: %v", err)}, nil
		}
	}

	// Retrieve the current state to confirm the update.
	_, status, err := ctx.Coordinator.GetAgentResult(ctx.Ctx, in.ID)
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("task not found: %v", err)}, nil
	}

	task := Task{
		ID:        in.ID,
		Status:    TaskStatus(status),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if in.Description != "" {
		task.Description = in.Description
	}

	out, _ := json.Marshal(task)
	return &tools.Result{Content: string(out)}, nil
}

// ── TaskStop ──────────────────────────────────────────────────────────────────

// TaskStopInput is the input schema for TaskStop.
type TaskStopInput struct {
	ID string `json:"id"`
}

// TaskStopTool is the exported singleton instance.
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

func (t *taskStopTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{IsError: true, Content: "TaskStop is not available: coordinator mode is not enabled"}, nil
	}

	var in TaskStopInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if in.ID == "" {
		return &tools.Result{IsError: true, Content: "id is required"}, nil
	}

	if err := ctx.Coordinator.StopAgent(ctx.Ctx, in.ID); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("failed to stop task %s: %v", in.ID, err)}, nil
	}

	result := map[string]string{
		"id":     in.ID,
		"status": "stopped",
	}
	out, _ := json.Marshal(result)
	return &tools.Result{Content: string(out)}, nil
}

// ── TaskOutput ────────────────────────────────────────────────────────────────

// TaskOutputInput is the input schema for TaskOutput.
type TaskOutputInput struct {
	ID string `json:"id"`
	// Since is an optional byte offset; only output after this offset is returned (optional).
	Since *int `json:"since,omitempty"`
}

// TaskOutputTool is the exported singleton instance.
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

func (t *taskOutputTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{IsError: true, Content: "TaskOutput is not available: coordinator mode is not enabled"}, nil
	}

	var in TaskOutputInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if in.ID == "" {
		return &tools.Result{IsError: true, Content: "id is required"}, nil
	}

	result, status, err := ctx.Coordinator.GetAgentResult(ctx.Ctx, in.ID)
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("task not found: %v", err)}, nil
	}

	output := result
	if in.Since != nil && *in.Since > 0 && *in.Since < len(output) {
		output = output[*in.Since:]
	}

	taskOutput := map[string]string{
		"id":     in.ID,
		"status": status,
		"output": output,
	}
	out, _ := json.Marshal(taskOutput)
	return &tools.Result{Content: string(out)}, nil
}
