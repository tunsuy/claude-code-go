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

// ── helpers ───────────────────────────────────────────────────────────────────

// makeTree creates a temp directory with the given files (relative paths).
// Returns the root dir path and a cleanup function.
func makeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("makeTree mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("makeTree write %s: %v", rel, err)
		}
	}
	return root
}

func globCall(t *testing.T, pattern, path string) fileops.GlobOutput {
	t.Helper()
	in, _ := json.Marshal(fileops.GlobInput{Pattern: pattern, Path: path})
	result, err := fileops.GlobTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("GlobTool.Call error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GlobTool.Call returned error: %v", result.Content)
	}
	out, ok := result.Content.(fileops.GlobOutput)
	if !ok {
		t.Fatalf("unexpected result type: %T", result.Content)
	}
	return out
}

// ── GlobTool tests ────────────────────────────────────────────────────────────

func TestGlobTool_Name(t *testing.T) {
	if fileops.GlobTool.Name() != "Glob" {
		t.Errorf("expected name Glob, got %q", fileops.GlobTool.Name())
	}
}

func TestGlobTool_IsConcurrencySafe(t *testing.T) {
	if !fileops.GlobTool.IsConcurrencySafe(nil) {
		t.Error("expected GlobTool to be concurrency-safe")
	}
}

func TestGlobTool_IsReadOnly(t *testing.T) {
	if !fileops.GlobTool.IsReadOnly(nil) {
		t.Error("expected GlobTool to be read-only")
	}
}

func TestGlobTool_ValidateInput_MissingPattern(t *testing.T) {
	in, _ := json.Marshal(fileops.GlobInput{Pattern: ""})
	vr, err := fileops.GlobTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for empty pattern")
	}
}

func TestGlobTool_ValidateInput_ValidPattern(t *testing.T) {
	in, _ := json.Marshal(fileops.GlobInput{Pattern: "**/*.go"})
	vr, err := fileops.GlobTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vr.OK {
		t.Errorf("expected validation OK, got reason: %q", vr.Reason)
	}
}

func TestGlobTool_SimpleExtension(t *testing.T) {
	root := makeTree(t, map[string]string{
		"a.go":     "package a",
		"b.go":     "package b",
		"c.ts":     "const x = 1",
		"sub/d.go": "package sub",
	})

	out := globCall(t, "**/*.go", root)
	if out.NumFiles != 3 {
		t.Errorf("expected 3 .go files, got %d: %v", out.NumFiles, out.Filenames)
	}
}

func TestGlobTool_RootPattern(t *testing.T) {
	root := makeTree(t, map[string]string{
		"a.go":     "package a",
		"sub/b.go": "package sub",
	})

	// *.go should only match top-level files (no **)
	out := globCall(t, "*.go", root)
	if out.NumFiles != 1 {
		t.Errorf("expected 1 top-level .go file, got %d: %v", out.NumFiles, out.Filenames)
	}
}

func TestGlobTool_DoubleStarMatchesNested(t *testing.T) {
	root := makeTree(t, map[string]string{
		"a/b/c/file.txt": "hello",
		"file.txt":       "top",
	})

	out := globCall(t, "**/*.txt", root)
	if out.NumFiles != 2 {
		t.Errorf("expected 2 .txt files, got %d: %v", out.NumFiles, out.Filenames)
	}
}

func TestGlobTool_PrefixPattern(t *testing.T) {
	root := makeTree(t, map[string]string{
		"src/a.ts":   "const a = 1",
		"src/b.ts":   "const b = 1",
		"test/c.ts":  "const c = 1",
	})

	out := globCall(t, "src/**/*.ts", root)
	if out.NumFiles != 2 {
		t.Errorf("expected 2 src/*.ts files, got %d: %v", out.NumFiles, out.Filenames)
	}
	for _, f := range out.Filenames {
		if !strings.Contains(f, "src") {
			t.Errorf("unexpected file outside src/: %s", f)
		}
	}
}

func TestGlobTool_NoMatches(t *testing.T) {
	root := makeTree(t, map[string]string{
		"a.go": "package a",
	})

	out := globCall(t, "**/*.rs", root)
	if out.NumFiles != 0 {
		t.Errorf("expected 0 matches, got %d", out.NumFiles)
	}
}

func TestGlobTool_SkipsHiddenDirs(t *testing.T) {
	root := makeTree(t, map[string]string{
		".git/config":  "[core]",
		"visible.go":   "package main",
	})

	out := globCall(t, "**/*", root)
	for _, f := range out.Filenames {
		if strings.Contains(f, ".git") {
			t.Errorf("expected .git to be skipped, but got: %s", f)
		}
	}
}

func TestGlobTool_NonExistentPath(t *testing.T) {
	in, _ := json.Marshal(fileops.GlobInput{Pattern: "**/*.go", Path: "/nonexistent/path/xyz"})
	result, err := fileops.GlobTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for non-existent path")
	}
}

func TestGlobTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(fileops.GlobInput{Pattern: "**/*.go"})
	name := fileops.GlobTool.UserFacingName(in)
	if name != "Glob(**/*.go)" {
		t.Errorf("unexpected UserFacingName: %q", name)
	}
}

func TestGlobTool_MapResultToToolResultBlock(t *testing.T) {
	out := fileops.GlobOutput{
		Filenames: []string{"/tmp/a.go", "/tmp/b.go"},
		NumFiles:  2,
	}
	raw, err := fileops.GlobTool.MapResultToToolResultBlock(out, "tool_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	if err := json.Unmarshal(raw, &block); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if block["type"] != "tool_result" {
		t.Errorf("expected type=tool_result")
	}
	if block["tool_use_id"] != "tool_123" {
		t.Errorf("expected tool_use_id=tool_123")
	}
	content := block["content"].(string)
	if !strings.Contains(content, "a.go") {
		t.Errorf("expected content to contain filenames, got %q", content)
	}
}

func TestGlobTool_MapResultToToolResultBlock_Empty(t *testing.T) {
	out := fileops.GlobOutput{Filenames: nil, NumFiles: 0}
	raw, err := fileops.GlobTool.MapResultToToolResultBlock(out, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	_ = json.Unmarshal(raw, &block)
	if !strings.Contains(block["content"].(string), "No files found") {
		t.Errorf("expected 'No files found' in empty result, got %q", block["content"])
	}
}

// ── Glob InputSchema ──────────────────────────────────────────────────────────

func TestGlobTool_InputSchema(t *testing.T) {
	schema := fileops.GlobTool.InputSchema()
	if schema.Type != "object" {
		t.Errorf("expected schema.Type=object, got %q", schema.Type)
	}
	if _, ok := schema.Properties["pattern"]; !ok {
		t.Error("expected 'pattern' in schema properties")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "pattern" {
		t.Errorf("expected Required=[pattern], got %v", schema.Required)
	}
}

// ── GlobTool interface compliance ─────────────────────────────────────────────

func TestGlobTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = fileops.GlobTool
}
