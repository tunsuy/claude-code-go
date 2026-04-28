package agent

import (
	"encoding/json"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// GetAgentStatusInput is the input schema for GetAgentStatus.
type GetAgentStatusInput struct {
	// AgentID is the identifier of the agent to query (required).
	AgentID string `json:"agent_id"`
}

// GetAgentStatusOutput is the structured output of GetAgentStatus.
type GetAgentStatusOutput struct {
	// AgentID is the queried agent's ID.
	AgentID string `json:"agent_id"`
	// Status is the current lifecycle state.
	Status string `json:"status"`
	// Result is the final output (only present when status is completed/failed).
	Result string `json:"result,omitempty"`
}

// ── Tool implementation ───────────────────────────────────────────────────────

// GetAgentStatusTool is the exported singleton instance.
var GetAgentStatusTool tools.Tool = &getAgentStatusTool{}

type getAgentStatusTool struct{ tools.BaseTool }

func (t *getAgentStatusTool) Name() string { return "GetAgentStatus" }

func (t *getAgentStatusTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Returns the current status and result of a sub-agent.

Use this to poll background agents spawned with background: true.
Status values: running, completed, failed, killed.`
}

func (t *getAgentStatusTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"agent_id": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The agent ID to check (returned by Agent tool in background mode)",
			}),
		},
		[]string{"agent_id"},
	)
}

func (t *getAgentStatusTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *getAgentStatusTool) IsReadOnly(_ tools.Input) bool        { return true }

func (t *getAgentStatusTool) UserFacingName(input tools.Input) string {
	var in GetAgentStatusInput
	if json.Unmarshal(input, &in) == nil && in.AgentID != "" {
		return fmt.Sprintf("GetAgentStatus(%s)", in.AgentID)
	}
	return "GetAgentStatus"
}

func (t *getAgentStatusTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if ctx == nil || ctx.Coordinator == nil {
		return &tools.Result{
			IsError: true,
			Content: "GetAgentStatus is not available: coordinator mode is not enabled",
		}, nil
	}

	var in GetAgentStatusInput
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

	result, status, err := ctx.Coordinator.GetAgentResult(ctx.Ctx, in.AgentID)
	if err != nil {
		return &tools.Result{
			IsError: true,
			Content: fmt.Sprintf("agent not found: %v", err),
		}, nil
	}

	out := GetAgentStatusOutput{
		AgentID: in.AgentID,
		Status:  status,
	}
	// Only include result when the agent has finished.
	if status == "completed" || status == "failed" {
		out.Result = result
	}

	outBytes, _ := json.Marshal(out)
	return &tools.Result{Content: string(outBytes)}, nil
}
