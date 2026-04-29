---
package: mcp
import_path: internal/tools/mcp
layer: tools
generated_at: 2026-04-29T02:31:52Z
source_files: [doc.go, mcp.go]
---

# internal/tools/mcp

> Layer: **Tools** · Files: 2 · Interfaces: 0 · Structs: 4 · Functions: 1

## Structs

- **ListMcpResourcesInput** — 1 fields: ServerName
- **MCPProxyTool** — 5 fields (embeds tools.BaseTool)
- **MCPToolInput** — 1 fields: Params
- **ReadMcpResourceInput** — 2 fields: ServerName, URI

## Functions

- `NewMCPProxyTool(name string, serverName string, description string, schema tools.InputSchema) *MCPProxyTool`

## Dependencies

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
