package fileops_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tool "github.com/anthropics/claude-code-go/internal/tool"
	"github.com/anthropics/claude-code-go/internal/tools/fileops"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func grepCall(t *testing.T, in fileops.GrepInput) fileops.GrepOutput {
	t.Helper()
	raw, _ := json.Marshal(in)
	result, err := fileops.GrepTool.Call(raw, nil, nil)
	if err != nil {
		t.Fatalf("GrepTool.Call error: %v", err)
	}
	if result.IsError {
		t.Fatalf("GrepTool returned error: %v", result.Content)
	}
	out, ok := result.Content.(fileops.GrepOutput)
	if !ok {
		t.Fatalf("unexpected result type: %T", result.Content)
	}
	return out
}

// makeTextTree creates files in a temp dir. Convenience wrapper around makeTree.
func makeTextTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	return root
}

// ── GrepTool identity tests ───────────────────────────────────────────────────

func TestGrepTool_Name(t *testing.T) {
	if fileops.GrepTool.Name() != "Grep" {
		t.Errorf("expected name Grep, got %q", fileops.GrepTool.Name())
	}
}

func TestGrepTool_IsConcurrencySafe(t *testing.T) {
	if !fileops.GrepTool.IsConcurrencySafe(nil) {
		t.Error("expected GrepTool to be concurrency-safe")
	}
}

func TestGrepTool_IsReadOnly(t *testing.T) {
	if !fileops.GrepTool.IsReadOnly(nil) {
		t.Error("expected GrepTool to be read-only")
	}
}

func TestGrepTool_ImplementsToolInterface(t *testing.T) {
	var _ tool.Tool = fileops.GrepTool
}

// ── ValidateInput ─────────────────────────────────────────────────────────────

func TestGrepTool_ValidateInput_EmptyPattern(t *testing.T) {
	in, _ := json.Marshal(fileops.GrepInput{Pattern: ""})
	vr, err := fileops.GrepTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for empty pattern")
	}
}

func TestGrepTool_ValidateInput_InvalidRegex(t *testing.T) {
	in, _ := json.Marshal(fileops.GrepInput{Pattern: "["}) // invalid regex
	vr, err := fileops.GrepTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for invalid regex")
	}
}

func TestGrepTool_ValidateInput_InvalidOutputMode(t *testing.T) {
	in, _ := json.Marshal(fileops.GrepInput{Pattern: "foo", OutputMode: "invalid"})
	vr, err := fileops.GrepTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vr.OK {
		t.Error("expected validation failure for invalid output_mode")
	}
}

func TestGrepTool_ValidateInput_Valid(t *testing.T) {
	in, _ := json.Marshal(fileops.GrepInput{Pattern: `\bfoo\b`, OutputMode: "content"})
	vr, err := fileops.GrepTool.ValidateInput(in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !vr.OK {
		t.Errorf("expected OK, got reason: %q", vr.Reason)
	}
}

// ── Content mode ──────────────────────────────────────────────────────────────

func TestGrepTool_ContentMode_BasicMatch(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"a.go": "package main\nfunc main() {}\n",
		"b.go": "package util\nfunc Helper() {}\n",
	})

	out := grepCall(t, fileops.GrepInput{
		Pattern:    "func",
		Path:       root,
		OutputMode: "content",
	})

	if out.NumResults != 2 {
		t.Errorf("expected 2 matches for 'func', got %d", out.NumResults)
	}
	for _, m := range out.Matches {
		if !strings.Contains(m.Content, "func") {
			t.Errorf("match content %q does not contain 'func'", m.Content)
		}
	}
}

func TestGrepTool_ContentMode_NoMatches(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"a.txt": "hello world\n",
	})

	out := grepCall(t, fileops.GrepInput{Pattern: "zzznomatch", Path: root})
	if out.NumResults != 0 {
		t.Errorf("expected 0 matches, got %d", out.NumResults)
	}
}

func TestGrepTool_ContentMode_MultiLineFile(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"f.txt": "apple\nbanana\napricot\ncherry\n",
	})

	out := grepCall(t, fileops.GrepInput{Pattern: "^a", Path: root})
	if out.NumResults != 2 {
		t.Errorf("expected 2 lines starting with 'a', got %d", out.NumResults)
	}
}

func TestGrepTool_ContentMode_RegexCapture(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"f.go": "// TODO: fix this\n// FIXME: that\n",
	})

	out := grepCall(t, fileops.GrepInput{Pattern: `TODO|FIXME`, Path: root})
	if out.NumResults != 2 {
		t.Errorf("expected 2 matches for TODO|FIXME, got %d", out.NumResults)
	}
}

