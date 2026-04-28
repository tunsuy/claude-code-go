package memdir_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/memdir"
)

func TestProcessIncludes_RelativePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create the included file.
	includedPath := filepath.Join(dir, "extra.md")
	if err := os.WriteFile(includedPath, []byte("included content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create the main file content with an @include directive.
	mainContent := "# Main\n@./extra.md\n# End"
	mainPath := filepath.Join(dir, "CLAUDE.md")

	result, err := memdir.ProcessIncludes(mainContent, mainPath, 0)
	if err != nil {
		t.Fatalf("ProcessIncludes returned error: %v", err)
	}

	if !strings.Contains(result, "included content") {
		t.Errorf("expected included content in result, got:\n%s", result)
	}
	if !strings.Contains(result, "# Main") {
		t.Errorf("expected # Main in result, got:\n%s", result)
	}
	if !strings.Contains(result, "# End") {
		t.Errorf("expected # End in result, got:\n%s", result)
	}
}

func TestProcessIncludes_HomePath(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	// Create a temp file in home for the test.
	tmpFile := filepath.Join(home, ".claude-test-include-"+t.Name()+".md")
	if err := os.WriteFile(tmpFile, []byte("home content"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	mainContent := "# Test\n@~/" + filepath.Base(tmpFile)
	mainPath := filepath.Join(t.TempDir(), "main.md")

	result, err := memdir.ProcessIncludes(mainContent, mainPath, 0)
	if err != nil {
		t.Fatalf("ProcessIncludes returned error: %v", err)
	}

	if !strings.Contains(result, "home content") {
		t.Errorf("expected home content in result, got:\n%s", result)
	}
}

func TestProcessIncludes_NestedIncludes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create nested include chain: main → level1 → level2.
	level2Path := filepath.Join(dir, "level2.md")
	if err := os.WriteFile(level2Path, []byte("deep content"), 0o644); err != nil {
		t.Fatal(err)
	}

	level1Path := filepath.Join(dir, "level1.md")
	if err := os.WriteFile(level1Path, []byte("@./level2.md"), 0o644); err != nil {
		t.Fatal(err)
	}

	mainContent := "top\n@./level1.md\nbottom"
	mainPath := filepath.Join(dir, "main.md")

	result, err := memdir.ProcessIncludes(mainContent, mainPath, 0)
	if err != nil {
		t.Fatalf("ProcessIncludes returned error: %v", err)
	}

	if !strings.Contains(result, "deep content") {
		t.Errorf("expected deep content in nested result, got:\n%s", result)
	}
	if !strings.Contains(result, "top") {
		t.Errorf("expected 'top' in result, got:\n%s", result)
	}
	if !strings.Contains(result, "bottom") {
		t.Errorf("expected 'bottom' in result, got:\n%s", result)
	}
}

func TestProcessIncludes_MaxDepth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create a file that includes itself (but we test depth limit, not circular).
	// We'll create a chain that exceeds MaxIncludeDepth.
	for i := 0; i <= memdir.MaxIncludeDepth+1; i++ {
		var content string
		if i < memdir.MaxIncludeDepth+1 {
			nextFile := filepath.Join(dir, nextFileName(i+1))
			_ = nextFile
			content = "@./" + nextFileName(i+1)
		} else {
			content = "final"
		}
		filePath := filepath.Join(dir, nextFileName(i))
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mainContent := "@./" + nextFileName(0)
	mainPath := filepath.Join(dir, "main.md")

	// Should not return an error for the main call (depth=0),
	// but deep includes beyond max depth won't be expanded.
	result, err := memdir.ProcessIncludes(mainContent, mainPath, 0)
	if err != nil {
		t.Fatalf("ProcessIncludes returned error: %v", err)
	}

	// The result should contain something (not empty).
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func nextFileName(i int) string {
	return "file" + string(rune('0'+i)) + ".md"
}

func TestProcessIncludes_CircularDetection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create circular includes: a.md → b.md → a.md.
	aPath := filepath.Join(dir, "a.md")
	bPath := filepath.Join(dir, "b.md")
	if err := os.WriteFile(aPath, []byte("A\n@./b.md"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("B\n@./a.md"), 0o644); err != nil {
		t.Fatal(err)
	}

	mainContent := "start\n@./a.md\nend"
	mainPath := filepath.Join(dir, "main.md")

	result, err := memdir.ProcessIncludes(mainContent, mainPath, 0)
	if err != nil {
		t.Fatalf("ProcessIncludes returned error: %v", err)
	}

	// Should contain the content from A and B but not recurse infinitely.
	if !strings.Contains(result, "start") {
		t.Errorf("expected 'start' in result, got:\n%s", result)
	}
	if !strings.Contains(result, "end") {
		t.Errorf("expected 'end' in result, got:\n%s", result)
	}
	// Should have a circular include comment.
	if !strings.Contains(result, "circular include skipped") {
		t.Errorf("expected circular include comment in result, got:\n%s", result)
	}
}

func TestProcessIncludes_FileNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mainContent := "before\n@./nonexistent.md\nafter"
	mainPath := filepath.Join(dir, "main.md")

	result, err := memdir.ProcessIncludes(mainContent, mainPath, 0)
	if err != nil {
		t.Fatalf("ProcessIncludes returned error: %v", err)
	}

	if !strings.Contains(result, "<!-- include not found: ./nonexistent.md -->") {
		t.Errorf("expected 'include not found' comment, got:\n%s", result)
	}
	if !strings.Contains(result, "before") {
		t.Errorf("expected 'before' in result, got:\n%s", result)
	}
	if !strings.Contains(result, "after") {
		t.Errorf("expected 'after' in result, got:\n%s", result)
	}
}

func TestProcessIncludes_NoIncludes(t *testing.T) {
	t.Parallel()

	content := "# Title\n\nJust regular content.\nNo includes here."
	mainPath := filepath.Join(t.TempDir(), "main.md")

	result, err := memdir.ProcessIncludes(content, mainPath, 0)
	if err != nil {
		t.Fatalf("ProcessIncludes returned error: %v", err)
	}

	if result != content {
		t.Errorf("expected content to pass through unchanged.\nGot:\n%s\nWant:\n%s", result, content)
	}
}

func TestProcessIncludes_IndentedDirective(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	includedPath := filepath.Join(dir, "extra.md")
	if err := os.WriteFile(includedPath, []byte("included"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Indented @include should still be recognized.
	mainContent := "# Title\n  @./extra.md\n# End"
	mainPath := filepath.Join(dir, "main.md")

	result, err := memdir.ProcessIncludes(mainContent, mainPath, 0)
	if err != nil {
		t.Fatalf("ProcessIncludes returned error: %v", err)
	}

	if !strings.Contains(result, "included") {
		t.Errorf("expected included content for indented directive, got:\n%s", result)
	}
}
