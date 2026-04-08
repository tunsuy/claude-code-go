package fileops_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/tools"
	"github.com/anthropics/claude-code-go/internal/tools/fileops"
)

// ── NotebookEditTool ──────────────────────────────────────────────────────────

func TestNotebookEditTool_Name(t *testing.T) {
	if fileops.NotebookEditTool.Name() != "NotebookEdit" {
		t.Errorf("expected NotebookEdit, got %q", fileops.NotebookEditTool.Name())
	}
}

func TestNotebookEditTool_IsConcurrencySafe_False(t *testing.T) {
	if fileops.NotebookEditTool.IsConcurrencySafe(nil) {
		t.Error("NotebookEditTool must not be concurrency-safe")
	}
}

func TestNotebookEditTool_IsReadOnly_False(t *testing.T) {
	if fileops.NotebookEditTool.IsReadOnly(nil) {
		t.Error("NotebookEditTool must not be read-only")
	}
}

func TestNotebookEditTool_InputSchema_RequiredFields(t *testing.T) {
	schema := fileops.NotebookEditTool.InputSchema()
	for _, key := range []string{"notebook_path", "cell_number", "new_source"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("schema missing %q", key)
		}
	}
	reqMap := make(map[string]bool)
	for _, r := range schema.Required {
		reqMap[r] = true
	}
	for _, key := range []string{"notebook_path", "cell_number", "new_source"} {
		if !reqMap[key] {
			t.Errorf("expected %q in Required", key)
		}
	}
}

func TestNotebookEditTool_InputSchema_OptionalFields(t *testing.T) {
	schema := fileops.NotebookEditTool.InputSchema()
	for _, key := range []string{"cell_type", "edit_mode"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("schema missing optional field %q", key)
		}
	}
}

func TestNotebookEditTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(fileops.NotebookEditInput{
		NotebookPath: "/tmp/nb.ipynb",
		CellNumber:   0,
		NewSource:    "print('hello')",
	})
	name := fileops.NotebookEditTool.UserFacingName(in)
	if name != "NotebookEdit(/tmp/nb.ipynb)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestNotebookEditTool_UserFacingName_NoInput(t *testing.T) {
	if fileops.NotebookEditTool.UserFacingName(nil) != "NotebookEdit" {
		t.Error("expected fallback 'NotebookEdit'")
	}
}

func TestNotebookEditTool_Call_ReturnsNotImplemented(t *testing.T) {
	in, _ := json.Marshal(fileops.NotebookEditInput{
		NotebookPath: "/tmp/nb.ipynb",
		CellNumber:   0,
		NewSource:    "print('hello')",
	})
	result, err := fileops.NotebookEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for unimplemented tool")
	}
	msg, ok := result.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", result.Content)
	}
	if !strings.Contains(msg, "not yet implemented") {
		t.Errorf("expected 'not yet implemented' in message, got %q", msg)
	}
}

func TestNotebookEditTool_Call_InvalidJSON(t *testing.T) {
	result, err := fileops.NotebookEditTool.Call([]byte("not-json"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON")
	}
}

func TestNotebookEditTool_Call_WithEditMode(t *testing.T) {
	in, _ := json.Marshal(fileops.NotebookEditInput{
		NotebookPath: "/tmp/nb.ipynb",
		CellNumber:   1,
		NewSource:    "# new cell",
		CellType:     "markdown",
		EditMode:     "insert",
	})
	result, err := fileops.NotebookEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for unimplemented tool")
	}
}

func TestNotebookEditTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = fileops.NotebookEditTool
}
