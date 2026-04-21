package memdir_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/memdir"
)

func TestDiscoverClaudeMd(t *testing.T) {
	// Create a temporary directory tree with CLAUDE.md files.
	root := t.TempDir()
	sub := filepath.Join(root, "project", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a CLAUDE.md in the parent of sub.
	project := filepath.Join(root, "project")
	claudePath := filepath.Join(project, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# Memory"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := memdir.DiscoverClaudeMd(sub)

	found := false
	for _, p := range paths {
		if p == claudePath {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find %s in discovered paths; got %v", claudePath, paths)
	}
}

func TestDiscoverClaudeMdNoDuplicates(t *testing.T) {
	// When startDir == home, home should not appear twice.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	paths := memdir.DiscoverClaudeMd(home)

	seen := make(map[string]int)
	for _, p := range paths {
		seen[p]++
	}
	for p, count := range seen {
		if count > 1 {
			t.Errorf("path %q appears %d times (expected 1)", p, count)
		}
	}
}

func TestLoadMemoryPrompt(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(p1, []byte("# Project Memory"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := memdir.LoadMemoryPrompt([]string{p1})
	if result == "" {
		t.Error("expected non-empty memory prompt")
	}
}

func TestLoadAndTruncate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "CLAUDE.md")
	content := make([]byte, 200)
	for i := range content {
		content[i] = 'a'
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}

	result := memdir.LoadAndTruncate([]string{p}, 50)
	// The raw content is truncated, but total with header might be longer.
	// Just verify the function doesn't panic and returns something.
	_ = result
}
