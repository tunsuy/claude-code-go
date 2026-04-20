package misc

import (
	"encoding/json"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── SkillTool ─────────────────────────────────────────────────────────────────

// SkillInput is the input schema for SkillTool.
type SkillInput struct {
	// Skill is the skill name or slash-command path (required).
	Skill string `json:"skill"`
	// Args is optional arguments to pass to the skill.
	Args string `json:"args,omitempty"`
}

// SkillTool_ is the exported singleton instance.
// (Trailing underscore avoids collision with the 'tool' package import alias.)
// TODO(dep): Requires Agent-Core skill registry and loader.
var SkillTool tools.Tool = &skillTool{}

type skillTool struct{ tools.BaseTool }

func (t *skillTool) Name() string { return "Skill" }

func (t *skillTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Invokes a named skill (slash command) within the current session.`
}

func (t *skillTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"skill": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The skill name or slash-command path to invoke",
			}),
			"args": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional arguments to pass to the skill",
			}),
		},
		[]string{"skill"},
	)
}

func (t *skillTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *skillTool) IsReadOnly(_ tools.Input) bool        { return false }

func (t *skillTool) UserFacingName(input tools.Input) string {
	var in SkillInput
	if json.Unmarshal(input, &in) == nil && in.Skill != "" {
		return fmt.Sprintf("Skill(%s)", in.Skill)
	}
	return "Skill"
}

func (t *skillTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core skill registry.
	return &tools.Result{IsError: true, Content: "Skill tool not yet implemented (TODO(dep))"}, nil
}

// ── BriefTool ─────────────────────────────────────────────────────────────────

// BriefInput is the input schema for BriefTool.
type BriefInput struct {
	// Content is the text to be briefed/summarised.
	Content string `json:"content"`
}

// BriefTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core summarisation pipeline.
var BriefTool tools.Tool = &briefTool{}

type briefTool struct{ tools.BaseTool }

func (t *briefTool) Name() string { return "Brief" }

func (t *briefTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Generates a concise summary (brief) of the provided content.`
}

func (t *briefTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"content": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The content to summarise",
			}),
		},
		[]string{"content"},
	)
}

func (t *briefTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *briefTool) IsReadOnly(_ tools.Input) bool        { return true }
func (t *briefTool) UserFacingName(_ tools.Input) string  { return "Brief" }

func (t *briefTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core summarisation pipeline.
	return &tools.Result{IsError: true, Content: "Brief tool not yet implemented (TODO(dep))"}, nil
}

// ── ToolSearch ────────────────────────────────────────────────────────────────

// ToolSearchInput is the input schema for ToolSearch.
type ToolSearchInput struct {
	// Query is the natural-language search query (required).
	Query string `json:"query"`
}

// ToolSearchTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core tool registry and search index.
var ToolSearchTool tools.Tool = &toolSearchTool{}

type toolSearchTool struct{ tools.BaseTool }

func (t *toolSearchTool) Name() string { return "ToolSearch" }

func (t *toolSearchTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Searches the tool registry for tools matching a natural-language query.`
}

func (t *toolSearchTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"query": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Natural-language description of the desired tool",
			}),
		},
		[]string{"query"},
	)
}

func (t *toolSearchTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *toolSearchTool) IsReadOnly(_ tools.Input) bool        { return true }

func (t *toolSearchTool) UserFacingName(input tools.Input) string {
	var in ToolSearchInput
	if json.Unmarshal(input, &in) == nil && in.Query != "" {
		return fmt.Sprintf("ToolSearch(%s)", in.Query)
	}
	return "ToolSearch"
}

func (t *toolSearchTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core tool registry search index.
	return &tools.Result{IsError: true, Content: "ToolSearch not yet implemented (TODO(dep))"}, nil
}

// ── SleepTool ─────────────────────────────────────────────────────────────────

// SleepInput is the input schema for SleepTool.
type SleepInput struct {
	// Milliseconds is the duration to sleep in milliseconds (required).
	Milliseconds int `json:"milliseconds"`
}

// SleepTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core task scheduler integration.
var SleepTool tools.Tool = &sleepTool{}

type sleepTool struct{ tools.BaseTool }

func (t *sleepTool) Name() string { return "Sleep" }

func (t *sleepTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Pauses execution for the specified number of milliseconds.`
}

func (t *sleepTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"milliseconds": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": "Number of milliseconds to sleep",
				"minimum":     0,
			}),
		},
		[]string{"milliseconds"},
	)
}

func (t *sleepTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *sleepTool) IsReadOnly(_ tools.Input) bool        { return true }

func (t *sleepTool) UserFacingName(input tools.Input) string {
	var in SleepInput
	if json.Unmarshal(input, &in) == nil {
		return fmt.Sprintf("Sleep(%dms)", in.Milliseconds)
	}
	return "Sleep"
}

func (t *sleepTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement with context-aware sleep.
	return &tools.Result{IsError: true, Content: "Sleep tool not yet implemented (TODO(dep))"}, nil
}

// ── SyntheticOutput ───────────────────────────────────────────────────────────

// SyntheticOutputInput is the input schema for SyntheticOutput.
type SyntheticOutputInput struct {
	// Content is the synthetic content to inject as a tool result.
	Content string `json:"content"`
	// IsError marks the output as an error result.
	IsError bool `json:"is_error,omitempty"`
}

// SyntheticOutputTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core turn / conversation manipulation layer.
var SyntheticOutputTool tools.Tool = &syntheticOutputTool{}

type syntheticOutputTool struct{ tools.BaseTool }

func (t *syntheticOutputTool) Name() string { return "SyntheticOutput" }

func (t *syntheticOutputTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Injects a synthetic tool result into the conversation without executing a real tool call.`
}

func (t *syntheticOutputTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"content": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The content to inject as a tool result",
			}),
			"is_error": tools.PropSchema(map[string]any{
				"type":        "boolean",
				"description": "If true, marks the synthetic result as an error",
			}),
		},
		[]string{"content"},
	)
}

func (t *syntheticOutputTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *syntheticOutputTool) IsReadOnly(_ tools.Input) bool        { return true }
func (t *syntheticOutputTool) UserFacingName(_ tools.Input) string  { return "SyntheticOutput" }

func (t *syntheticOutputTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core conversation manipulation layer.
	return &tools.Result{IsError: true, Content: "SyntheticOutput not yet implemented (TODO(dep))"}, nil
}
