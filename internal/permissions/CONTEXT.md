---
package: permissions
import_path: internal/permissions
layer: services
generated_at: 2026-04-29T02:31:52Z
source_files: [ask.go, checker.go, denial.go]
---

# internal/permissions

> Layer: **Services** · Files: 3 · Interfaces: 1 · Structs: 6 · Functions: 1

## Interfaces

### Checker (3 methods)
> Checker is the top-level permission pipeline interface.

```go
type Checker interface {
    CanUseTool(ctx context.Context, toolName string, input tools.Input, tctx *tools.UseContext) (tools.PermissionResult, error)
    RequestPermission(ctx context.Context, req PermissionRequest, tctx *tools.UseContext) (tools.PermissionResult, error)
    GetDenialCount() int
}
```

## Structs

- **AskRequest** — 8 fields: ID, ToolName, ToolUseID, Message, Input, Suggestions, BlockedPath, ProjectPath
- **AskResponse** — 3 fields: ID, Decision, UserModified
- **CheckerConfig** — 5 fields: PermCtx, Dispatcher, Registry, AskCh, RespCh
- **DenialRecord** — 4 fields: ToolName, ToolUseID, Reason, DeniedAt
- **DenialTrackingState** — 3 fields: DenialCount, LastDeniedAt, RecentDenials
- **PermissionRequest** — 4 fields: ToolName, ToolUseID, Input, ToolResult

## Functions

- `NewChecker(cfg CheckerConfig) Checker`

## Change Impact

**Exported type references (files that use types from this package):**
- `AskRequest` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `AskResponse` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `Checker` → `internal/bootstrap/wire.go`, `internal/engine/engine.go`, `internal/engine/orchestration.go`
- `CheckerConfig` → `internal/bootstrap/wire.go`
- `PermissionRequest` → `internal/engine/orchestration.go`

## Dependencies

**Imports:** `internal/hooks`, `internal/tools`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/engine`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
