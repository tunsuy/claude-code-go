package fileops

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// FileEditInput is the input schema for the Edit tool.
type FileEditInput struct {
	// FilePath is the absolute path to edit (required).
	FilePath string `json:"file_path"`
	// OldString is the exact text to find and replace (required).
	OldString string `json:"old_string"`
	// NewString is the replacement text (required; may be empty to delete).
	NewString string `json:"new_string"`
}

// ── Tool implementation ───────────────────────────────────────────────────────

type fileEditTool struct{ tool.BaseTool }

// FileEditTool is the exported singleton instance.
// It implements tool.PathTool.
var FileEditTool tool.Tool = &fileEditTool{}

func (t *fileEditTool) Name() string { return "Edit" }

func (t *fileEditTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Performs exact string replacements in files.

Usage:
- You must use your Read tool at least once in the conversation before editing
- The edit will FAIL if old_string is not unique in the file; provide more context to make it unique
- Use replace_all=true (if available) to change every instance
- ALWAYS prefer editing existing files; NEVER write new files unless required
- old_string must match the existing content exactly, including indentation and whitespace`
}

func (t *fileEditTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"file_path": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to modify",
			}),
			"old_string": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The text to replace (must be unique in the file)",
			}),
			"new_string": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The replacement text",
			}),
		},
		[]string{"file_path", "old_string", "new_string"},
	)
}

func (t *fileEditTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *fileEditTool) IsReadOnly(_ tool.Input) bool         { return false }

func (t *fileEditTool) UserFacingName(input tool.Input) string {
	var in FileEditInput
	if json.Unmarshal(input, &in) == nil && in.FilePath != "" {
		return fmt.Sprintf("Edit(%s)", in.FilePath)
	}
	return "Edit"
}

// GetPath implements tool.PathTool.
func (t *fileEditTool) GetPath(input tool.Input) string {
	var in FileEditInput
	if json.Unmarshal(input, &in) == nil {
		return expandPath(in.FilePath)
	}
	return ""
}

// Call executes the FileEdit tool.
// TODO(dep): full permission checking requires Agent-Core permission system.
func (t *fileEditTool) Call(input tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	var in FileEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tool.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}
	if in.FilePath == "" {
		return &tool.Result{IsError: true, Content: "file_path is required"}, nil
	}
	if in.OldString == "" {
		return &tool.Result{IsError: true, Content: "old_string must not be empty"}, nil
	}

	fullPath := expandPath(in.FilePath)

	if isBlockedDevicePath(fullPath) {
		return &tool.Result{IsError: true, Content: fmt.Sprintf("cannot edit device file: %s", fullPath)}, nil
	}

	// Read existing content.
	data, err := os.ReadFile(fullPath)
	if os.IsNotExist(err) {
		return &tool.Result{IsError: true, Content: fmt.Sprintf("file does not exist: %s", fullPath)}, nil
	}
	if err != nil {
		return &tool.Result{IsError: true, Content: fmt.Sprintf("cannot read file: %v", err)}, nil
	}

	content := string(data)

	// Uniqueness check (§7.7 in design doc).
	count := strings.Count(content, in.OldString)
	switch count {
	case 0:
		return &tool.Result{
			IsError: true,
			Content: fmt.Sprintf("old_string not found in %s\nThe old_string must match exactly (including whitespace and indentation).", fullPath),
		}, nil
	case 1:
		// Exactly one match — proceed.
	default:
		return &tool.Result{
			IsError: true,
			Content: fmt.Sprintf("old_string matches %d locations in %s. Please provide more context to make it unique.", count, fullPath),
		}, nil
	}

	// TODO(dep): permission check via UseContext.PermCtx before writing.

	newContent := strings.Replace(content, in.OldString, in.NewString, 1)

	if err := writeFileAtomic(fullPath, []byte(newContent)); err != nil {
		return &tool.Result{IsError: true, Content: fmt.Sprintf("write failed: %v", err)}, nil
	}

	return &tool.Result{Content: fmt.Sprintf("Successfully edited %s", fullPath)}, nil
}
