package agent

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/claude-code-go/internal/tools"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// AgentInput is the input schema for the Agent tools.
type AgentInput struct {
	// Prompt is the task or question for the sub-agent (required).
	Prompt string `json:"prompt"`
	// SystemPrompt is an optional override for the sub-agent system prompt.
	SystemPrompt string `json:"system_prompt,omitempty"`
	// AllowedTools restricts which tools the sub-agent may use (optional).
	AllowedTools []string `json:"allowed_tools,omitempty"`
	// MaxTurns limits the number of turns the sub-agent may take (optional).
	MaxTurns *int `json:"max_turns,omitempty"`
}

// AgentOutput is the structured output of the Agent tools.
type AgentOutput struct {
	// Response is the sub-agent's final response text.
	Response string `json:"response"`
}

// ── Tool implementation ───────────────────────────────────────────────────────

// AgentTool is the exported singleton instance.
// TODO(dep): Full implementation requires Agent-Core's orchestrator and
// session-management layer (SubAgentManager).
var AgentTool tools.Tool = &agentTool{}

type agentTool struct{ tools.BaseTool }

func (t *agentTool) Name() string { return "Agent" }

func (t *agentTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Launches a sub-agent to handle a complex sub-task autonomously.

Usage notes:
- Use this tool for tasks that require multiple steps or tool calls
- The sub-agent receives the same built-in tools as the parent agent
- Returns the sub-agent's final response`
}

func (t *agentTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"prompt": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The task or question to send to the sub-agent",
			}),
			"system_prompt": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional system prompt override for the sub-agent",
			}),
			"allowed_tools": tools.PropSchema(map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional list of tool names the sub-agent is allowed to use",
			}),
			"max_turns": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": "Optional maximum number of turns the sub-agent may take",
			}),
		},
		[]string{"prompt"},
	)
}

func (t *agentTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *agentTool) IsReadOnly(_ tools.Input) bool        { return false }

func (t *agentTool) UserFacingName(input tools.Input) string {
	var in AgentInput
	if json.Unmarshal(input, &in) == nil && in.Prompt != "" {
		prompt := in.Prompt
		if len(prompt) > 60 {
			prompt = prompt[:60] + "…"
		}
		return fmt.Sprintf("Agent(%s)", prompt)
	}
	return "Agent"
}

func (t *agentTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core SubAgentManager.
	return &tools.Result{
		IsError: true,
		Content: "Agent tool not yet implemented: requires Agent-Core orchestrator (TODO(dep))",
	}, nil
}
