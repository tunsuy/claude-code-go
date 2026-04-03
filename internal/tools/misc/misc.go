package misc

import (
	"encoding/json"
	"fmt"

	tool "github.com/anthropics/claude-code-go/internal/tool"
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
var SkillTool tool.Tool = &skillTool{}

type skillTool struct{ tool.BaseTool }

func (t *skillTool) Name() string { return "Skill" }

func (t *skillTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Invokes a named skill (slash command) within the current session.`
}

func (t *skillTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"skill": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The skill name or slash-command path to invoke",
			}),
			"args": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional arguments to pass to the skill",
			}),
		},
		[]string{"skill"},
	)
}

func (t *skillTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *skillTool) IsReadOnly(_ tool.Input) bool        { return false }

func (t *skillTool) UserFacingName(input tool.Input) string {
	var in SkillInput
	if json.Unmarshal(input, &in) == nil && in.Skill != "" {
		return fmt.Sprintf("Skill(%s)", in.Skill)
	}
	return "Skill"
}

func (t *skillTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core skill registry.
	return &tool.Result{IsError: true, Content: "Skill tool not yet implemented (TODO(dep))"}, nil
}

// ── BriefTool ─────────────────────────────────────────────────────────────────

// BriefInput is the input schema for BriefTool.
type BriefInput struct {
	// Content is the text to be briefed/summarised.
	Content string `json:"content"`
}

// BriefTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core summarisation pipeline.
var BriefTool tool.Tool = &briefTool{}

type briefTool struct{ tool.BaseTool }

func (t *briefTool) Name() string { return "Brief" }

func (t *briefTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Generates a concise summary (brief) of the provided content.`
}

func (t *briefTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"content": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The content to summarise",
			}),
		},
		[]string{"content"},
	)
}

func (t *briefTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (t *briefTool) IsReadOnly(_ tool.Input) bool        { return true }
func (t *briefTool) UserFacingName(_ tool.Input) string  { return "Brief" }

func (t *briefTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core summarisation pipeline.
	return &tool.Result{IsError: true, Content: "Brief tool not yet implemented (TODO(dep))"}, nil
}

// ── ToolSearch ────────────────────────────────────────────────────────────────

// ToolSearchInput is the input schema for ToolSearch.
type ToolSearchInput struct {
	// Query is the natural-language search query (required).
	Query string `json:"query"`
}

// ToolSearchTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core tool registry and search index.
var ToolSearchTool tool.Tool = &toolSearchTool{}

type toolSearchTool struct{ tool.BaseTool }

func (t *toolSearchTool) Name() string { return "ToolSearch" }

func (t *toolSearchTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Searches the tool registry for tools matching a natural-language query.`
}

func (t *toolSearchTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"query": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "Natural-language description of the desired tool",
			}),
		},
		[]string{"query"},
	)
}

func (t *toolSearchTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (t *toolSearchTool) IsReadOnly(_ tool.Input) bool        { return true }

func (t *toolSearchTool) UserFacingName(input tool.Input) string {
	var in ToolSearchInput
	if json.Unmarshal(input, &in) == nil && in.Query != "" {
		return fmt.Sprintf("ToolSearch(%s)", in.Query)
	}
	return "ToolSearch"
}

func (t *toolSearchTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core tool registry search index.
	return &tool.Result{IsError: true, Content: "ToolSearch not yet implemented (TODO(dep))"}, nil
}

// ── SleepTool ─────────────────────────────────────────────────────────────────

// SleepInput is the input schema for SleepTool.
type SleepInput struct {
	// Milliseconds is the duration to sleep in milliseconds (required).
	Milliseconds int `json:"milliseconds"`
}

// SleepTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core task scheduler integration.
var SleepTool tool.Tool = &sleepTool{}

type sleepTool struct{ tool.BaseTool }

func (t *sleepTool) Name() string { return "Sleep" }

func (t *sleepTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Pauses execution for the specified number of milliseconds.`
}

func (t *sleepTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"milliseconds": tool.PropSchema(map[string]any{
				"type":        "integer",
				"description": "Number of milliseconds to sleep",
				"minimum":     0,
			}),
		},
		[]string{"milliseconds"},
	)
}

func (t *sleepTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (t *sleepTool) IsReadOnly(_ tool.Input) bool        { return true }

func (t *sleepTool) UserFacingName(input tool.Input) string {
	var in SleepInput
	if json.Unmarshal(input, &in) == nil {
		return fmt.Sprintf("Sleep(%dms)", in.Milliseconds)
	}
	return "Sleep"
}

func (t *sleepTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement with context-aware sleep.
	return &tool.Result{IsError: true, Content: "Sleep tool not yet implemented (TODO(dep))"}, nil
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
var SyntheticOutputTool tool.Tool = &syntheticOutputTool{}

type syntheticOutputTool struct{ tool.BaseTool }

func (t *syntheticOutputTool) Name() string { return "SyntheticOutput" }

func (t *syntheticOutputTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Injects a synthetic tool result into the conversation without executing a real tool call.`
}

func (t *syntheticOutputTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"content": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The content to inject as a tool result",
			}),
			"is_error": tool.PropSchema(map[string]any{
				"type":        "boolean",
				"description": "If true, marks the synthetic result as an error",
			}),
		},
		[]string{"content"},
	)
}

func (t *syntheticOutputTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *syntheticOutputTool) IsReadOnly(_ tool.Input) bool        { return true }
func (t *syntheticOutputTool) UserFacingName(_ tool.Input) string  { return "SyntheticOutput" }

func (t *syntheticOutputTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core conversation manipulation layer.
	return &tool.Result{IsError: true, Content: "SyntheticOutput not yet implemented (TODO(dep))"}, nil
}
