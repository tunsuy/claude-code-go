package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/memdir"
	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── MemoryReadTool ───────────────────────────────────────────────────────────

// MemoryReadInput is the input schema for MemoryReadTool.
type MemoryReadInput struct {
	// Name is the memory name to read (without .md extension). Optional.
	// If empty, lists all memories.
	Name string `json:"name,omitempty"`
}

// MemoryReadTool is the exported singleton instance for reading memories.
var MemoryReadTool tools.Tool = &memoryReadTool{}

type memoryReadTool struct{ tools.BaseTool }

// Name returns the canonical tool name.
func (t *memoryReadTool) Name() string { return "MemoryRead" }

// Description returns the tool description for the system prompt.
func (t *memoryReadTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Read project memories. Without a name, lists all available memories with titles and types.
With a name, reads the full content of a specific memory file.

Memories are persistent project knowledge that survives across sessions — things like
user preferences, coding conventions, architecture decisions, and feedback corrections.`
}

// InputSchema returns the JSON Schema for this tool's input.
func (t *memoryReadTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"name": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The memory name to read (without .md extension). If omitted, lists all memories.",
			}),
		},
		nil, // no required fields
	)
}

// IsConcurrencySafe returns true because reading is safe for concurrency.
func (t *memoryReadTool) IsConcurrencySafe(_ tools.Input) bool { return true }

// IsReadOnly returns true because this tool does not modify state.
func (t *memoryReadTool) IsReadOnly(_ tools.Input) bool { return true }

// UserFacingName returns a human-readable label for the tool call.
func (t *memoryReadTool) UserFacingName(input tools.Input) string {
	var in MemoryReadInput
	if json.Unmarshal(input, &in) == nil && in.Name != "" {
		return fmt.Sprintf("MemoryRead(%s)", in.Name)
	}
	return "MemoryRead(list)"
}

// Call executes the memory read operation.
func (t *memoryReadTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in MemoryReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	store := getStore(ctx)
	if store == nil {
		return &tools.Result{IsError: true, Content: "memory store not available (no project directory)"}, nil
	}

	if in.Name == "" {
		return listMemories(store)
	}
	return readMemory(store, in.Name)
}

func listMemories(store *memdir.MemoryStore) (*tools.Result, error) {
	memories, err := store.ListMemories()
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("list memories: %v", err)}, nil
	}

	if len(memories) == 0 {
		return &tools.Result{Content: "No memories found. Use MemoryWrite to create one."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d memories:\n\n", len(memories)))
	for _, mf := range memories {
		title := mf.Header.Title
		filename := mf.Path[strings.LastIndex(mf.Path, "/")+1:]
		slug := strings.TrimSuffix(filename, ".md")
		if title == "" {
			title = slug
		}
		// Show slug name so the LLM knows the exact name for MemoryRead/MemoryDelete.
		sb.WriteString(fmt.Sprintf("- [%s](%s) [%s] (%s)\n",
			title, filename, mf.Header.Type, mf.Header.UpdatedAt.Format("2006-01-02")))
		summary := firstLine(mf.Body)
		if summary != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", summary))
		}
	}
	return &tools.Result{Content: sb.String()}, nil
}

func readMemory(store *memdir.MemoryStore, name string) (*tools.Result, error) {
	mf, err := store.ReadMemory(name)
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("read memory %q: %v", name, err)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", mf.Header.Title))
	sb.WriteString(fmt.Sprintf("Type: %s | Created: %s | Updated: %s\n",
		mf.Header.Type,
		mf.Header.CreatedAt.Format("2006-01-02"),
		mf.Header.UpdatedAt.Format("2006-01-02")))
	if len(mf.Header.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(mf.Header.Tags, ", ")))
	}
	sb.WriteString("\n---\n\n")
	sb.WriteString(mf.Body)
	return &tools.Result{Content: sb.String()}, nil
}

// firstLine returns the first non-empty line, truncated to 120 chars.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 120 {
				return line[:117] + "..."
			}
			return line
		}
	}
	return ""
}

// ── MemoryWriteTool ──────────────────────────────────────────────────────────

// MemoryWriteInput is the input schema for MemoryWriteTool.
type MemoryWriteInput struct {
	// Title is the memory title (required).
	Title string `json:"title"`
	// Content is the memory content in Markdown (required).
	Content string `json:"content"`
	// Type classifies the memory (user/feedback/project/reference). Default: project.
	Type string `json:"type,omitempty"`
	// Tags are optional labels for categorisation.
	Tags []string `json:"tags,omitempty"`
}

// MemoryWriteTool is the exported singleton instance for writing memories.
var MemoryWriteTool tools.Tool = &memoryWriteTool{}

type memoryWriteTool struct{ tools.BaseTool }

// Name returns the canonical tool name.
func (t *memoryWriteTool) Name() string { return "MemoryWrite" }

// Description returns the tool description for the system prompt.
func (t *memoryWriteTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Write a persistent memory about the project or user preferences.

Memories survive across sessions and help you provide better, personalised assistance.
Use this tool to remember:
- User preferences (coding style, tool choices, naming conventions)
- Project architecture decisions and patterns
- Feedback and corrections from the user
- Important external references

Memory types: "user" (preferences), "feedback" (corrections), "project" (knowledge), "reference" (external).`
}

// InputSchema returns the JSON Schema for this tool's input.
func (t *memoryWriteTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"title": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "A short, descriptive title for this memory",
			}),
			"content": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The memory content in Markdown format",
			}),
			"type": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Memory type: user, feedback, project, or reference",
				"enum":        []string{"user", "feedback", "project", "reference"},
				"default":     "project",
			}),
			"tags": tools.PropSchema(map[string]any{
				"type":        "array",
				"description": "Optional tags for categorisation",
				"items":       map[string]any{"type": "string"},
			}),
		},
		[]string{"title", "content"},
	)
}

