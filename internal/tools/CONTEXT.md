---
package: tools
import_path: internal/tools
layer: tools
generated_at: 2026-04-29T02:31:52Z
source_files: [base.go, registry.go, tool.go]
---

# internal/tools

> Layer: **Tools** · Files: 3 · Interfaces: 6 · Structs: 10 · Functions: 3

## Interfaces

### AgentCoordinator (9 methods)
> AgentCoordinator is the subset of coordinator.Coordinator that tools need.

```go
type AgentCoordinator interface {
    SpawnAgent(ctx context.Context, req AgentSpawnRequest) (string, error)
    SendMessage(ctx context.Context, agentID string, message string) error
    StopAgent(ctx context.Context, agentID string) error
    GetAgentStatus(ctx context.Context, agentID string) (string, error)
    GetAgentResult(ctx context.Context, agentID string) (result string, status string, err error)
    ListAgents() map[string]string
    WaitForAgent(ctx context.Context, agentID string) (string, error)
    ResolveAgent(ctx context.Context, target string) (string, error)
    BroadcastMessage(ctx context.Context, message string) (int, error)
}
```

### MCPToolInfo (1 methods)
> MCPToolInfo is the optional sub-interface for MCP-backed tools.

```go
type MCPToolInfo interface {
    MCPInfo() MCPInfo
}
```

### PathTool (1 methods)
> PathTool is the optional sub-interface implemented by file-operation tools.

```go
type PathTool interface {
    GetPath(input Input) string
}
```

### PermissionContext (1 methods)
> PermissionContext provides current permission-mode information to tools for

```go
type PermissionContext interface {
    Mode() string
}
```

### SearchOrReadTool (1 methods)
> SearchOrReadTool is the optional sub-interface for search/read tools.

```go
type SearchOrReadTool interface {
    IsSearchOrRead(input Input) SearchOrReadResult
}
```

### Tool (19 methods)
> Tool is the interface every built-in tool must implement.

```go
type Tool interface {
    Name() string
    Aliases() []string
    Description(input Input, permCtx PermissionContext) string
    InputSchema() InputSchema
    Prompt(ctx context.Context, permCtx PermissionContext) (string, error)
    MaxResultSizeChars() int
    SearchHint() string
    IsConcurrencySafe(input Input) bool
    IsReadOnly(input Input) bool
    IsDestructive(input Input) bool
    IsEnabled() bool
    InterruptBehavior() InterruptBehavior
    ValidateInput(input Input, ctx *UseContext) (ValidationResult, error)
    CheckPermissions(input Input, ctx *UseContext) (PermissionResult, error)
    PreparePermissionMatcher(input Input) (func(pattern string) bool, error)
    Call(input Input, ctx *UseContext, onProgress OnProgressFn) (*Result, error)
    MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error)
    ToAutoClassifierInput(input Input) string
    UserFacingName(input Input) string
}
```

## Structs

- **AgentSpawnRequest** — 10 fields: Description, Prompt, AllowedTools, DenyTools, MaxTurns, AgentType, Model, Background, ...
- **BaseTool**
- **InputSchema** — 3 fields: Type, Properties, Required
- **MCPInfo** — 2 fields: ServerName, ToolName
- **PermissionResult** — 2 fields: Behavior, Reason
- **Registry** — 4 fields
- **Result** — 3 fields: Content, IsError, ContextModifier
- **SearchOrReadResult** — 3 fields: IsSearch, IsRead, Path
- **UseContext** — 5 fields: Ctx, AbortCh, PermCtx, Coordinator, AgentID
- **ValidationResult** — 2 fields: OK, Reason

## Function Types

- `OnProgressFn` — `func(data any)`

## Functions

- `NewInputSchema(props map[string]json.RawMessage, required []string) InputSchema`
- `NewRegistry() *Registry`
- `PropSchema(def map[string]any) json.RawMessage`

## Constants