func TestGrepTool_ContentMode_LineNumbers(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"f.txt": "one\ntwo\nthree\n",
	})

	out := grepCall(t, fileops.GrepInput{Pattern: "two", Path: root})
	if len(out.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(out.Matches))
	}
	if out.Matches[0].Line != 2 {
		t.Errorf("expected line 2, got %d", out.Matches[0].Line)
	}
}

// ── files_with_matches mode ───────────────────────────────────────────────────

func TestGrepTool_FilesWithMatchesMode(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"a.go":    "func Hello() {}",
		"b.go":    "func World() {}",
		"c.go":    "const X = 1",
		"sub/d.go": "func Sub() {}",
	})

	out := grepCall(t, fileops.GrepInput{
		Pattern:    "func",
		Path:       root,
		OutputMode: "files_with_matches",
	})

	if out.NumResults != 3 {
		t.Errorf("expected 3 files with 'func', got %d: %v", out.NumResults, out.Files)
	}
	if len(out.Matches) != 0 {
		t.Error("expected no Matches in files_with_matches mode")
	}
}

// ── count mode ────────────────────────────────────────────────────────────────

func TestGrepTool_CountMode(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"a.txt": "foo\nfoo\nfoo\n",
		"b.txt": "foo\n",
	})

	out := grepCall(t, fileops.GrepInput{
		Pattern:    "foo",
		Path:       root,
		OutputMode: "count",
	})

	if out.Counts == nil {
		t.Fatal("expected Counts map in count mode")
	}
	total := 0
	for _, c := range out.Counts {
		total += c
	}
	if total != 4 {
		t.Errorf("expected 4 total 'foo' occurrences, got %d", total)
	}
}

// ── Include filter ────────────────────────────────────────────────────────────

func TestGrepTool_IncludeFilter(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"a.go":  "func GoFunc() {}",
		"b.ts":  "function tsFunc() {}",
		"c.txt": "some text",
	})

	out := grepCall(t, fileops.GrepInput{
		Pattern: "func",
		Path:    root,
		Include: "*.go",
	})

	if out.NumResults != 1 {
		t.Errorf("expected 1 match (*.go only), got %d", out.NumResults)
	}
	if len(out.Matches) > 0 && !strings.HasSuffix(out.Matches[0].Path, ".go") {
		t.Errorf("expected match in .go file, got %s", out.Matches[0].Path)
	}
}

// ── Single file search ────────────────────────────────────────────────────────

func TestGrepTool_SingleFile(t *testing.T) {
	root := makeTextTree(t, map[string]string{
		"target.txt": "line1\ntarget line\nline3\n",
	})
	filePath := filepath.Join(root, "target.txt")

	out := grepCall(t, fileops.GrepInput{
		Pattern: "target",
		Path:    filePath,
	})

	if out.NumResults != 1 {
		t.Errorf("expected 1 match, got %d", out.NumResults)
	}
}

// ── Non-existent path ─────────────────────────────────────────────────────────

func TestGrepTool_NonExistentPath(t *testing.T) {
	raw, _ := json.Marshal(fileops.GrepInput{
		Pattern: "foo",
		Path:    "/nonexistent/path/xyz",
	})
	result, err := fileops.GrepTool.Call(raw, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for non-existent path")
	}
}

// ── MapResultToToolResultBlock ────────────────────────────────────────────────

func TestGrepTool_MapResultToToolResultBlock_Content(t *testing.T) {
	out := fileops.GrepOutput{
		Matches:    []fileops.GrepMatch{{Path: "/a.go", Line: 1, Content: "func main()"}},
		NumResults: 1,
	}
	raw, err := fileops.GrepTool.MapResultToToolResultBlock(out, "tid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var block map[string]any
	json.Unmarshal(raw, &block)
	if !strings.Contains(block["content"].(string), "func main()") {
		t.Errorf("expected match content in output, got %q", block["content"])
	}
}

func TestGrepTool_MapResultToToolResultBlock_NoMatches(t *testing.T) {
	out := fileops.GrepOutput{NumResults: 0}
	raw, _ := fileops.GrepTool.MapResultToToolResultBlock(out, "tid")
	var block map[string]any
	json.Unmarshal(raw, &block)
	if !strings.Contains(block["content"].(string), "No matches found") {
		t.Errorf("expected 'No matches found', got %q", block["content"])
	}
}

// ── UserFacingName ────────────────────────────────────────────────────────────

func TestGrepTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(fileops.GrepInput{Pattern: `\bfoo\b`})
	name := fileops.GrepTool.UserFacingName(in)
	if name != `Grep(\bfoo\b)` {
		t.Errorf("unexpected UserFacingName: %q", name)
	}
}
