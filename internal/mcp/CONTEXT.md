---
package: mcp
import_path: internal/mcp
layer: services
generated_at: 2026-04-29T02:31:52Z
source_files: [adapter.go, client.go, jsonrpc.go, pool.go, transport.go]
---

# internal/mcp

> Layer: **Services** · Files: 5 · Interfaces: 2 · Structs: 17 · Functions: 4

## Interfaces

### MCPClient (7 methods)
> MCPClient represents a connection to a single MCP server.

```go
type MCPClient interface {
    ListTools(ctx context.Context) ([]MCPToolDef, error)
    CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error)
    ListResources(ctx context.Context) ([]MCPResource, error)
    ReadResource(ctx context.Context, uri string) (*MCPResourceContent, error)
    Ping(ctx context.Context) error
    Close() error
    ServerInfo() ServerInfo
}
```

### Transport (3 methods)
> Transport is the abstraction for a bidirectional JSON-RPC communication channel.

```go
type Transport interface {
    Send(ctx context.Context, msg *JSONRPCMessage) error
    Recv(ctx context.Context) (*JSONRPCMessage, error)
    Close() error
}
```

## Structs

- **HTTPTransportConfig** — 3 fields: URL, Headers, OAuth
- **JSONRPCError** — 3 fields: Code, Message, Data
- **JSONRPCMessage** — 6 fields: JSONRPC, ID, Method, Params, Result, Error
- **MCPContent** — 4 fields: Type, Text, Data, MIMEType
- **MCPOAuthConfig** — 3 fields: ClientID, CallbackPort, AuthServerMetadataURL
- **MCPResource** — 4 fields: URI, Name, Description, MIMEType
- **MCPResourceContent** — 4 fields: URI, MIMEType, Text, Blob
- **MCPToolDef** — 3 fields: Name, Description, InputSchema
- **MCPToolResult** — 3 fields: Content, IsError, Meta
- **Pool** — 2 fields
- **SSETransportConfig** — 3 fields: URL, Headers, OAuth
- **ServerCapabilities** — 3 fields: Tools, Resources, Prompts
- **ServerConfig** — 5 fields: Transport, Stdio, SSE, HTTP, Scope
- **ServerConnection** — 6 fields: Name, Status, Config, Client, Error, ReconnectAttempt
- **ServerInfo** — 4 fields: Name, Version, Capabilities, Instructions
- **StdioTransportConfig** — 3 fields: Command, Args, Env
- **WSTransportConfig** — 3 fields: URL, Headers, OAuth

## Functions

- `AdaptToTool(serverName string, def MCPToolDef, client MCPClient) tools.Tool`
- `NewPool() *Pool`
- `NewTransport(t TransportType, cfg any) (Transport, error)`
- `NormalizeToolName(name string) string`

## Constants

- `DefaultToolTimeout`
- `MaxMCPDescriptionLength`
- `StatusConnected`
- `StatusDisabled`
- `StatusFailed`
- `StatusNeedsAuth`
- `StatusPending`
- `TransportHTTP`
- `TransportSSE`
- `TransportStdio`
- `TransportWS`

## Change Impact

**Adapters (update when interfaces change):**
- `adapter.go`

**Test Mocks (must add new methods when interfaces change):**
- `mockTransport` in `internal/mcp/mcp_test.go`
- `mockMCPClient` in `internal/mcp/mcp_test.go`

**Exported type references (files that use types from this package):**
- `JSONRPCError` → `internal/bootstrap/mcp.go`
- `JSONRPCMessage` → `internal/bootstrap/mcp.go`
- `Pool` → `internal/bootstrap/wire.go`

## Dependencies

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
