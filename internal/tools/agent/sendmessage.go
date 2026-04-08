package agent

import (
	"encoding/json"
	"fmt"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// SendMessageInput is the input schema for the SendMessage tool.
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
var SendMessageTool tool.Tool = &sendMessageTool{}

type sendMessageTool struct{ tool.BaseTool }

func (t *sendMessageTool) Name() string { return "SendMessage" }

func (t *sendMessageTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Sends a message to a running sub-agent and returns its response.

Usage notes:
- The agent_id must refer to a currently active sub-agent session
- Returns the sub-agent's reply to the message`
}

func (t *sendMessageTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"agent_id": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The identifier of the target sub-agent",
			}),
			"content": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The message content to send to the sub-agent",
			}),
		},
		[]string{"agent_id", "content"},
	)
}

func (t *sendMessageTool) IsConcurrencySafe(_ tool.Input) bool { return false }
func (t *sendMessageTool) IsReadOnly(_ tool.Input) bool        { return false }

func (t *sendMessageTool) UserFacingName(input tool.Input) string {
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

func (t *sendMessageTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core session routing.
	return &tool.Result{
		IsError: true,
		Content: "SendMessage tool not yet implemented: requires Agent-Core session router (TODO(dep))",
	}, nil
}