- `InterruptBehaviorBlock`
- `InterruptBehaviorCancel`
- `PermissionAllow`
- `PermissionAsk`
- `PermissionDeny`
- `PermissionPassthrough`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `stubTool` in `internal/engine/orchestration_test.go`
- `stubRegistry` in `internal/permissions/checker_test.go`
- `mockTool` in `internal/tools/registry_test.go`
- `stubTool` in `test/integration/engine_e2e_test.go`

**Exported type references (files that use types from this package):**
- `AgentCoordinator` → `internal/bootstrap/wire.go`, `internal/coordinator/adapter.go`, `internal/tools/agent/agent_test.go` (test), `internal/tools/tasks/tasks_test.go` (test), `internal/tui/init.go` + 1 more
- `AgentSpawnRequest` → `internal/coordinator/adapter.go`, `internal/coordinator/adapter_test.go` (test), `internal/tools/agent/agent.go`, `internal/tools/agent/agent_test.go` (test), `internal/tools/tasks/tasks.go` + 1 more
- `BaseTool` → `internal/engine/engine_test.go` (test), `internal/mcp/adapter.go`, `internal/tools/agent/agent.go`, `internal/tools/agent/getagentstatus.go`, `internal/tools/agent/sendmessage.go` + 15 more
- `InputSchema` → `internal/engine/orchestration_test.go` (test), `internal/mcp/adapter.go`, `internal/permissions/checker_test.go` (test), `internal/tools/agent/agent.go`, `internal/tools/agent/getagentstatus.go` + 16 more
- `MCPInfo` → `internal/mcp/adapter.go`, `internal/tools/mcp/mcp.go`
- `MCPToolInfo` → `internal/mcp/adapter.go`, `internal/tools/mcp/mcp.go`
- `OnProgressFn` → `internal/engine/orchestration_test.go` (test), `internal/mcp/adapter.go`, `internal/permissions/checker_test.go` (test), `internal/tools/agent/agent.go`, `internal/tools/agent/getagentstatus.go` + 16 more
- `PathTool` → `internal/tools/fileops/fileedit.go`, `internal/tools/fileops/fileread.go`, `internal/tools/fileops/filewrite.go`, `internal/tools/fileops/notebookedit.go`
- `PermissionContext` → `internal/engine/orchestration_test.go` (test), `internal/engine/query.go`, `internal/mcp/adapter.go`, `internal/permissions/checker_test.go` (test), `internal/tools/agent/agent.go` + 17 more
- `PermissionResult` → `internal/engine/orchestration_test.go` (test), `internal/mcp/adapter.go`, `internal/permissions/ask.go`, `internal/permissions/checker.go`, `internal/permissions/checker_test.go` (test) + 1 more
- `Registry` → `internal/bootstrap/mcp.go`, `internal/bootstrap/tools.go`, `internal/bootstrap/wire.go`, `internal/engine/engine.go`, `internal/engine/orchestration.go` + 1 more
- `Result` → `internal/engine/orchestration_test.go` (test), `internal/mcp/adapter.go`, `internal/permissions/checker_test.go` (test), `internal/tools/agent/agent.go`, `internal/tools/agent/getagentstatus.go` + 16 more
- `Tool` → `internal/engine/orchestration_test.go` (test), `internal/mcp/adapter.go`, `internal/permissions/checker.go`, `internal/permissions/checker_test.go` (test), `internal/tools/agent/agent.go` + 31 more
- `UseContext` → `internal/bootstrap/mcp.go`, `internal/bootstrap/run.go`, `internal/bootstrap/wire.go`, `internal/engine/cache_params.go`, `internal/engine/cache_params_test.go` (test) + 34 more
- `ValidationResult` → `internal/engine/orchestration_test.go` (test), `internal/mcp/adapter.go`, `internal/permissions/checker_test.go` (test), `internal/tools/fileops/fileread.go`, `internal/tools/fileops/glob.go` + 5 more

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/coordinator`, `internal/engine`, `internal/mcp`, `internal/permissions`, `internal/tools/agent`, `internal/tools/fileops`, `internal/tools/interact`, `internal/tools/mcp`, `internal/tools/memory`, `internal/tools/misc`, `internal/tools/shell`, `internal/tools/tasks`, `internal/tools/web`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
