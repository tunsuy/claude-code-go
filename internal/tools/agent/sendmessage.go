package agent

import (
	"encoding/json"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// SendMessageInput is the input schema for the SendMessage tools.
type SendMessageInput struct {
	// AgentID is the target sub-agent identifier (required).
	AgentID string `json:"agent_id"`
	// Content is the message content to send (required).
	Content string `json:"content"`
}

// SendMessageOutput is the structured output of SendMessage.
type SendMessageOutput struct {
	// Response is the sub-agent's reply.
	Response string `json:"response"`
}

// ── Tool implementation ───────────────────────────────────────────────────────

// SendMessageTool is the exported singleton instance.
// TODO(dep): Full implementation requires Agent-Core's session/message-routing layer.
var SendMessageTool tools.Tool = &sendMessageTool{}

type sendMessageTool struct{ tools.BaseTool }

func (t *sendMessageTool) Name() string { return "SendMessage" }

func (t *sendMessageTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Sends a message to a running sub-agent and returns its response.

Usage notes:
- The agent_id must refer to a currently active sub-agent session
- Returns the sub-agent's reply to the message`
}

func (t *sendMessageTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"agent_id": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The identifier of the target sub-agent",
			}),
			"content": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The message content to send to the sub-agent",
			}),
		},
		[]string{"agent_id", "content"},
	)
}

func (t *sendMessageTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *sendMessageTool) IsReadOnly(_ tools.Input) bool        { return false }

func (t *sendMessageTool) UserFacingName(input tools.Input) string {
	var in SendMessageInput
	if json.Unmarshal(input, &in) == nil && in.AgentID != "" {
		msg := in.Content
		if len(msg) > 40 {
			msg = msg[:40] + "…"
		}
		return fmt.Sprintf("SendMessage(→%s: %s)", in.AgentID, msg)
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
	if in.AgentID == "" {
		return &tools.Result{
			IsError: true,
			Content: "agent_id is required",
		}, nil
	}
	if in.Content == "" {
		return &tools.Result{
			IsError: true,
			Content: "content is required",
		}, nil
	}

	// Deliver the message via the coordinator.
	if err := ctx.Coordinator.SendMessage(ctx.Ctx, in.AgentID, in.Content); err != nil {
		return &tools.Result{
			IsError: true,
			Content: fmt.Sprintf("failed to send message to agent %s: %v", in.AgentID, err),
		}, nil
	}

	out := SendMessageOutput{
		Response: fmt.Sprintf("Message delivered to agent %s", in.AgentID),
	}
	outBytes, _ := json.Marshal(out)

	return &tools.Result{
		Content: string(outBytes),
	}, nil
}
