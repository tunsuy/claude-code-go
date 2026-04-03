package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropics/claude-code-go/internal/tool"
)

// toolNameRe matches characters NOT allowed in Anthropic tool names.
var toolNameRe = regexp.MustCompile(`[^a-zA-Z0-9_\-]`)

// NormalizeToolName maps an MCP tool name to an Anthropic API-safe name.
// Only [a-zA-Z0-9_-] characters are kept; all others are replaced with "_".
func NormalizeToolName(name string) string {
	return toolNameRe.ReplaceAllString(name, "_")
}

// AdaptToTool adapts an MCPToolDef to the internal tool.Tool interface.
//
// Naming convention: "{serverName}__{normalizedToolName}" (double underscore),
// matching TS buildMcpToolName().
func AdaptToTool(serverName string, def MCPToolDef, client MCPClient) tool.Tool {
	normalized := NormalizeToolName(def.Name)
	fullName := serverName + "__" + normalized
	return &mcpTool{
		fullName:   fullName,
		serverName: serverName,
		rawName:    def.Name,
		desc:       def.Description,
		schema:     def.InputSchema,
		client:     client,
	}
}

// mcpTool implements tool.Tool for an MCP-backed tool.
type mcpTool struct {
	tool.BaseTool
	fullName   string
	serverName string
	rawName    string
	desc       string
	schema     json.RawMessage
	client     MCPClient
}

// ── Identity & Metadata ──────────────────────────────────────────────────────

func (t *mcpTool) Name() string { return t.fullName }
func (t *mcpTool) Aliases() []string { return nil }
func (t *mcpTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return t.desc
}
func (t *mcpTool) InputSchema() tool.InputSchema {
	// Deserialise the MCP JSON Schema into tool.InputSchema
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(t.schema, &s); err != nil {
		return tool.InputSchema{Type: "object"}
	}
	return tool.InputSchema{
		Type:       "object",
		Properties: s.Properties,
		Required:   s.Required,
	}
}
func (t *mcpTool) Prompt(_ context.Context, _ tool.PermissionContext) (string, error) {
	return "", nil
}
func (t *mcpTool) MaxResultSizeChars() int { return -1 }
func (t *mcpTool) SearchHint() string      { return t.serverName + " " + t.rawName }

// ── Concurrency & Safety ─────────────────────────────────────────────────────

func (t *mcpTool) IsConcurrencySafe(_ tool.Input) bool  { return false }
func (t *mcpTool) IsReadOnly(_ tool.Input) bool         { return false }
func (t *mcpTool) IsDestructive(_ tool.Input) bool      { return false }
func (t *mcpTool) IsEnabled() bool                      { return true }
func (t *mcpTool) InterruptBehavior() tool.InterruptBehavior {
	return tool.InterruptBehaviorCancel
}

// ── Permissions ──────────────────────────────────────────────────────────────

func (t *mcpTool) ValidateInput(_ tool.Input, _ *tool.UseContext) (tool.ValidationResult, error) {
	return tool.ValidationResult{OK: true}, nil
}
func (t *mcpTool) CheckPermissions(_ tool.Input, _ *tool.UseContext) (tool.PermissionResult, error) {
	return tool.PermissionResult{Behavior: tool.PermissionAsk}, nil
}
func (t *mcpTool) PreparePermissionMatcher(_ tool.Input) (func(string) bool, error) {
	return nil, nil
}

// ── Execution ────────────────────────────────────────────────────────────────

func (t *mcpTool) Call(input tool.Input, ctx *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	var args map[string]any
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return &tool.Result{
				Content: fmt.Sprintf("invalid input: %v", err),
				IsError: true,
			}, nil
		}
	}

	result, err := t.client.CallTool(ctx.Ctx, t.rawName, args)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("mcp tool error: %v", err),
			IsError: true,
		}, nil
	}

	// Consolidate content blocks into a single string
	var parts []string
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			parts = append(parts, c.Text)
		case "image":
			parts = append(parts, fmt.Sprintf("[image: %s]", c.MIMEType))
		default:
			parts = append(parts, fmt.Sprintf("[%s content]", c.Type))
		}
	}
	content := strings.Join(parts, "\n")
	return &tool.Result{
		Content: content,
		IsError: result.IsError,
	}, nil
}

// ── Serialization ────────────────────────────────────────────────────────────

func (t *mcpTool) MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error) {
	var contentStr string
	switch v := output.(type) {
	case string:
		contentStr = v
	default:
		raw, err := json.Marshal(output)
		if err != nil {
			return nil, fmt.Errorf("mcpTool: marshal: %w", err)
		}
		contentStr = string(raw)
	}
	block := map[string]any{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     contentStr,
	}
	return json.Marshal(block)
}

func (t *mcpTool) ToAutoClassifierInput(_ tool.Input) string { return "" }

func (t *mcpTool) UserFacingName(_ tool.Input) string {
	return fmt.Sprintf("MCP(%s::%s)", t.serverName, t.rawName)
}

// MCPInfo implements tool.MCPToolInfo.
func (t *mcpTool) MCPInfo() tool.MCPInfo {
	return tool.MCPInfo{
		ServerName: t.serverName,
		ToolName:   t.rawName,
	}
}
