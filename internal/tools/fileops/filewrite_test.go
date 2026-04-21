package fileops_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/fileops"
)

// ── FileWriteTool ─────────────────────────────────────────────────────────────

func TestFileWriteTool_Name(t *testing.T) {
	if fileops.FileWriteTool.Name() != "Write" {
		t.Errorf("expected Write, got %q", fileops.FileWriteTool.Name())
	}
}

func TestFileWriteTool_IsConcurrencySafe_False(t *testing.T) {
	if fileops.FileWriteTool.IsConcurrencySafe(nil) {
		t.Error("FileWriteTool must not be concurrency-safe")
	}
}

func TestFileWriteTool_IsReadOnly_False(t *testing.T) {
	if fileops.FileWriteTool.IsReadOnly(nil) {
		t.Error("FileWriteTool must not be read-only")
	}
}

func TestFileWriteTool_IsDestructive_True(t *testing.T) {
	if !fileops.FileWriteTool.IsDestructive(nil) {
		t.Error("FileWriteTool must be destructive")
	}
}

func TestFileWriteTool_InputSchema(t *testing.T) {
	schema := fileops.FileWriteTool.InputSchema()
	if _, ok := schema.Properties["file_path"]; !ok {
		t.Error("schema missing 'file_path'")
	}
	if _, ok := schema.Properties["content"]; !ok {
		t.Error("schema missing 'content'")
	}
	reqMap := make(map[string]bool)
	for _, r := range schema.Required {
		reqMap[r] = true
	}
	if !reqMap["file_path"] || !reqMap["content"] {
		t.Errorf("expected file_path and content in Required, got %v", schema.Required)
	}
}

func TestFileWriteTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(fileops.FileWriteInput{FilePath: "/tmp/out.txt", Content: "hello"})
	name := fileops.FileWriteTool.UserFacingName(in)
	if name != "Write(/tmp/out.txt)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestFileWriteTool_UserFacingName_NoInput(t *testing.T) {
	if fileops.FileWriteTool.UserFacingName(nil) != "Write" {
		t.Error("expected fallback 'Write'")
	}
}

func TestFileWriteTool_Call_CreateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newfile.txt")

	in, _ := json.Marshal(fileops.FileWriteInput{FilePath: path, Content: "hello world"})
	result, err := fileops.FileWriteTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestFileWriteTool_Call_OverwriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	_ = os.WriteFile(path, []byte("old content"), 0o644)

	in, _ := json.Marshal(fileops.FileWriteInput{FilePath: path, Content: "new content"})
	result, err := fileops.FileWriteTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new content" {
		t.Errorf("expected new content, got %q", string(data))
	}
}

func TestFileWriteTool_Call_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.txt")

	in, _ := json.Marshal(fileops.FileWriteInput{FilePath: path, Content: "deep write"})
	result, err := fileops.FileWriteTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "deep write" {
		t.Errorf("unexpected content: %q", string(data))
	}
}

func TestFileWriteTool_Call_EmptyFilePath(t *testing.T) {
	in, _ := json.Marshal(fileops.FileWriteInput{FilePath: "", Content: "x"})
	result, err := fileops.FileWriteTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty file_path")
	}
}

func TestFileWriteTool_Call_DeviceBlocked(t *testing.T) {
	in, _ := json.Marshal(fileops.FileWriteInput{FilePath: "/dev/zero", Content: "x"})
	result, err := fileops.FileWriteTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for device file")
	}
}

func TestFileWriteTool_Call_InvalidJSON(t *testing.T) {
	result, err := fileops.FileWriteTool.Call([]byte("not-json"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON")
	}
}

func TestFileWriteTool_Call_WriteEmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	in, _ := json.Marshal(fileops.FileWriteInput{FilePath: path, Content: ""})
	result, err := fileops.FileWriteTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	data, _ := os.ReadFile(path)
	if len(data) != 0 {
		t.Errorf("expected empty file, got %q", string(data))
	}
}

func TestFileWriteTool_Call_SuccessMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "success.txt")

	in, _ := json.Marshal(fileops.FileWriteInput{FilePath: path, Content: "test"})
	result, err := fileops.FileWriteTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %v", result.Content)
	}
	msg, ok := result.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", result.Content)
	}
	if !strings.Contains(msg, path) {
		t.Errorf("expected path in success message: %q", msg)
	}
}

func TestFileWriteTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = fileops.FileWriteTool
}
