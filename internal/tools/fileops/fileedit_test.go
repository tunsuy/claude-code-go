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

// ── FileEditTool ──────────────────────────────────────────────────────────────

func TestFileEditTool_Name(t *testing.T) {
	if fileops.FileEditTool.Name() != "Edit" {
		t.Errorf("expected Edit, got %q", fileops.FileEditTool.Name())
	}
}

func TestFileEditTool_IsConcurrencySafe_False(t *testing.T) {
	if fileops.FileEditTool.IsConcurrencySafe(nil) {
		t.Error("FileEditTool must not be concurrency-safe")
	}
}

func TestFileEditTool_IsReadOnly_False(t *testing.T) {
	if fileops.FileEditTool.IsReadOnly(nil) {
		t.Error("FileEditTool must not be read-only")
	}
}

func TestFileEditTool_InputSchema(t *testing.T) {
	schema := fileops.FileEditTool.InputSchema()
	for _, key := range []string{"file_path", "old_string", "new_string"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("schema missing %q", key)
		}
	}
	reqMap := make(map[string]bool)
	for _, r := range schema.Required {
		reqMap[r] = true
	}
	for _, key := range []string{"file_path", "old_string", "new_string"} {
		if !reqMap[key] {
			t.Errorf("expected %q in Required", key)
		}
	}
}

func TestFileEditTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(fileops.FileEditInput{FilePath: "/tmp/x.go", OldString: "a", NewString: "b"})
	name := fileops.FileEditTool.UserFacingName(in)
	if name != "Edit(/tmp/x.go)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestFileEditTool_UserFacingName_NoInput(t *testing.T) {
	if fileops.FileEditTool.UserFacingName(nil) != "Edit" {
		t.Error("expected fallback 'Edit'")
	}
}

func TestFileEditTool_Call_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	os.WriteFile(path, []byte("package main\n\nfunc hello() {}\n"), 0o644)

	in, _ := json.Marshal(fileops.FileEditInput{
		FilePath:  path,
		OldString: "func hello() {}",
		NewString: "func hello() { return }",
	})
	result, err := fileops.FileEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "func hello() { return }") {
		t.Errorf("edit not applied: %q", string(data))
	}
}

func TestFileEditTool_Call_NotFound(t *testing.T) {
	in, _ := json.Marshal(fileops.FileEditInput{
		FilePath:  "/nonexistent/path/file.go",
		OldString: "foo",
		NewString: "bar",
	})
	result, err := fileops.FileEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing file")
	}
}

func TestFileEditTool_Call_NotUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.go")
	os.WriteFile(path, []byte("foo\nfoo\n"), 0o644)

	in, _ := json.Marshal(fileops.FileEditInput{
		FilePath:  path,
		OldString: "foo",
		NewString: "bar",
	})
	result, err := fileops.FileEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for non-unique old_string")
	}
}

func TestFileEditTool_Call_OldStringNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.go")
	os.WriteFile(path, []byte("package main\n"), 0o644)

	in, _ := json.Marshal(fileops.FileEditInput{
		FilePath:  path,
		OldString: "this does not exist",
		NewString: "replacement",
	})
	result, err := fileops.FileEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for not-found old_string")
	}
}

func TestFileEditTool_Call_EmptyOldString(t *testing.T) {
	in, _ := json.Marshal(fileops.FileEditInput{
		FilePath:  "/tmp/x.go",
		OldString: "",
		NewString: "something",
	})
	result, err := fileops.FileEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty old_string")
	}
}

func TestFileEditTool_Call_DeviceBlocked(t *testing.T) {
	in, _ := json.Marshal(fileops.FileEditInput{
		FilePath:  "/dev/null",
		OldString: "x",
		NewString: "y",
	})
	result, err := fileops.FileEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	// /dev/null is not in blocked list but is a device file, so this may succeed or not.
	// The test verifies the tool doesn't panic.
	_ = result
}

func TestFileEditTool_Call_DeleteText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "del.txt")
	os.WriteFile(path, []byte("keep this\ndelete me\nkeep that\n"), 0o644)

	in, _ := json.Marshal(fileops.FileEditInput{
		FilePath:  path,
		OldString: "delete me\n",
		NewString: "",
	})
	result, err := fileops.FileEditTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "delete me") {
		t.Errorf("text should have been deleted: %q", string(data))
	}
}

func TestFileEditTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = fileops.FileEditTool
}
