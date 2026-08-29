package fileops

import (
	"encoding/json"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

func TestCheckDangerousPathFiles(t *testing.T) {
	t.Parallel()
	dangerous := []string{
		"/Users/alice/.gitconfig",
		".bashrc",
		"/home/bob/project/.zshrc",
		"~/.zprofile",
		"/repo/.mcp.json",
		"/repo/.claude.json",
		"/repo/.gitmodules",
	}
	for _, p := range dangerous {
		matched, reason, dangerous := checkDangerousPath(p)
		if !dangerous {
			t.Errorf("checkDangerousPath(%q) not dangerous, want dangerous", p)
			continue
		}
		if matched == "" || reason == "" {
			t.Errorf("checkDangerousPath(%q) = (%q, %q); want non-empty matched and reason", p, matched, reason)
		}
	}
}

func TestCheckDangerousPathDirectories(t *testing.T) {
	t.Parallel()
	dangerous := []string{
		"/repo/.git",
		"/repo/.git/hooks/pre-commit",
		"/repo/.vscode/settings.json",
		"/repo/.idea/workspace.xml",
		"/repo/.claude/settings.json",
		"/repo/.claude/settings/local.json",
	}
	for _, p := range dangerous {
		if _, _, dangerous := checkDangerousPath(p); !dangerous {
			t.Errorf("checkDangerousPath(%q) not dangerous, want dangerous", p)
		}
	}
}

func TestCheckDangerousPathSafePaths(t *testing.T) {
	t.Parallel()
	safe := []string{
		"main.go",
		"/Users/alice/dev/project/src/main.go",
		"docs/README.md",
		".github/workflows/ci.yml",
		"go.mod",
		"testdata/.gitkeep",
		"config/appsettings.json",
		"/repo/git/config", // "git" without dot is not protected
		"",
	}
	for _, p := range safe {
		if _, _, dangerous := checkDangerousPath(p); dangerous {
			t.Errorf("checkDangerousPath(%q) dangerous, want safe", p)
		}
	}
}

func TestCheckDangerousPathCaseInsensitive(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"/repo/.GIT/config", "/repo/.BASHRC", "/repo/.Claude/settings.json"} {
		if _, _, dangerous := checkDangerousPath(p); !dangerous {
			t.Errorf("checkDangerousPath(%q) not dangerous, want dangerous (case-insensitive)", p)
		}
	}
}

func TestWriteTargetPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"file_path", `{"file_path":"/tmp/a.go","content":"x"}`, "/tmp/a.go"},
		{"notebook_path", `{"notebook_path":"/tmp/nb.ipynb"}`, "/tmp/nb.ipynb"},
		{"no path", `{"content":"x"}`, ""},
		{"invalid json", `{`, ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := writeTargetPath([]byte(tt.input)); got != tt.want {
				t.Errorf("writeTargetPath(%s) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWriteToolsDenyDangerousPaths(t *testing.T) {
	t.Parallel()
	mustJSON := func(v any) tools.Input {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return tools.Input(b)
	}

	tests := []struct {
		name  string
		tool  tools.Tool
		input tools.Input
	}{
		{
			name:  "Write .bashrc",
			tool:  FileWriteTool,
			input: mustJSON(FileWriteInput{FilePath: "/Users/alice/.bashrc", Content: "evil"}),
		},
		{
			name:  "Write inside .git",
			tool:  FileWriteTool,
			input: mustJSON(FileWriteInput{FilePath: "/repo/.git/config", Content: "evil"}),
		},
		{
			name:  "Edit .claude.json",
			tool:  FileEditTool,
			input: mustJSON(FileEditInput{FilePath: "/Users/alice/.claude.json", OldString: "a", NewString: "b"}),
		},
		{
			name:  "Edit inside .claude",
			tool:  FileEditTool,
			input: mustJSON(FileEditInput{FilePath: "/repo/.claude/settings.json", OldString: "a", NewString: "b"}),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := tt.tool.CheckPermissions(tt.input, nil)
			if err != nil {
				t.Fatalf("CheckPermissions error: %v", err)
			}
			if res.Behavior != tools.PermissionDeny {
				t.Errorf("behavior = %v, want deny (reason=%q)", res.Behavior, res.Reason)
			}
			if res.Reason == "" {
				t.Error("deny result should carry a reason")
			}
		})
	}
}

func TestWriteToolsAllowSafePaths(t *testing.T) {
	t.Parallel()
	mustJSON := func(v any) tools.Input {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return tools.Input(b)
	}

	inputs := []tools.Input{
		mustJSON(FileWriteInput{FilePath: "/repo/src/main.go", Content: "package main"}),
		mustJSON(FileEditInput{FilePath: "/repo/README.md", OldString: "a", NewString: "b"}),
		mustJSON(map[string]any{"notebook_path": "/repo/analysis.ipynb"}),
	}
	for i, in := range inputs {
		var tool tools.Tool = FileWriteTool
		if i == 1 {
			tool = FileEditTool
		} else if i == 2 {
			tool = NotebookEditTool
		}
		res, err := tool.CheckPermissions(in, nil)
		if err != nil {
			t.Fatalf("CheckPermissions error: %v", err)
		}
		if res.Behavior != tools.PermissionPassthrough {
			t.Errorf("case %d: behavior = %v, want passthrough", i, res.Behavior)
		}
	}
}
