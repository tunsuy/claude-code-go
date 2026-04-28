package agent

import (
	"encoding/json"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// SendMessageInput is the input schema for the SendMessage tools.
type SendMessageInput struct {
	// To is the target: agent name, agent ID, or "*" for broadcast (preferred).
	To string `json:"to,omitempty"`
	// AgentID is the target sub-agent identifier (deprecated, use "to" instead).
	AgentID string `json:"agent_id,omitempty"`
	// Content is the message content to send (required).
	Content string `json:"content"`
}

// SendMessageOutput is the structured output of SendMessage.
type SendMessageOutput struct {
	// Response is confirmation of delivery.
	Response string `json:"response"`
}

// ── Tool implementation ───────────────────────────────────────────────────────

// SendMessageTool is the exported singleton instance.
var SendMessageTool tools.Tool = &sendMessageTool{}

type sendMessageTool struct{ tools.BaseTool }

func (t *sendMessageTool) Name() string { return "SendMessage" }

func (t *sendMessageTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Sends a message to a running sub-agent and returns a delivery confirmation.

The "to" field supports:
- Agent name (e.g. "researcher") — routes by human-readable name
- Agent ID (UUID) — routes by exact ID
- "*" — broadcasts to ALL running agents

Usage notes:
- The target must refer to a currently active sub-agent
- Use agent names for readability when you set agent_name in Agent tool`
}

func (t *sendMessageTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"to": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Target agent: name, ID, or \"*\" for broadcast",
			}),
			"agent_id": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Deprecated: use 'to' instead. The identifier of the target sub-agent.",
			}),
			"content": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The message content to send to the sub-agent",
			}),
		},
		[]string{"content"},
	)
}

func (t *sendMessageTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *sendMessageTool) IsReadOnly(_ tools.Input) bool        { return false }

func (t *sendMessageTool) UserFacingName(input tools.Input) string {
	var in SendMessageInput
	if json.Unmarshal(input, &in) == nil {
		target := in.To
		if target == "" {
			target = in.AgentID
		}
		msg := in.Content
		if len(msg) > 40 {
			msg = msg[:40] + "…"
		}
		if target != "" {
			return fmt.Sprintf("SendMessage(→%s: %s)", target, msg)
		}
	}
	return "SendMessage"
}

func (t *sendMessageTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{
			IsError: true,
			Content: "SendMessage tool is not available: coordinator mode is not enabled",
		}, nil
	}

	var in SendMessageInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{
			IsError: true,
			Content: fmt.Sprintf("invalid input: %v", err),
		}, nil
	}

	// Resolve target: prefer "to", fallback to "agent_id".
	target := in.To
	if target == "" {
		target = in.AgentID
	}
	if target == "" {
		return &tools.Result{
			IsError: true,
			Content: "either 'to' or 'agent_id' is required",
		}, nil
	}
	if in.Content == "" {
		return &tools.Result{
			IsError: true,
			Content: "content is required",
		}, nil
	}

	// Handle broadcast.
	if target == "*" {
		sent, err := ctx.Coordinator.BroadcastMessage(ctx.Ctx, in.Content)
		if err != nil {
			return &tools.Result{
				IsError: true,
				Content: fmt.Sprintf("broadcast failed: %v", err),
			}, nil
		}
		out := SendMessageOutput{
			Response: fmt.Sprintf("Message broadcast to %d running agent(s)", sent),
		}
		outBytes, _ := json.Marshal(out)
		return &tools.Result{Content: string(outBytes)}, nil
	}

	// Resolve name → ID.
	resolvedID, err := ctx.Coordinator.ResolveAgent(ctx.Ctx, target)
	if err != nil {
		return &tools.Result{
			IsError: true,
			Content: fmt.Sprintf("failed to resolve agent %q: %v", target, err),
		}, nil
	}

	// Deliver the message via the coordinator.
	if err := ctx.Coordinator.SendMessage(ctx.Ctx, resolvedID, in.Content); err != nil {
		return &tools.Result{
			IsError: true,
			Content: fmt.Sprintf("failed to send message to agent %s: %v", target, err),
		}, nil
	}

	out := SendMessageOutput{
		Response: fmt.Sprintf("Message delivered to agent %s", target),
	}
	outBytes, _ := json.Marshal(out)

	return &tools.Result{
		Content: string(outBytes),
	}, nil
}
