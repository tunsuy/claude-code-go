package fileops

import (
	"encoding/json"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// FileWriteInput is the input schema for the Write tools.
type FileWriteInput struct {
	// FilePath is the absolute path to write (required).
	FilePath string `json:"file_path"`
	// Content is the content to write to the file (required).
	Content string `json:"content"`
}

// ── Tool implementation ───────────────────────────────────────────────────────

type fileWriteTool struct{ tools.BaseTool }

// FileWriteTool is the exported singleton instance.
// It implements tools.PathTool.
var FileWriteTool tools.Tool = &fileWriteTool{}

func (t *fileWriteTool) Name() string { return "Write" }

func (t *fileWriteTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Writes a file to the local filesystem.

Usage:
- This tool will overwrite the existing file if there is one at the provided path
- If this is an existing file, you MUST use the Read tool first to read the file's contents
- Prefer the Edit tool for modifying existing files — only use Write for new files or complete rewrites
- The file_path parameter must be an absolute path, not a relative path`
}

func (t *fileWriteTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"file_path": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to write (must be absolute, not relative)",
			}),
			"content": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The content to write to the file",
			}),
		},
		[]string{"file_path", "content"},
	)
}

func (t *fileWriteTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *fileWriteTool) IsReadOnly(_ tools.Input) bool         { return false }
func (t *fileWriteTool) IsDestructive(_ tools.Input) bool      { return true }

func (t *fileWriteTool) UserFacingName(input tools.Input) string {
	var in FileWriteInput
	if json.Unmarshal(input, &in) == nil && in.FilePath != "" {
		return fmt.Sprintf("Write(%s)", in.FilePath)
	}
	return "Write"
}

// GetPath implements tools.PathTool.
func (t *fileWriteTool) GetPath(input tools.Input) string {
	var in FileWriteInput
	if json.Unmarshal(input, &in) == nil {
		return expandPath(in.FilePath)
	}
	return ""
}

// Call executes the FileWrite tools.
// TODO(dep): full implementation requires the permissions system (Agent-Core).
// Path conflict detection with other write tools requires PathTool sub-interface
// support in the engine's partitionToolCalls.
func (t *fileWriteTool) Call(input tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in FileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}
	if in.FilePath == "" {
		return &tools.Result{IsError: true, Content: "file_path is required"}, nil
	}

	fullPath := expandPath(in.FilePath)

	if isBlockedDevicePath(fullPath) {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("cannot write to device file: %s", fullPath)}, nil
	}

	// TODO(dep): permission check via UseContext.PermCtx before writing.

	if err := writeFileAtomic(fullPath, []byte(in.Content)); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("write failed: %v", err)}, nil
	}

	return &tools.Result{Content: fmt.Sprintf("File written successfully: %s", fullPath)}, nil
}
