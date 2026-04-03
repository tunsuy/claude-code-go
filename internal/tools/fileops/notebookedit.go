package fileops

import (
	"encoding/json"
	"fmt"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// NotebookEditTool is the exported singleton instance.
// TODO(dep): Full Jupyter notebook editing requires a proper .ipynb JSON parser.
// For now this provides the interface skeleton and basic validation.
var NotebookEditTool tool.Tool = &notebookEditTool{}

type notebookEditTool struct{ tool.BaseTool }

// NotebookEditInput is the input schema for the NotebookEdit tool.
type NotebookEditInput struct {
	// NotebookPath is the absolute path to the .ipynb file (required).
	NotebookPath string `json:"notebook_path"`
	// CellNumber is the 0-indexed cell to edit (required).
	CellNumber int `json:"cell_number"`
	// NewSource is the new cell source content (required).
	NewSource string `json:"new_source"`
	// CellType is "code" or "markdown" (optional, defaults to current type).
	CellType string `json:"cell_type,omitempty"`
	// EditMode is "replace", "insert", or "delete" (optional, defaults to "replace").
	EditMode string `json:"edit_mode,omitempty"`
}

func (t *notebookEditTool) Name() string { return "NotebookEdit" }

func (t *notebookEditTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Completely replaces the contents of a specific cell in a Jupyter notebook (.ipynb file).
The notebook_path parameter must be an absolute path. The cell_number is 0-indexed.
Use edit_mode=insert to add a new cell. Use edit_mode=delete to delete a cell.`
}

func (t *notebookEditTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"notebook_path": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The absolute path to the Jupyter notebook file",
			}),
			"cell_number": tool.PropSchema(map[string]any{
				"type":        "integer",
				"description": "The 0-indexed cell number to edit",
			}),
			"new_source": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The new source for the cell",
			}),
			"cell_type": tool.PropSchema(map[string]any{
				"type":        "string",
				"enum":        []string{"code", "markdown"},
				"description": `Cell type: "code" or "markdown"`,
			}),
			"edit_mode": tool.PropSchema(map[string]any{
				"type":        "string",
				"enum":        []string{"replace", "insert", "delete"},
				"description": `Edit mode: "replace" (default), "insert", or "delete"`,
			}),
		},
		[]string{"notebook_path", "cell_number", "new_source"},
	)
}

func (t *notebookEditTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *notebookEditTool) IsReadOnly(_ tool.Input) bool         { return false }

func (t *notebookEditTool) UserFacingName(input tool.Input) string {
	var in NotebookEditInput
	if json.Unmarshal(input, &in) == nil && in.NotebookPath != "" {
		return fmt.Sprintf("NotebookEdit(%s)", in.NotebookPath)
	}
	return "NotebookEdit"
}

// GetPath implements tool.PathTool.
func (t *notebookEditTool) GetPath(input tool.Input) string {
	var in NotebookEditInput
	if json.Unmarshal(input, &in) == nil {
		return expandPath(in.NotebookPath)
	}
	return ""
}

// Call executes the NotebookEdit tool.
// TODO(dep): Full .ipynb editing implementation. Currently returns an error
// indicating the feature is not yet implemented.
func (t *notebookEditTool) Call(input tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	var in NotebookEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tool.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}
	// TODO(dep): implement full notebook editing
	return &tool.Result{
		IsError: true,
		Content: "NotebookEdit is not yet implemented; requires .ipynb JSON parser",
	}, nil
}
