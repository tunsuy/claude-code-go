package permissions

import (
	"path/filepath"
	"testing"
)

func TestIsDangerousPathDangerousFiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{"gitconfig absolute", "/Users/alice/.gitconfig"},
		{"gitconfig relative", ".gitconfig"},
		{"bashrc nested", "/home/bob/projects/.bashrc"},
		{"zshrc", "~/.zshrc"},
		{"mcp json", ".mcp.json"},
		{"claude json", "/Users/alice/.claude.json"},
		{"ripgreprc", "/home/bob/.ripgreprc"},
		{"gitmodules", "/repo/.gitmodules"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !IsDangerousPath(tt.path) {
				t.Errorf("IsDangerousPath(%q) = false, want true", tt.path)
			}
		})
	}
}

func TestIsDangerousPathDangerousDirectories(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{"git dir", "/repo/.git"},
		{"git internals", "/repo/.git/hooks/pre-commit"},
		{"vscode", "/repo/.vscode/settings.json"},
		{"idea", "/repo/.idea/workspace.xml"},
		{"claude dir", "/repo/.claude/settings.json"},
		{"claude nested", "/repo/.claude/settings/local.json"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !IsDangerousPath(tt.path) {
				t.Errorf("IsDangerousPath(%q) = false, want true", tt.path)
			}
		})
	}
}

func TestIsDangerousPathSafePaths(t *testing.T) {
	t.Parallel()
	safe := []string{
		"main.go",
		"/Users/alice/dev/project/src/main.go",
		"docs/README.md",
		".github/workflows/ci.yml",
		"go.mod",
		"testdata/.gitkeep",       // similar prefix, different name
		"config/appsettings.json", // json but not a protected file
		"/repo/git/config",        // "git" without dot is not dangerous
	}
	for _, p := range safe {
		if IsDangerousPath(p) {
			t.Errorf("IsDangerousPath(%q) = true, want false", p)
		}
	}
}

func TestIsDangerousPathCaseInsensitive(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"/repo/.GIT/config", "/repo/.BASHRC", "/repo/.Claude/settings.json"} {
		if !IsDangerousPath(p) {
			t.Errorf("IsDangerousPath(%q) = false, want true (case-insensitive)", p)
		}
	}
}

func TestCheckDangerousPathResult(t *testing.T) {
	t.Parallel()
	res := CheckDangerousPath("/repo/.git/config")
	if !res.Dangerous {
		t.Fatal("expected .git/config to be dangerous")
	}
	if res.Matched != ".git" {
		t.Errorf("Matched = %q, want .git", res.Matched)
	}
	if res.Reason == "" {
		t.Error("Reason should not be empty for a dangerous path")
	}

	res = CheckDangerousPath("/repo/main.go")
	if res.Dangerous || res.Matched != "" || res.Reason != "" {
		t.Errorf("unexpected result for safe path: %+v", res)
	}
}

func TestExpandHome(t *testing.T) {
	t.Parallel()
	got, err := expandHome("/abs/path")
	if err != nil || got != "/abs/path" {
		t.Errorf("expandHome(/abs/path) = %q, %v", got, err)
	}
	got, err = expandHome("relative")
	if err != nil || got != "relative" {
		t.Errorf("expandHome(relative) = %q, %v", got, err)
	}
	// "~" and "~/..." expand to home.
	for in, wantSuffix := range map[string]string{"~": "", "~/": filepath.ToSlash(filepath.Join("", ""))} {
		_ = wantSuffix
		got, err = expandHome(in)
		if err != nil {
			t.Fatalf("expandHome(%q) error: %v", in, err)
		}
		if got == "" || !filepath.IsAbs(got) {
			t.Errorf("expandHome(%q) = %q, want absolute home path", in, got)
		}
	}
}