// IsConcurrencySafe returns false because writing modifies the filesystem.
func (t *memoryWriteTool) IsConcurrencySafe(_ tools.Input) bool { return false }

// IsReadOnly returns false because this tool modifies state.
func (t *memoryWriteTool) IsReadOnly(_ tools.Input) bool { return false }

// ValidateInput checks required fields.
func (t *memoryWriteTool) ValidateInput(input tools.Input, _ *tools.UseContext) (tools.ValidationResult, error) {
	var in MemoryWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.ValidationResult{OK: false, Reason: fmt.Sprintf("invalid input JSON: %v", err)}, nil
	}
	if in.Title == "" {
		return tools.ValidationResult{OK: false, Reason: "title is required"}, nil
	}
	if in.Content == "" {
		return tools.ValidationResult{OK: false, Reason: "content is required"}, nil
	}
	return tools.ValidationResult{OK: true}, nil
}

// UserFacingName returns a human-readable label for the tool call.
func (t *memoryWriteTool) UserFacingName(input tools.Input) string {
	var in MemoryWriteInput
	if json.Unmarshal(input, &in) == nil && in.Title != "" {
		return fmt.Sprintf("MemoryWrite(%s)", in.Title)
	}
	return "MemoryWrite"
}

// Call executes the memory write operation.
func (t *memoryWriteTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in MemoryWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	store := getStore(ctx)
	if store == nil {
		return &tools.Result{IsError: true, Content: "memory store not available (no project directory)"}, nil
	}

	memType := memdir.MemoryType(in.Type)
	if !memdir.ValidMemoryTypes[memType] {
		memType = memdir.MemoryTypeProject
	}

	mf := &memdir.MemoryFile{
		Header: memdir.MemoryHeader{
			Title:  in.Title,
			Type:   memType,
			Tags:   in.Tags,
			Source: "assistant",
		},
		Body: in.Content,
	}

	if err := store.WriteMemory(mf); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("write memory: %v", err)}, nil
	}

	// Rebuild the index after writing.
	if err := store.BuildIndex(); err != nil {
		// Non-fatal: the memory was written, index rebuild failed.
		return &tools.Result{Content: fmt.Sprintf("Memory %q saved (warning: index rebuild failed: %v)", in.Title, err)}, nil
	}

	return &tools.Result{Content: fmt.Sprintf("Memory %q saved successfully as type %q.", in.Title, memType)}, nil
}

// ── MemoryDeleteTool ─────────────────────────────────────────────────────────

// MemoryDeleteInput is the input schema for MemoryDeleteTool.
type MemoryDeleteInput struct {
	// Name is the memory name to delete (without .md extension, required).
	Name string `json:"name"`
}

// MemoryDeleteTool is the exported singleton instance for deleting memories.
var MemoryDeleteTool tools.Tool = &memoryDeleteTool{}

type memoryDeleteTool struct{ tools.BaseTool }

// Name returns the canonical tool name.
func (t *memoryDeleteTool) Name() string { return "MemoryDelete" }

// Description returns the tool description for the system prompt.
func (t *memoryDeleteTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Delete a persistent memory by name. Use MemoryRead (without arguments) to list available memories first.`
}

// InputSchema returns the JSON Schema for this tool's input.
func (t *memoryDeleteTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"name": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The memory name to delete (without .md extension)",
			}),
		},
		[]string{"name"},
	)
}

// IsConcurrencySafe returns false because deletion modifies the filesystem.
func (t *memoryDeleteTool) IsConcurrencySafe(_ tools.Input) bool { return false }

// IsReadOnly returns false because this tool modifies state.
func (t *memoryDeleteTool) IsReadOnly(_ tools.Input) bool { return false }

// IsDestructive returns true because deletion is hard to reverse.
func (t *memoryDeleteTool) IsDestructive(_ tools.Input) bool { return true }

// ValidateInput checks required fields.
func (t *memoryDeleteTool) ValidateInput(input tools.Input, _ *tools.UseContext) (tools.ValidationResult, error) {
	var in MemoryDeleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.ValidationResult{OK: false, Reason: fmt.Sprintf("invalid input JSON: %v", err)}, nil
	}
	if in.Name == "" {
		return tools.ValidationResult{OK: false, Reason: "name is required"}, nil
	}
	return tools.ValidationResult{OK: true}, nil
}

// UserFacingName returns a human-readable label for the tool call.
func (t *memoryDeleteTool) UserFacingName(input tools.Input) string {
	var in MemoryDeleteInput
	if json.Unmarshal(input, &in) == nil && in.Name != "" {
		return fmt.Sprintf("MemoryDelete(%s)", in.Name)
	}
	return "MemoryDelete"
}

// Call executes the memory delete operation.
func (t *memoryDeleteTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in MemoryDeleteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	store := getStore(ctx)
	if store == nil {
		return &tools.Result{IsError: true, Content: "memory store not available (no project directory)"}, nil
	}

	if err := store.DeleteMemory(in.Name); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("delete memory: %v", err)}, nil
	}

	// Rebuild the index after deletion.
	_ = store.BuildIndex()

	return &tools.Result{Content: fmt.Sprintf("Memory %q deleted successfully.", in.Name)}, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// getStore extracts the MemoryStore from the UseContext.
// Returns nil if no store is available.
func getStore(_ *tools.UseContext) *memdir.MemoryStore {
	// Get the working directory from os, then create a MemoryStore.
	// In a real implementation, the MemoryStore would be injected via
	// the UseContext or a dependency container.
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}
	store, err := memdir.NewMemoryStore(wd)
	if err != nil {
		return nil
	}
	return store
}
