---
package: coordinator
import_path: internal/coordinator
layer: core
generated_at: 2026-04-29T02:31:52Z
source_files: [adapter.go, coordinator.go, prompt.go]
---

# internal/coordinator

> Layer: **Core** · Files: 3 · Interfaces: 1 · Structs: 7 · Functions: 6

## Interfaces

### Coordinator (8 methods)
> Coordinator manages the lifecycle and message routing of multiple sub-agents.

```go
type Coordinator interface {
    SpawnAgent(ctx context.Context, req SpawnRequest) (AgentID, error)
    SendMessage(ctx context.Context, to AgentID, message string) error
    StopAgent(ctx context.Context, agentID AgentID) error
    GetAgentStatus(ctx context.Context, agentID AgentID) (AgentStatus, error)
    Subscribe(agentID AgentID) (<-chan TaskNotification, error)
    IsCoordinatorMode() bool
    ResolveAgent(target string) (AgentID, error)
    BroadcastMessage(ctx context.Context, message string) (int, error)
}
```

## Structs

- **AgentUsage** — 3 fields: TotalTokens, ToolUses, DurationMs
- **Config** — 5 fields: CoordinatorMode, RunAgent, OnProgress, OnStatusChange, SummaryGenerator
- **Event** — 8 fields: Kind, AgentID, Description, AgentName, AgentType, Activity, Detail, Status
- **MCPClientInfo** — 1 fields: Name
- **ProgressEvent** — 6 fields: AgentID, Description, AgentName, AgentType, Activity, Detail
- **SpawnRequest** — 12 fields: Description, SubagentType, Prompt, Model, AllowedTools, DenyTools, MaxTurns, ParentAgentID, ...
- **TaskNotification** — 5 fields: TaskID, Status, Summary, Result, Usage

## Function Types

- `OnProgressFn` — `func(evt ProgressEvent)`
- `OnStatusChangeFn` — `func(agentID AgentID, description string, name string, agentType string, status AgentStatus)`
- `RunAgentFn` — `func(ctx context.Context, agentID AgentID, req SpawnRequest, inboxCh <-chan string) (string, error)`
- `SummaryGeneratorFn` — `func(agentID AgentID, result string) string`

## Functions

- `DefaultSummaryGenerator(_ AgentID, result string) string`
- `FormatTaskNotification(n TaskNotification) string`
- `GetCoordinatorSystemPrompt(isSimpleMode bool) string`
- `GetCoordinatorUserContext(mcpClients []MCPClientInfo, scratchpadDir string) map[string]string`
- `New(cfg Config) Coordinator`
- `NewAgentCoordinator(c Coordinator) tools.AgentCoordinator`

## Constants

- `AgentStatusCompleted`
- `AgentStatusFailed`
- `AgentStatusRunning`
- `AgentStatusStopped`
- `EventProgress`
- `EventStatusChange`

## Change Impact

**Adapters (update when interfaces change):**
- `adapter.go`

**Test Mocks (must add new methods when interfaces change):**
- `mockCoordinator` in `internal/tools/agent/agent_test.go`
- `mockCoordinator` in `internal/tools/tasks/tasks_test.go`

**Exported type references (files that use types from this package):**
- `Config` → `internal/bootstrap/wire.go`
- `Coordinator` → `internal/bootstrap/wire.go`
- `Event` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `OnProgressFn` → `internal/bootstrap/wire.go`
- `OnStatusChangeFn` → `internal/bootstrap/wire.go`
- `ProgressEvent` → `internal/bootstrap/wire.go`
- `RunAgentFn` → `internal/bootstrap/wire.go`
- `SpawnRequest` → `internal/bootstrap/wire.go`

## Dependencies

**Imports:** `internal/tools`, `pkg/utils/ids`

**Imported by:** `internal/bootstrap`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- Coordinator vs AgentCoordinator: 两个接口是为了打破 tools→coordinator 的循环依赖。tools 包定义 AgentCoordinator（string-based），coordinator 包实现 Coordinator（typed AgentID），adapter.go 做桥接。改任一个接口都要同步改另一个。(2026-04-28)
- UseContext.Coordinator 在子 agent 里故意设为 nil，防止无限递归 spawn。见 bootstrap/wire.go buildRunAgentFn()。(2026-04-28)
- nameIndex 用于 SendMessage 名称路由，SpawnAgent 时注册，ResolveAgent 时查找。(2026-04-28)
