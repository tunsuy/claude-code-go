package interact_test

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/claude-code-go/internal/tools"
	"github.com/anthropics/claude-code-go/internal/tools/interact"
)

// ── TodoWriteTool ─────────────────────────────────────────────────────────────

func TestTodoWriteTool_Name(t *testing.T) {
	if interact.TodoWriteTool.Name() != "TodoWrite" {
		t.Errorf("expected TodoWrite, got %q", interact.TodoWriteTool.Name())
	}
}

func TestTodoWriteTool_IsConcurrencySafe_False(t *testing.T) {
	if interact.TodoWriteTool.IsConcurrencySafe(nil) {
		t.Error("TodoWriteTool must not be concurrency-safe")
	}
}

func TestTodoWriteTool_IsReadOnly_False(t *testing.T) {
	if interact.TodoWriteTool.IsReadOnly(nil) {
		t.Error("TodoWriteTool must not be read-only")
	}
}

func TestTodoWriteTool_InputSchema(t *testing.T) {
	schema := interact.TodoWriteTool.InputSchema()
	if _, ok := schema.Properties["todos"]; !ok {
		t.Error("schema missing 'todos'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "todos" {
		t.Errorf("expected Required=[todos], got %v", schema.Required)
	}
}

func TestTodoWriteTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(interact.TodoWriteInput{Todos: []interact.TodoItem{}})
	result, err := interact.TodoWriteTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── AskUserQuestionTool ───────────────────────────────────────────────────────

func TestAskUserQuestionTool_Name(t *testing.T) {
	if interact.AskUserQuestionTool.Name() != "AskUserQuestion" {
		t.Errorf("expected AskUserQuestion, got %q", interact.AskUserQuestionTool.Name())
	}
}

func TestAskUserQuestionTool_IsConcurrencySafe_False(t *testing.T) {
	if interact.AskUserQuestionTool.IsConcurrencySafe(nil) {
		t.Error("AskUserQuestionTool must not be concurrency-safe")
	}
}

func TestAskUserQuestionTool_IsReadOnly_True(t *testing.T) {
	if !interact.AskUserQuestionTool.IsReadOnly(nil) {
		t.Error("AskUserQuestionTool should be read-only")
	}
}

func TestAskUserQuestionTool_InputSchema(t *testing.T) {
	schema := interact.AskUserQuestionTool.InputSchema()
	if _, ok := schema.Properties["question"]; !ok {
		t.Error("schema missing 'question'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "question" {
		t.Errorf("expected Required=[question], got %v", schema.Required)
	}
}

func TestAskUserQuestionTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(interact.AskUserQuestionInput{Question: "what to do?"})
	result, err := interact.AskUserQuestionTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── EnterPlanModeTool ─────────────────────────────────────────────────────────

func TestEnterPlanModeTool_Name(t *testing.T) {
	if interact.EnterPlanModeTool.Name() != "EnterPlanMode" {
		t.Errorf("expected EnterPlanMode, got %q", interact.EnterPlanModeTool.Name())
	}
}

func TestEnterPlanModeTool_IsConcurrencySafe_False(t *testing.T) {
	if interact.EnterPlanModeTool.IsConcurrencySafe(nil) {
		t.Error("EnterPlanModeTool must not be concurrency-safe")
	}
}

func TestEnterPlanModeTool_IsReadOnly_True(t *testing.T) {
	if !interact.EnterPlanModeTool.IsReadOnly(nil) {
		t.Error("EnterPlanModeTool should be read-only")
	}
}

func TestEnterPlanModeTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(interact.EnterPlanModeInput{Plan: "step 1, step 2"})
	result, err := interact.EnterPlanModeTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── ExitPlanModeTool ──────────────────────────────────────────────────────────

func TestExitPlanModeTool_Name(t *testing.T) {
	if interact.ExitPlanModeTool.Name() != "ExitPlanMode" {
		t.Errorf("expected ExitPlanMode, got %q", interact.ExitPlanModeTool.Name())
	}
}

func TestExitPlanModeTool_IsConcurrencySafe_False(t *testing.T) {
	if interact.ExitPlanModeTool.IsConcurrencySafe(nil) {
		t.Error("ExitPlanModeTool must not be concurrency-safe")
	}
}

func TestExitPlanModeTool_InputSchema(t *testing.T) {
	schema := interact.ExitPlanModeTool.InputSchema()
	if _, ok := schema.Properties["approved"]; !ok {
		t.Error("schema missing 'approved'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "approved" {
		t.Errorf("expected Required=[approved], got %v", schema.Required)
	}
}

func TestExitPlanModeTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(interact.ExitPlanModeInput{Approved: true})
	result, err := interact.ExitPlanModeTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── EnterWorktreeTool ─────────────────────────────────────────────────────────

func TestEnterWorktreeTool_Name(t *testing.T) {
	if interact.EnterWorktreeTool.Name() != "EnterWorktree" {
		t.Errorf("expected EnterWorktree, got %q", interact.EnterWorktreeTool.Name())
	}
}

func TestEnterWorktreeTool_IsConcurrencySafe_False(t *testing.T) {
	if interact.EnterWorktreeTool.IsConcurrencySafe(nil) {
		t.Error("EnterWorktreeTool must not be concurrency-safe")
	}
}

func TestEnterWorktreeTool_IsReadOnly_False(t *testing.T) {
	if interact.EnterWorktreeTool.IsReadOnly(nil) {
		t.Error("EnterWorktreeTool must not be read-only")
	}
}

func TestEnterWorktreeTool_UserFacingName_WithName(t *testing.T) {
	in, _ := json.Marshal(interact.EnterWorktreeInput{Name: "my-branch"})
	name := interact.EnterWorktreeTool.UserFacingName(in)
	if name != "EnterWorktree(my-branch)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestEnterWorktreeTool_UserFacingName_NoName(t *testing.T) {
	in, _ := json.Marshal(interact.EnterWorktreeInput{})
	name := interact.EnterWorktreeTool.UserFacingName(in)
	if name != "EnterWorktree" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestEnterWorktreeTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(interact.EnterWorktreeInput{Name: "test"})
	result, err := interact.EnterWorktreeTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── ExitWorktreeTool ──────────────────────────────────────────────────────────

func TestExitWorktreeTool_Name(t *testing.T) {
	if interact.ExitWorktreeTool.Name() != "ExitWorktree" {
		t.Errorf("expected ExitWorktree, got %q", interact.ExitWorktreeTool.Name())
	}
}

func TestExitWorktreeTool_IsDestructive_True(t *testing.T) {
	if !interact.ExitWorktreeTool.IsDestructive(nil) {
		t.Error("ExitWorktreeTool must be destructive")
	}
}

func TestExitWorktreeTool_IsConcurrencySafe_False(t *testing.T) {
	if interact.ExitWorktreeTool.IsConcurrencySafe(nil) {
		t.Error("ExitWorktreeTool must not be concurrency-safe")
	}
}

func TestExitWorktreeTool_InputSchema(t *testing.T) {
	schema := interact.ExitWorktreeTool.InputSchema()
	if _, ok := schema.Properties["action"]; !ok {
		t.Error("schema missing 'action'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "action" {
		t.Errorf("expected Required=[action], got %v", schema.Required)
	}
}

func TestExitWorktreeTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(interact.ExitWorktreeInput{Action: "keep"})
	result, err := interact.ExitWorktreeTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestAllInteractTools_ImplementToolInterface(t *testing.T) {
	var _ tools.Tool = interact.TodoWriteTool
	var _ tools.Tool = interact.AskUserQuestionTool
	var _ tools.Tool = interact.EnterPlanModeTool
	var _ tools.Tool = interact.ExitPlanModeTool
	var _ tools.Tool = interact.EnterWorktreeTool
	var _ tools.Tool = interact.ExitWorktreeTool
}
