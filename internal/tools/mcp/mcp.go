package mcp

import (
	"encoding/json"
	"fmt"

	tool "github.com/anthropics/claude-code-go/internal/tool"
)

// ── MCPTool (proxy) ───────────────────────────────────────────────────────────

// MCPToolInput is the input schema for an MCP proxy tool call.
// The actual parameters depend on the upstream MCP server's tool schema.
type MCPToolInput struct {
	// Params contains the raw parameters to forward to the MCP server.
	Params json.RawMessage `json:"params,omitempty"`
}

// MCPProxyTool is a runtime-instantiated proxy for a single MCP server tool.
// TODO(dep): Requires Agent-Core MCP client and server registry.
type MCPProxyTool struct {
	tool.BaseTool
	// name is the canonical tool name (e.g. "mcp__server__toolName").
	name string
	// serverName is the human-readable MCP server name.
	serverName string
	// description is the upstream tool description.
	description string
	// schema is the upstream tool's input schema.
	schema tool.InputSchema
}

// NewMCPProxyTool constructs a proxy tool for a single MCP server tool.
func NewMCPProxyTool(name, serverName, description string, schema tool.InputSchema) *MCPProxyTool {
	return &MCPProxyTool{
		name:        name,
		serverName:  serverName,
		description: description,
		schema:      schema,
	}
}

func (t *MCPProxyTool) Name() string { return t.name }

func (t *MCPProxyTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return t.description
}

func (t *MCPProxyTool) InputSchema() tool.InputSchema { return t.schema }

func (t *MCPProxyTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (t *MCPProxyTool) IsReadOnly(_ tool.Input) bool        { return false }

func (t *MCPProxyTool) UserFacingName(_ tool.Input) string {
	return fmt.Sprintf("mcp__%s", t.serverName)
}

// MCPInfo implements tool.MCPToolInfo.
func (t *MCPProxyTool) MCPInfo() tool.MCPInfo {
	return tool.MCPInfo{ServerName: t.serverName}
}

func (t *MCPProxyTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Forward call to the MCP server via Agent-Core MCP client.
	return &tool.Result{
		IsError: true,
		Content: fmt.Sprintf("MCPProxyTool(%s): MCP client not yet implemented (TODO(dep))", t.name),
	}, nil
}

// ── ListMcpResources ──────────────────────────────────────────────────────────

// ListMcpResourcesInput is the input schema for ListMcpResources.
type ListMcpResourcesInput struct {
	// ServerName filters to a specific MCP server (optional).
	ServerName string `json:"server_name,omitempty"`
}

// ListMcpResourcesTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core MCP client.
var ListMcpResourcesTool tool.Tool = &listMcpResourcesTool{}

type listMcpResourcesTool struct{ tool.BaseTool }

func (t *listMcpResourcesTool) Name() string { return "ListMcpResources" }

func (t *listMcpResourcesTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Lists all available resources from connected MCP servers.

Usage notes:
- Optionally filter by server_name
- Returns resource URIs, names, and descriptions`
}

func (t *listMcpResourcesTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"server_name": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional MCP server name to filter results",
			}),
		},
		[]string{},
	)
}

func (t *listMcpResourcesTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (t *listMcpResourcesTool) IsReadOnly(_ tool.Input) bool        { return true }
func (t *listMcpResourcesTool) UserFacingName(_ tool.Input) string  { return "ListMcpResources" }

func (t *listMcpResourcesTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core MCP client.
	return &tool.Result{IsError: true, Content: "ListMcpResources not yet implemented (TODO(dep))"}, nil
}

// ── ReadMcpResource ───────────────────────────────────────────────────────────

// ReadMcpResourceInput is the input schema for ReadMcpResource.
type ReadMcpResourceInput struct {
	// ServerName is the MCP server to query (required).
	ServerName string `json:"server_name"`
	// URI is the resource URI to read (required).
	URI string `json:"uri"`
}

// ReadMcpResourceTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core MCP client.
var ReadMcpResourceTool tool.Tool = &readMcpResourceTool{}

type readMcpResourceTool struct{ tool.BaseTool }

func (t *readMcpResourceTool) Name() string { return "ReadMcpResource" }

func (t *readMcpResourceTool) Description(_ tool.Input, _ tool.PermissionContext) string {
	return `Reads the contents of a specific MCP resource by URI.`
}

func (t *readMcpResourceTool) InputSchema() tool.InputSchema {
	return tool.NewInputSchema(
		map[string]json.RawMessage{
			"server_name": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The MCP server name",
			}),
			"uri": tool.PropSchema(map[string]any{
				"type":        "string",
				"description": "The resource URI to read",
			}),
		},
		[]string{"server_name", "uri"},
	)
}

func (t *readMcpResourceTool) IsConcurrencySafe(_ tool.Input) bool { return true }
func (t *readMcpResourceTool) IsReadOnly(_ tool.Input) bool        { return true }

func (t *readMcpResourceTool) UserFacingName(input tool.Input) string {
	var in ReadMcpResourceInput
	if json.Unmarshal(input, &in) == nil && in.URI != "" {
		return fmt.Sprintf("ReadMcpResource(%s)", in.URI)
	}
	return "ReadMcpResource"
}

func (t *readMcpResourceTool) Call(_ tool.Input, _ *tool.UseContext, _ tool.OnProgressFn) (*tool.Result, error) {
	// TODO(dep): Implement via Agent-Core MCP client.
	return &tool.Result{IsError: true, Content: "ReadMcpResource not yet implemented (TODO(dep))"}, nil
}
