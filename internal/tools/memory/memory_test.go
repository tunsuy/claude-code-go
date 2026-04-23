package memory_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/memory"
)

// newInput marshals v into a json.RawMessage for tool input.
func newInput(v any) tools.Input {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestMemoryReadTool_Metadata(t *testing.T) {
	tool := memory.MemoryReadTool

	if tool.Name() != "MemoryRead" {
		t.Errorf("Name: got %q, want %q", tool.Name(), "MemoryRead")
	}
	if !tool.IsConcurrencySafe(nil) {
		t.Error("MemoryRead should be concurrency-safe")
	}
	if !tool.IsReadOnly(nil) {
		t.Error("MemoryRead should be read-only")
	}

	schema := tool.InputSchema()
	if schema.Type != "object" {
		t.Errorf("InputSchema.Type: got %q, want %q", schema.Type, "object")
	}
}

func TestMemoryWriteTool_Metadata(t *testing.T) {
	tool := memory.MemoryWriteTool

	if tool.Name() != "MemoryWrite" {
		t.Errorf("Name: got %q, want %q", tool.Name(), "MemoryWrite")
	}
	if tool.IsConcurrencySafe(nil) {
		t.Error("MemoryWrite should NOT be concurrency-safe")
	}
	if tool.IsReadOnly(nil) {
		t.Error("MemoryWrite should NOT be read-only")
	}

	schema := tool.InputSchema()
	if len(schema.Required) != 2 {
		t.Errorf("InputSchema.Required: got %d, want 2", len(schema.Required))
	}
}

func TestMemoryDeleteTool_Metadata(t *testing.T) {
	tool := memory.MemoryDeleteTool

	if tool.Name() != "MemoryDelete" {
		t.Errorf("Name: got %q, want %q", tool.Name(), "MemoryDelete")
	}
	if !tool.IsDestructive(nil) {
		t.Error("MemoryDelete should be destructive")
	}

	schema := tool.InputSchema()
	if len(schema.Required) != 1 {
		t.Errorf("InputSchema.Required: got %d, want 1", len(schema.Required))
	}
}

func TestMemoryWriteTool_ValidateInput(t *testing.T) {
	tool := memory.MemoryWriteTool

	tests := []struct {
		name   string
		input  any
		wantOK bool
	}{
		{
			name:   "valid input",
			input:  map[string]any{"title": "Test", "content": "Hello"},
			wantOK: true,
		},
		{
			name:   "missing title",
			input:  map[string]any{"content": "Hello"},
			wantOK: false,
		},
		{
			name:   "missing content",
			input:  map[string]any{"title": "Test"},
			wantOK: false,
		},
		{
			name:   "empty title",
			input:  map[string]any{"title": "", "content": "Hello"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := newInput(tt.input)
			result, err := tool.ValidateInput(input, nil)
			if err != nil {
				t.Fatalf("ValidateInput error: %v", err)
			}
			if result.OK != tt.wantOK {
				t.Errorf("ValidateInput.OK: got %v, want %v (reason: %s)", result.OK, tt.wantOK, result.Reason)
			}
		})
	}
}

func TestMemoryReadTool_CallListInWorkDir(t *testing.T) {
	// Change to a temp directory so getStore can find a project.
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	tool := memory.MemoryReadTool
	input := newInput(map[string]any{})
	result, err := tool.Call(input, &tools.UseContext{}, nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}
}

func TestMemoryWriteTool_CallSuccess(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		_ = os.Chdir(origDir)
	}()

	tool := memory.MemoryWriteTool
	input := newInput(map[string]any{
		"title":   "Test Write",
		"content": "This is a test memory written from a tool call.",
		"type":    "project",
	})
	result, err := tool.Call(input, &tools.UseContext{}, nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if result.IsError {
		t.Errorf("unexpected error: %v", result.Content)
	}

	// Verify we can read it back.
	readInput := newInput(map[string]any{})
	readResult, _ := memory.MemoryReadTool.Call(readInput, &tools.UseContext{}, nil)
	content, ok := readResult.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", readResult.Content)
	}
	if len(content) == 0 {
		t.Error("expected non-empty result from MemoryRead after write")
	}
}

func TestMemoryUserFacingName(t *testing.T) {
	tests := []struct {
		tool     tools.Tool
		input    any
		expected string
	}{
		{memory.MemoryReadTool, map[string]any{"name": "test"}, "MemoryRead(test)"},
		{memory.MemoryReadTool, map[string]any{}, "MemoryRead(list)"},
		{memory.MemoryWriteTool, map[string]any{"title": "My Pref"}, "MemoryWrite(My Pref)"},
		{memory.MemoryWriteTool, map[string]any{}, "MemoryWrite"},
		{memory.MemoryDeleteTool, map[string]any{"name": "old"}, "MemoryDelete(old)"},
		{memory.MemoryDeleteTool, map[string]any{}, "MemoryDelete"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			input := newInput(tt.input)
			got := tt.tool.UserFacingName(input)
			if got != tt.expected {
				t.Errorf("UserFacingName: got %q, want %q", got, tt.expected)
			}
		})
	}
}
