package interact

import (
	"encoding/json"
	"fmt"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// ── EnterPlanMode ─────────────────────────────────────────────────────────────

// EnterPlanModeInput is the input schema for EnterPlanMode.
type EnterPlanModeInput struct {
	// Plan is a human-readable plan summary to display.
	Plan string `json:"plan,omitempty"`
}

// EnterPlanModeTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core plan-mode state management.
var EnterPlanModeTool tool.Tool = &enterPlanModeTool{}

type enterPlanModeTool struct{ tool.BaseTool }

func (t *enterPlanModeTool) Name() string { return "EnterPlanMode" }

func (t *enterPlanModeTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Enters plan mode, pausing execution until the plan is approved by the user.

Usage notes:
- Use when you want to present a plan and get user approval before proceeding
- Execution resumes only after ExitPlanMode is called (typically by the user)`
}

func (t *enterPlanModeTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"plan": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "Human-readable plan summary to display to the user",
			}),
		},
		[]string{},
	)
}

func (t *enterPlanModeTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *enterPlanModeTool) IsReadOnly(_ tool.Input) bool        { return true }
func (t *enterPlanModeTool) InterruptBehavior() tool.InterruptBehavior {
	return tool.InterruptBehaviorBlock
}
func (t *enterPlanModeTool) UserFacingName(_ tool.Input) string { return "EnterPlanMode" }

func (t *enterPlanModeTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core plan-mode state machine.
	return &tool.Result{IsError: true, Content: "EnterPlanMode not yet implemented (TODO(dep))"}, nil
}

// ── ExitPlanMode ──────────────────────────────────────────────────────────────

// ExitPlanModeInput is the input schema for ExitPlanMode.
type ExitPlanModeInput struct {
	// Approved is true if the user approved the plan, false to abort.
	Approved bool `json:"approved"`
}

// ExitPlanModeTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core plan-mode state management.
var ExitPlanModeTool tool.Tool = &exitPlanModeTool{}

type exitPlanModeTool struct{ tool.BaseTool }

func (t *exitPlanModeTool) Name() string { return "ExitPlanMode" }

func (t *exitPlanModeTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Exits plan mode. Set approved=true to proceed with the plan, false to abort.`
}

func (t *exitPlanModeTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"approved": tool.PropSchema(map[string]any{
				"type":        "boolean",
				"description": "True to approve and proceed; false to abort",
			}),
		},
		[]string{"approved"},
	)
}

func (t *exitPlanModeTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *exitPlanModeTool) IsReadOnly(_ tool.Input) bool        { return true }
func (t *exitPlanModeTool) UserFacingName(_ tool.Input) string  { return "ExitPlanMode" }

func (t *exitPlanModeTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core plan-mode state machine.
	return &tool.Result{IsError: true, Content: "ExitPlanMode not yet implemented (TODO(dep))"}, nil
}

// ── EnterWorktree ─────────────────────────────────────────────────────────────

// EnterWorktreeInput is the input schema for EnterWorktree.
type EnterWorktreeInput struct {
	// Name is an optional name for the new worktree branch.
	Name string `json:"name,omitempty"`
}

// EnterWorktreeTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core worktree manager.
var EnterWorktreeTool tool.Tool = &enterWorktreeTool{}

type enterWorktreeTool struct{ tool.BaseTool }

func (t *enterWorktreeTool) Name() string { return "EnterWorktree" }

func (t *enterWorktreeTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Creates and switches to an isolated git worktree for the current session.

Usage notes:
- Only usable when the user explicitly asks to work in a worktree
- Creates a new branch based on HEAD inside .claude/worktrees/
- Use ExitWorktree to leave the worktree`
}

func (t *enterWorktreeTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"name": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional name for the worktree; random name generated if omitted",
			}),
		},
		[]string{},
	)
}

func (t *enterWorktreeTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *enterWorktreeTool) IsReadOnly(_ tool.Input) bool        { return false }

func (t *enterWorktreeTool) UserFacingName(input tool.Input) string {
	var in EnterWorktreeInput
	if json.Unmarshal(input, &in) == nil && in.Name != "" {
		return fmt.Sprintf("EnterWorktree(%s)", in.Name)
	}
	return "EnterWorktree"
}

func (t *enterWorktreeTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core worktree manager.
	return &tool.Result{IsError: true, Content: "EnterWorktree not yet implemented (TODO(dep))"}, nil
}

// ── ExitWorktree ──────────────────────────────────────────────────────────────

// ExitWorktreeInput is the input schema for ExitWorktree.
type ExitWorktreeInput struct {
	// Action is "keep" or "remove".
	Action string `json:"action"`
	// DiscardChanges, when true, removes a worktree even if it has uncommitted changes.
	DiscardChanges bool `json:"discard_changes,omitempty"`
}

// ExitWorktreeTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core worktree manager.
var ExitWorktreeTool tool.Tool = &exitWorktreeTool{}

type exitWorktreeTool struct{ tool.BaseTool }

func (t *exitWorktreeTool) Name() string { return "ExitWorktree" }

func (t *exitWorktreeTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Exits the current worktree session and returns to the original directory.

Usage notes:
- action="keep" leaves the worktree on disk; action="remove" deletes it
- Set discard_changes=true to force removal even with uncommitted changes`
}

func (t *exitWorktreeTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"action": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "\"keep\" or \"remove\"",
				"enum":        []string{"keep", "remove"},
			}),
			"discard_changes": tool.PropSchema(map[string]any{
				"type":        "boolean",
				"description": "If true, removes even when there are uncommitted changes",
			}),
		},
		[]string{"action"},
	)
}

func (t *exitWorktreeTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *exitWorktreeTool) IsReadOnly(_ tool.Input) bool        { return false }
func (t *exitWorktreeTool) IsDestructive(_ tool.Input) bool     { return true }
func (t *exitWorktreeTool) UserFacingName(_ tool.Input) string  { return "ExitWorktree" }

func (t *exitWorktreeTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core worktree manager.
	return &tool.Result{IsError: true, Content: "ExitWorktree not yet implemented (TODO(dep))"}, nil
}
