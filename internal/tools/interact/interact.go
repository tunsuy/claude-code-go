package interact

import (
	"encoding/json"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// ── TodoWrite ─────────────────────────────────────────────────────────────────

// TodoItem represents a single TODO list entry.
type TodoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"` // "pending" | "in_progress" | "completed"
	// Priority is optional; higher number = higher priority.
	Priority int `json:"priority,omitempty"`
}

// TodoWriteInput is the input schema for TodoWrite.
type TodoWriteInput struct {
	// Todos is the complete list of TODO items to write (replaces existing list).
	Todos []TodoItem `json:"todos"`
}

// TodoWriteTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core session state / UI layer.
var TodoWriteTool tool.Tool = &todoWriteTool{}

type todoWriteTool struct{ tool.BaseTool }

func (t *todoWriteTool) Name() string { return "TodoWrite" }

func (t *todoWriteTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Writes the complete TODO list for the current session, replacing the existing list.

Usage notes:
- Provide the full list on every call; partial updates are not supported
- Status must be one of: pending, in_progress, completed`
}

func (t *todoWriteTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"todos": tool.PropSchema(map[string]any{
				"type":        "array",
				"description": "The complete list of TODO items",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":       map[string]any{"type": "string"},
						"content":  map[string]any{"type": "string"},
						"status":   map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
						"priority": map[string]any{"type": "integer"},
					},
					"required": []string{"id", "content", "status"},
				},
			}),
		},
		[]string{"todos"},
	)
}

func (t *todoWriteTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *todoWriteTool) IsReadOnly(_ tool.Input) bool        { return false }
func (t *todoWriteTool) UserFacingName(_ tool.Input) string  { return "TodoWrite" }

func (t *todoWriteTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core session state / UI layer.
	return &tool.Result{IsError: true, Content: "TodoWrite not yet implemented (TODO(dep))"}, nil
}

// ── AskUserQuestion ───────────────────────────────────────────────────────────

// AskUserQuestionInput is the input schema for AskUserQuestion.
type AskUserQuestionInput struct {
	// Question is the question to ask the user (required).
	Question string `json:"question"`
	// Options is an optional list of suggested reply options.
	Options []string `json:"options,omitempty"`
}

// AskUserQuestionTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core UI/interaction layer.
var AskUserQuestionTool tool.Tool = &askUserQuestionTool{}

type askUserQuestionTool struct{ tool.BaseTool }

func (t *askUserQuestionTool) Name() string { return "AskUserQuestion" }

func (t *askUserQuestionTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Pauses execution and asks the user a clarifying question, waiting for their response.

Usage notes:
- Use only when genuinely blocked and cannot proceed without user input
- Prefer inference over asking when possible
- Optionally provide response options to guide the user`
}

func (t *askUserQuestionTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"question": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The question to ask the user",
			}),
			"options": tool.PropSchema(map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional suggested response options",
			}),
		},
		[]string{"question"},
	)
}

func (t *askUserQuestionTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *askUserQuestionTool) IsReadOnly(_ tool.Input) bool        { return true }
func (t *askUserQuestionTool) InterruptBehavior() tool.InterruptBehavior {
	return tool.InterruptBehaviorBlock
}
func (t *askUserQuestionTool) UserFacingName(_ tool.Input) string { return "AskUserQuestion" }

func (t *askUserQuestionTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core UI layer (bubbletea input).
	return &tool.Result{IsError: true, Content: "AskUserQuestion not yet implemented (TODO(dep))"}, nil
}
