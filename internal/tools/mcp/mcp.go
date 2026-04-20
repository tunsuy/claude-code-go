package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// ── MCPTool (proxy) ───────────────────────────────────────────────────────────

// MCPToolInput is the input schema for an MCP proxy tool call.
// The actual parameters depend on the upstream MCP server's tool schema.
type MCPToolInput struct {
	// Params contains the raw parameters to forward to the MCP server.
	Params json.RawMessage `json:"params,omitempty"`
}

// MCPProxyTool is a runtime-instantiated proxy for a single MCP server tools.
// TODO(dep): Requires Agent-Core MCP client and server registry.
type MCPProxyTool struct {
	tools.BaseTool
	// name is the canonical tool name (e.g. "mcp__server__toolName").
	name string
	// serverName is the human-readable MCP server name.
	serverName string
	// description is the upstream tool description.
	description string
	// schema is the upstream tool's input schema.
	schema tools.InputSchema
}

// NewMCPProxyTool constructs a proxy tool for a single MCP server tools.
func NewMCPProxyTool(name, serverName, description string, schema tools.InputSchema) *MCPProxyTool {
	return &MCPProxyTool{
		name:        name,
		serverName:  serverName,
		description: description,
		schema:      schema,
	}
}

func (t *MCPProxyTool) Name() string { return t.name }

func (t *MCPProxyTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return t.description
}

func (t *MCPProxyTool) InputSchema() tools.InputSchema { return t.schema }

func (t *MCPProxyTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *MCPProxyTool) IsReadOnly(_ tools.Input) bool        { return false }

func (t *MCPProxyTool) UserFacingName(_ tools.Input) string {
	return fmt.Sprintf("mcp__%s", t.serverName)
}

// MCPInfo implements tools.MCPToolInfo.
func (t *MCPProxyTool) MCPInfo() tools.MCPInfo {
	return tools.MCPInfo{ServerName: t.serverName}
}

func (t *MCPProxyTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Forward call to the MCP server via Agent-Core MCP client.
	return &tools.Result{
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
var ListMcpResourcesTool tools.Tool = &listMcpResourcesTool{}

type listMcpResourcesTool struct{ tools.BaseTool }

func (t *listMcpResourcesTool) Name() string { return "ListMcpResources" }

func (t *listMcpResourcesTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Lists all available resources from connected MCP servers.

Usage notes:
- Optionally filter by server_name
- Returns resource URIs, names, and descriptions`
}

func (t *listMcpResourcesTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"server_name": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional MCP server name to filter results",
			}),
		},
		[]string{},
	)
}

func (t *listMcpResourcesTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *listMcpResourcesTool) IsReadOnly(_ tools.Input) bool        { return true }
func (t *listMcpResourcesTool) UserFacingName(_ tools.Input) string  { return "ListMcpResources" }

func (t *listMcpResourcesTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core MCP client.
	return &tools.Result{IsError: true, Content: "ListMcpResources not yet implemented (TODO(dep))"}, nil
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
var ReadMcpResourceTool tools.Tool = &readMcpResourceTool{}

type readMcpResourceTool struct{ tools.BaseTool }

func (t *readMcpResourceTool) Name() string { return "ReadMcpResource" }

func (t *readMcpResourceTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Reads the contents of a specific MCP resource by URI.`
}

func (t *readMcpResourceTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"server_name": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The MCP server name",
			}),
			"uri": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The resource URI to read",
			}),
		},
		[]string{"server_name", "uri"},
	)
}

func (t *readMcpResourceTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *readMcpResourceTool) IsReadOnly(_ tools.Input) bool        { return true }

func (t *readMcpResourceTool) UserFacingName(input tools.Input) string {
	var in ReadMcpResourceInput
	if json.Unmarshal(input, &in) == nil && in.URI != "" {
		return fmt.Sprintf("ReadMcpResource(%s)", in.URI)
	}
	return "ReadMcpResource"
}

func (t *readMcpResourceTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core MCP client.
	return &tools.Result{IsError: true, Content: "ReadMcpResource not yet implemented (TODO(dep))"}, nil
}
