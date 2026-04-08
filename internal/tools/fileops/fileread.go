package fileops

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/claude-code-go/internal/tools"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// FileReadInput matches the TS inputSchema exactly.
type FileReadInput struct {
	// FilePath is the absolute path to read (required).
	FilePath string `json:"file_path"`
	// Offset is the 1-indexed start line (optional, default 1).
	Offset *int `json:"offset,omitempty"`
	// Limit is the maximum number of lines to read (optional, default all).
	Limit *int `json:"limit,omitempty"`
}

// FileReadOutput is the structured output for text files.
type FileReadOutput struct {
	// Type is one of "text" | "image" | "binary".
	Type string `json:"type"`
	// FilePath is the resolved path that was read.
	FilePath string `json:"file_path"`
	// Content holds the file text (for type=="text").
	Content string `json:"content,omitempty"`
	// NumLines is the number of lines in Content.
	NumLines int `json:"num_lines,omitempty"`
	// StartLine is the 1-indexed first line of Content.
	StartLine int `json:"start_line,omitempty"`
	// TotalLines is the total number of lines in the file.
	TotalLines int `json:"total_lines,omitempty"`
	// Base64 holds base64-encoded image data (for type=="image").
	Base64 string `json:"base64,omitempty"`
	// MediaType is the MIME type of image data.
	MediaType string `json:"media_type,omitempty"`
}

// maxFileReadLines is a safety cap; the model should use offset/limit instead.
const maxFileReadLines = 20_000

// ── Tool implementation ───────────────────────────────────────────────────────

type fileReadTool struct{ tools.BaseTool }

// FileReadTool is the exported singleton instance.
// It implements tools.PathTool (the engine can call GetPath via type assertion).
var FileReadTool tools.Tool = &fileReadTool{}

func (t *fileReadTool) Name() string { return "Read" }

func (t *fileReadTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Reads a file from the local filesystem. You can access any file directly by using this tools.
Assume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default it reads up to 2000 lines starting from the beginning of the file
- You can optionally specify a line offset and limit (especially handy for long files), but it's recommended to read the whole file by not providing these parameters
- Results are returned with line numbers starting at 1
- This tool can read images (PNG, JPG, etc.)
- This tool can only read files, not directories`
}

func (t *fileReadTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"file_path": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The absolute path to the file to read",
			}),
			"offset": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": "The line number to start reading from (1-indexed). Only provide if the file is too large to read at once.",
			}),
			"limit": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": "The number of lines to read. Only provide if the file is too large to read at once.",
			}),
		},
		[]string{"file_path"},
	)
}

func (t *fileReadTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *fileReadTool) IsReadOnly(_ tools.Input) bool         { return true }
func (t *fileReadTool) SearchHint() string                   { return "read file open view cat" }

func (t *fileReadTool) UserFacingName(input tools.Input) string {
	var in FileReadInput
	if json.Unmarshal(input, &in) == nil && in.FilePath != "" {
		return fmt.Sprintf("Read(%s)", in.FilePath)
	}
	return "Read"
}

// GetPath implements tools.PathTool.
func (t *fileReadTool) GetPath(input tools.Input) string {
	var in FileReadInput
	if json.Unmarshal(input, &in) == nil {
		return expandPath(in.FilePath)
	}
	return ""
}

func (t *fileReadTool) ValidateInput(input tools.Input, _ *tools.UseContext) (tools.ValidationResult, error) {
	var in FileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.ValidationResult{OK: false, Reason: "invalid JSON: " + err.Error()}, nil
	}
	if strings.TrimSpace(in.FilePath) == "" {
		return tools.ValidationResult{OK: false, Reason: "file_path is required"}, nil
	}
	if in.Offset != nil && *in.Offset < 1 {
		return tools.ValidationResult{OK: false, Reason: "offset must be ≥ 1"}, nil
	}
	if in.Limit != nil && *in.Limit < 1 {
		return tools.ValidationResult{OK: false, Reason: "limit must be ≥ 1"}, nil
	}
	return tools.ValidationResult{OK: true}, nil
}

// Call executes the FileRead tools.
func (t *fileReadTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in FileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}

	fullPath := expandPath(in.FilePath)

	// Block device files.
	if isBlockedDevicePath(fullPath) {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("cannot read device file: %s", fullPath)}, nil
	}

	// Verify path exists and is a regular file.
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("file does not exist: %s", fullPath)}, nil
	}
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("cannot stat file: %v", err)}, nil
	}
	if info.IsDir() {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("%s is a directory, not a file", fullPath)}, nil
	}

	ext := strings.ToLower(filepath.Ext(fullPath))

	// Route by file type.
	switch {
	case isImageExtension(ext):
		return t.readImage(fullPath)
	default:
		return t.readText(fullPath, in, ctx)
	}
}

// readText reads a plain text file, optionally within a line range.
func (t *fileReadTool) readText(fullPath string, in FileReadInput, ctx *tools.UseContext) (*tools.Result, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("cannot open file: %v", err)}, nil
	}
	defer f.Close()

	// Determine offset (0-indexed internally).
	offset := 0
	if in.Offset != nil {
		offset = *in.Offset - 1
	}

	limit := maxFileReadLines
	if in.Limit != nil && *in.Limit > 0 {
		limit = *in.Limit
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var lines []string
	totalLines := 0
	lineNum := 0

	for scanner.Scan() {
		if ctx != nil {
			select {
			case <-ctx.Ctx.Done():
				return &tools.Result{IsError: true, Content: "read cancelled"}, nil
			default:
			}
		}
		totalLines++
		if lineNum < offset {
			lineNum++
			continue
		}
		if len(lines) >= limit {
			// Keep counting total but don't store.
			continue
		}
		lines = append(lines, fmt.Sprintf("%6d\t%s", totalLines, scanner.Text()))
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("error reading file: %v", err)}, nil
	}

	out := FileReadOutput{
		Type:       "text",
		FilePath:   in.FilePath,
		Content:    strings.Join(lines, "\n"),
		NumLines:   len(lines),
		StartLine:  offset + 1,
		TotalLines: totalLines,
	}
	return &tools.Result{Content: out}, nil
}

// readImage reads an image file and returns it as a base64-encoded block.
func (t *fileReadTool) readImage(fullPath string) (*tools.Result, error) {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("cannot read image: %v", err)}, nil
	}

	ext := strings.ToLower(filepath.Ext(fullPath))
	mediaType := extToMIME(ext)

	out := FileReadOutput{
		Type:      "image",
		FilePath:  fullPath,
		Base64:    base64.StdEncoding.EncodeToString(data),
		MediaType: mediaType,
	}
	return &tools.Result{Content: out}, nil
}

// extToMIME maps image file extensions to MIME types.
func extToMIME(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	default:
		return "image/png"
	}
}

func (t *fileReadTool) MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error) {
	out, ok := output.(FileReadOutput)
	if !ok {
		return t.BaseTool.MapResultToToolResultBlock(output, toolUseID)
	}

	switch out.Type {
	case "image":
		// Return an image content block.
		block := map[string]any{
			"type":        "tool_result",
			"tool_use_id": toolUseID,
			"content": []map[string]any{
				{
					"type": "image",
					"source": map[string]any{
						"type":       "base64",
						"media_type": out.MediaType,
						"data":       out.Base64,
					},
				},
			},
		}
		return json.Marshal(block)
	default:
		block := map[string]any{
			"type":        "tool_result",
			"tool_use_id": toolUseID,
			"content":     out.Content,
		}
		return json.Marshal(block)
	}
}
