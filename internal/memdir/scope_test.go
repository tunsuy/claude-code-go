package memdir_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/memdir"
)

func TestDiscoverAll_AllScopes(t *testing.T) {
	t.Parallel()

	// Set up a project directory with a .git marker.
	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create Project scope: CLAUDE.md
	projectClaude := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(projectClaude, []byte("# Project"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create Project scope: .claude/CLAUDE.md
	dotClaude := filepath.Join(root, ".claude")
	if err := os.MkdirAll(dotClaude, 0o755); err != nil {
		t.Fatal(err)
	}
	dotClaudeMd := filepath.Join(dotClaude, "CLAUDE.md")
	if err := os.WriteFile(dotClaudeMd, []byte("# Dot Claude"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create Local scope: CLAUDE.local.md
	localClaude := filepath.Join(root, "CLAUDE.local.md")
	if err := os.WriteFile(localClaude, []byte("# Local"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := memdir.DiscoverAll(root)
	if err != nil {
		t.Fatalf("DiscoverAll returned error: %v", err)
	}

	// We should find at least the project and local files we created.
	foundProject := false
	foundDotClaude := false
	foundLocal := false
	for _, f := range files {
		switch f.Path {
		case projectClaude:
			foundProject = true
			if f.Scope != memdir.ScopeProject {
				t.Errorf("expected ScopeProject for %s, got %v", f.Path, f.Scope)
			}
		case dotClaudeMd:
			foundDotClaude = true
			if f.Scope != memdir.ScopeProject {
				t.Errorf("expected ScopeProject for %s, got %v", f.Path, f.Scope)
			}
		case localClaude:
			foundLocal = true
			if f.Scope != memdir.ScopeLocal {
				t.Errorf("expected ScopeLocal for %s, got %v", f.Path, f.Scope)
			}
		}
	}
	if !foundProject {
		t.Error("expected to find project CLAUDE.md")
	}
	if !foundDotClaude {
		t.Error("expected to find .claude/CLAUDE.md")
	}
	if !foundLocal {
		t.Error("expected to find CLAUDE.local.md")
	}

	// Verify ordering: local should come after project files.
	lastProjectIdx := -1
	localIdx := -1
	for i, f := range files {
		if f.Scope == memdir.ScopeProject {
			lastProjectIdx = i
		}
		if f.Path == localClaude {
			localIdx = i
		}
	}
	if localIdx >= 0 && lastProjectIdx >= 0 && localIdx <= lastProjectIdx {
		t.Errorf("local file (idx %d) should come after project files (last idx %d)", localIdx, lastProjectIdx)
	}
}

func TestDiscoverAll_ProjectOnly(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	projectClaude := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(projectClaude, []byte("# Project Only"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := memdir.DiscoverAll(root)
	if err != nil {
		t.Fatalf("DiscoverAll returned error: %v", err)
	}

	found := false
	for _, f := range files {
		if f.Path == projectClaude && f.Scope == memdir.ScopeProject {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find project CLAUDE.md in results: %v", files)
	}
}

func TestDiscoverAll_RulesDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rulesDir := filepath.Join(root, ".claude", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create rules in non-alphabetical order to test sorting.
	ruleC := filepath.Join(rulesDir, "c-conventions.md")
	ruleA := filepath.Join(rulesDir, "a-style.md")
	ruleB := filepath.Join(rulesDir, "b-testing.md")
	for _, p := range []string{ruleC, ruleA, ruleB} {
		if err := os.WriteFile(p, []byte("# Rule: "+filepath.Base(p)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a non-md file that should be ignored.
	ignored := filepath.Join(rulesDir, "notes.txt")
	if err := os.WriteFile(ignored, []byte("not a rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := memdir.DiscoverAll(root)
	if err != nil {
		t.Fatalf("DiscoverAll returned error: %v", err)
	}

	// Filter only rules.
	var rules []string
	for _, f := range files {
		if f.Scope == memdir.ScopeProject && filepath.Dir(f.Path) == rulesDir {
			rules = append(rules, filepath.Base(f.Path))
		}
	}

	expected := []string{"a-style.md", "b-testing.md", "c-conventions.md"}
	if len(rules) != len(expected) {
		t.Fatalf("expected %d rules, got %d: %v", len(expected), len(rules), rules)
	}
	for i, name := range expected {
		if rules[i] != name {
			t.Errorf("rule[%d]: expected %q, got %q", i, name, rules[i])
		}
	}
}

func TestDiscoverAll_NoGitRoot(t *testing.T) {
	t.Parallel()

	// Create a temp dir with no .git directory anywhere.
	root := t.TempDir()
	sub := filepath.Join(root, "deep", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create CLAUDE.md in the starting dir (fallback uses startDir as root).
	claudePath := filepath.Join(sub, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# No Git"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := memdir.DiscoverAll(sub)
	if err != nil {
		t.Fatalf("DiscoverAll returned error: %v", err)
	}

	// Should still find the CLAUDE.md relative to the fallback directory.
	found := false
	for _, f := range files {
		if f.Path == claudePath {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find %s in results when no .git exists: %v", claudePath, files)
	}
}

func TestFindGitRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// DiscoverAll should find files relative to the git root, not the sub.
	projectClaude := filepath.Join(root, "CLAUDE.md")
	if err := os.WriteFile(projectClaude, []byte("# Root Project"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := memdir.DiscoverAll(sub)
	if err != nil {
		t.Fatalf("DiscoverAll returned error: %v", err)
	}

	found := false
	for _, f := range files {
		if f.Path == projectClaude {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find git root CLAUDE.md from subdirectory; got %v", files)
	}
}

func TestMemoryScope_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scope memdir.MemoryScope
		want  string
	}{
		{memdir.ScopeManaged, "managed"},
		{memdir.ScopeUser, "user"},
		{memdir.ScopeProject, "project"},
		{memdir.ScopeLocal, "local"},
	}
	for _, tt := range tests {
		if got := tt.scope.String(); got != tt.want {
			t.Errorf("MemoryScope(%d).String() = %q, want %q", tt.scope, got, tt.want)
		}
	}
}
