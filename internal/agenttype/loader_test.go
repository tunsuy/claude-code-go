package agenttype

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCustomAgents_NonexistentDir(t *testing.T) {
	t.Parallel()
	profiles, err := LoadCustomAgents("/nonexistent/path/agents")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profiles != nil {
		t.Error("expected nil profiles for nonexistent directory")
	}
}

func TestLoadCustomAgents_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	profiles, err := LoadCustomAgents(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestLoadCustomAgents_JSONFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	jsonContent := `{
		"name": "linter",
		"display_name": "Linter Agent",
		"description": "Runs linting",
		"system_prompt": "You are a linting agent.",
		"model": "haiku",
		"max_turns": 5,
		"tools": {"mode": "allowlist", "list": ["Bash", "Read"]}
	}`
	if err := os.WriteFile(filepath.Join(dir, "linter.json"), []byte(jsonContent), 0o644); err != nil {
		t.Fatal(err)
	}

	profiles, err := LoadCustomAgents(dir)
	if err != nil {
		t.Fatalf("LoadCustomAgents error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	p := profiles[0]
	if p.Type != "linter" {
		t.Errorf("Type = %q, want %q", p.Type, "linter")
	}
	if p.DisplayName != "Linter Agent" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName, "Linter Agent")
	}
	if p.Model != "haiku" {
		t.Errorf("Model = %q, want %q", p.Model, "haiku")
	}
	if p.MaxTurns != 5 {
		t.Errorf("MaxTurns = %d, want 5", p.MaxTurns)
	}
	if p.ToolFilter.Mode != ToolFilterAllowlist {
		t.Errorf("ToolFilter.Mode = %q, want %q", p.ToolFilter.Mode, ToolFilterAllowlist)
	}
}

func TestLoadCustomAgents_MarkdownWithFrontmatter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	mdContent := `---
name: reviewer
display_name: Code Reviewer
model: opus
max_turns: 15
---
You are a code review agent. Review code for quality issues.`

	if err := os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte(mdContent), 0o644); err != nil {
		t.Fatal(err)
	}

	profiles, err := LoadCustomAgents(dir)
	if err != nil {
		t.Fatalf("LoadCustomAgents error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	p := profiles[0]
	if p.Type != "reviewer" {
		t.Errorf("Type = %q, want %q", p.Type, "reviewer")
	}
	if p.Model != "opus" {
		t.Errorf("Model = %q, want %q", p.Model, "opus")
	}
	if p.SystemPrompt != "You are a code review agent. Review code for quality issues." {
		t.Errorf("SystemPrompt = %q", p.SystemPrompt)
	}
}

func TestLoadCustomAgents_MarkdownWithoutFrontmatter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	mdContent := `You are a simple agent with no frontmatter.`
	if err := os.WriteFile(filepath.Join(dir, "simple.md"), []byte(mdContent), 0o644); err != nil {
		t.Fatal(err)
	}

	profiles, err := LoadCustomAgents(dir)
	if err != nil {
		t.Fatalf("LoadCustomAgents error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	p := profiles[0]
	if p.Type != "simple" {
		t.Errorf("Type = %q, want %q", p.Type, "simple")
	}
	if p.SystemPrompt != "You are a simple agent with no frontmatter." {
		t.Errorf("SystemPrompt = %q", p.SystemPrompt)
	}
}

func TestLoadCustomAgents_SkipsUnknownExtensions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "notes.yaml"), []byte("also ignore"), 0o644)

	profiles, err := LoadCustomAgents(dir)
	if err != nil {
		t.Fatalf("LoadCustomAgents error: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestLoadCustomAgents_SkipsInvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{invalid json"), 0o644)

	profiles, err := LoadCustomAgents(dir)
	if err != nil {
		t.Fatalf("LoadCustomAgents error: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles (invalid skipped), got %d", len(profiles))
	}
}

func TestLoadCustomAgents_Multiple(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"name":"a","system_prompt":"agent a"}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.md"), []byte("Agent B prompt"), 0o644)

	profiles, err := LoadCustomAgents(dir)
	if err != nil {
		t.Fatalf("LoadCustomAgents error: %v", err)
	}
	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}
}
