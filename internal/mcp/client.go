// Package mcp provides the MCP (Model Context Protocol) client implementation.
package mcp

import (
	"context"
	"encoding/json"
)

// MCPClient represents a connection to a single MCP server.
// Corresponds to the TS @modelcontextprotocol/sdk Client instance.
type MCPClient interface {
	// ListTools enumerates tools exposed by the server.
	ListTools(ctx context.Context) ([]MCPToolDef, error)

	// CallTool invokes a named tool and returns structured results.
	CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error)

	// ListResources enumerates resources exposed by the server.
	ListResources(ctx context.Context) ([]MCPResource, error)

	// ReadResource reads a single resource by URI.
	ReadResource(ctx context.Context, uri string) (*MCPResourceContent, error)

	// Ping checks connection liveness.
	Ping(ctx context.Context) error

	// Close closes the connection and releases resources (transport level).
	Close() error

	// ServerInfo returns server information obtained during the handshake.
	ServerInfo() ServerInfo
}

// ServerInfo corresponds to the serverInfo field in the MCP initialize response.
type ServerInfo struct {
	Name         string
	Version      string
	Capabilities ServerCapabilities
	Instructions string // optional system-level hint
}

// ServerCapabilities indicates which capability sets the server supports.
type ServerCapabilities struct {
	Tools     bool
	Resources bool
	Prompts   bool
}

// MCPToolDef corresponds to a single tool definition in the MCP ListTools response.
type MCPToolDef struct {
	Name        string          // raw name (may contain "/" or other special chars)
	Description string
	InputSchema json.RawMessage // JSON Schema object
}

// MCPToolResult corresponds to the MCP CallTool response.
type MCPToolResult struct {
	Content []MCPContent
	IsError bool
	Meta    map[string]any
}

// MCPContent corresponds to an MCP content block (text / image / resource).
type MCPContent struct {
	Type     string // "text" | "image" | "resource"
	Text     string
	Data     string // base64 for image
	MIMEType string
}

// MCPResource corresponds to an MCP resource list item.
type MCPResource struct {
	URI         string
	Name        string
	Description string
	MIMEType    string
}

// MCPResourceContent is the content returned by ReadResource.
type MCPResourceContent struct {
	URI      string
	MIMEType string
	Text     string
	Blob     string // base64
}
