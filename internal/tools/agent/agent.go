package agent

import (
	"encoding/json"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
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

func (t *agentTool) Call(input tools.Input, ctx *tools.UseContext, onProgress tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{
			IsError: true,
			Content: "Agent tool is not available: coordinator mode is not enabled",
		}, nil
	}

	var in AgentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{
			IsError: true,
			Content: fmt.Sprintf("invalid input: %v", err),
		}, nil
	}
	if in.Prompt == "" {
		return &tools.Result{
			IsError: true,
			Content: "prompt is required",
		}, nil
	}

	maxTurns := 0
	if in.MaxTurns != nil {
		maxTurns = *in.MaxTurns
	}

	// Spawn the sub-agent via the coordinator.
	agentID, err := ctx.Coordinator.SpawnAgent(ctx.Ctx, tools.AgentSpawnRequest{
		Description:  truncateStr(in.Prompt, 100),
		Prompt:       in.Prompt,
		AllowedTools: in.AllowedTools,
		MaxTurns:     maxTurns,
	})
	if err != nil {
		return &tools.Result{
			IsError: true,
			Content: fmt.Sprintf("failed to spawn agent: %v", err),
		}, nil
	}

	// Report progress while waiting.
	if onProgress != nil {
		onProgress(map[string]string{
			"agent_id": agentID,
			"status":   "spawned",
		})
	}

	// Wait for the sub-agent to finish.
	result, err := ctx.Coordinator.WaitForAgent(ctx.Ctx, agentID)
	if err != nil {
		return &tools.Result{
			IsError: true,
			Content: fmt.Sprintf("agent %s failed: %v", agentID, err),
		}, nil
	}

	// Marshal the output.
	out := AgentOutput{Response: result}
	outBytes, _ := json.Marshal(out)

	return &tools.Result{
		Content: string(outBytes),
	}, nil
}

// truncateStr truncates s to maxLen runes, appending "…" if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
