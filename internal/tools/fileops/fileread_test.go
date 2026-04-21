package fileops_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/fileops"
)

// ── FileReadTool ──────────────────────────────────────────────────────────────

func TestFileReadTool_Name(t *testing.T) {
	if fileops.FileReadTool.Name() != "Read" {
		t.Errorf("expected Read, got %q", fileops.FileReadTool.Name())
	}
}

func TestFileReadTool_IsConcurrencySafe_True(t *testing.T) {
	if !fileops.FileReadTool.IsConcurrencySafe(nil) {
		t.Error("FileReadTool should be concurrency-safe")
	}
}

func TestFileReadTool_IsReadOnly_True(t *testing.T) {
	if !fileops.FileReadTool.IsReadOnly(nil) {
		t.Error("FileReadTool should be read-only")
	}
}

func TestFileReadTool_InputSchema(t *testing.T) {
	schema := fileops.FileReadTool.InputSchema()
	if _, ok := schema.Properties["file_path"]; !ok {
		t.Error("schema missing 'file_path'")
	}
	if _, ok := schema.Properties["offset"]; !ok {
		t.Error("schema missing 'offset'")
	}
	if _, ok := schema.Properties["limit"]; !ok {
		t.Error("schema missing 'limit'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "file_path" {
		t.Errorf("expected Required=[file_path], got %v", schema.Required)
	}
}

func TestFileReadTool_ValidateInput_EmptyPath(t *testing.T) {
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: "  "})
	vr, err := fileops.FileReadTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for empty file_path")
	}
}

func TestFileReadTool_ValidateInput_BadOffset(t *testing.T) {
	zero := 0
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: "/tmp/x", Offset: &zero})
	vr, err := fileops.FileReadTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for offset=0")
	}
}

func TestFileReadTool_ValidateInput_BadLimit(t *testing.T) {
	zero := 0
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: "/tmp/x", Limit: &zero})
	vr, err := fileops.FileReadTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for limit=0")
	}
}

func TestFileReadTool_ValidateInput_Valid(t *testing.T) {
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: "/tmp/x"})
	vr, err := fileops.FileReadTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vr.OK {
		t.Errorf("expected validation OK, got %q", vr.Reason)
	}
}

func TestFileReadTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: "/tmp/test.txt"})
	name := fileops.FileReadTool.UserFacingName(in)
	if name != "Read(/tmp/test.txt)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestFileReadTool_UserFacingName_NoInput(t *testing.T) {
	if fileops.FileReadTool.UserFacingName(nil) != "Read" {
		t.Error("expected fallback 'Read'")
	}
}

func TestFileReadTool_Call_ReadText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\n"
	_ = os.WriteFile(path, []byte(content), 0o644)

	in, _ := json.Marshal(fileops.FileReadInput{FilePath: path})
	result, err := fileops.FileReadTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	out, ok := result.Content.(fileops.FileReadOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if out.Type != "text" {
		t.Errorf("expected type=text, got %q", out.Type)
	}
	if out.TotalLines != 3 {
		t.Errorf("expected 3 lines, got %d", out.TotalLines)
	}
	if !strings.Contains(out.Content, "line1") || !strings.Contains(out.Content, "line3") {
		t.Errorf("expected content with all lines: %q", out.Content)
	}
}

func TestFileReadTool_Call_WithOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("A\nB\nC\nD\n"), 0o644)

	offset := 3
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: path, Offset: &offset})
	result, err := fileops.FileReadTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := result.Content.(fileops.FileReadOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if strings.Contains(out.Content, "A") || strings.Contains(out.Content, "B") {
		t.Errorf("expected only lines 3+, got %q", out.Content)
	}
	if !strings.Contains(out.Content, "C") {
		t.Errorf("expected line C in output: %q", out.Content)
	}
}

func TestFileReadTool_Call_WithLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("A\nB\nC\nD\nE\n"), 0o644)

	limit := 2
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: path, Limit: &limit})
	result, err := fileops.FileReadTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := result.Content.(fileops.FileReadOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if out.NumLines != 2 {
		t.Errorf("expected NumLines=2, got %d", out.NumLines)
	}
	if strings.Contains(out.Content, "C") || strings.Contains(out.Content, "D") {
		t.Errorf("expected only first 2 lines, got %q", out.Content)
	}
}

func TestFileReadTool_Call_NotFound(t *testing.T) {
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: "/nonexistent/path/file.txt"})
	result, err := fileops.FileReadTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing file")
	}
}

func TestFileReadTool_Call_Directory(t *testing.T) {
	dir := t.TempDir()
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: dir})
	result, err := fileops.FileReadTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for directory")
	}
}

func TestFileReadTool_Call_DeviceBlocked(t *testing.T) {
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: "/dev/zero"})
	result, err := fileops.FileReadTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for device file")
	}
}

func TestFileReadTool_Call_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bigfile.txt")
	// Write a file with enough lines to trigger cancellation check
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("some content line\n")
	}
	os.WriteFile(path, []byte(sb.String()), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	uc := &tools.UseContext{Ctx: ctx}
	in, _ := json.Marshal(fileops.FileReadInput{FilePath: path})
	result, err := fileops.FileReadTool.Call(in, uc, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	// May return cancelled or succeed if file is read before check — just verify no panic
	_ = result
}

func TestFileReadTool_Call_ImageFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	// Write minimal PNG-like data
	os.WriteFile(path, []byte("\x89PNG fake image data"), 0o644)

	in, _ := json.Marshal(fileops.FileReadInput{FilePath: path})
	result, err := fileops.FileReadTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	out, ok := result.Content.(fileops.FileReadOutput)
	if !ok {
		t.Fatalf("unexpected type: %T", result.Content)
	}
	if out.Type != "image" {
		t.Errorf("expected type=image, got %q", out.Type)
	}
	if out.Base64 == "" {
		t.Error("expected non-empty Base64")
	}
	if out.MediaType != "image/png" {
		t.Errorf("expected image/png, got %q", out.MediaType)
	}
}

func TestFileReadTool_MapResultToToolResultBlock_Text(t *testing.T) {
	out := fileops.FileReadOutput{Type: "text", FilePath: "/tmp/x", Content: "hello\n", TotalLines: 1}
	raw, err := fileops.FileReadTool.MapResultToToolResultBlock(out, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	_ = json.Unmarshal(raw, &block)
	if block["type"] != "tool_result" {
		t.Error("expected type=tool_result")
	}
	if block["content"] != "hello\n" {
		t.Errorf("unexpected content: %v", block["content"])
	}
}

func TestFileReadTool_MapResultToToolResultBlock_Image(t *testing.T) {
	out := fileops.FileReadOutput{
		Type:      "image",
		FilePath:  "/tmp/img.png",
		Base64:    "abc123",
		MediaType: "image/png",
	}
	raw, err := fileops.FileReadTool.MapResultToToolResultBlock(out, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	_ = json.Unmarshal(raw, &block)
	if block["type"] != "tool_result" {
		t.Error("expected type=tool_result")
	}
	contentList, ok := block["content"].([]any)
	if !ok || len(contentList) == 0 {
		t.Fatal("expected content to be a non-empty array for image")
	}
}

func TestFileReadTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = fileops.FileReadTool
}
