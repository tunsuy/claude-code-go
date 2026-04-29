---
package: state
import_path: internal/state
layer: infra
generated_at: 2026-04-29T02:31:52Z
source_files: [store.go]
---

# internal/state

> Layer: **Infra** · Files: 1 · Interfaces: 0 · Structs: 4 · Functions: 7

## Structs

- **AppState** — 16 fields: Settings, Verbose, MainLoopModel, ToolPermissionContext, SessionId, WorkingDir, GitBranch, Tasks, ...
- **ModelSetting** — 2 fields: ModelID, Provider
- **Store** — 6 fields
- **TaskState** — 4 fields: AgentId, Status, SessionId, TaskType

## Function Types

- `Listener` — `func(newState T, oldState T)`

## Functions

- `CloneAgentRegistry(m map[string]types.AgentId) map[string]types.AgentId`
- `CloneStringSlice(s []string) []string`
- `CloneTasks(m map[string]TaskState) map[string]TaskState`
- `GetDefaultAppState() AppState`
- `NewAppStateStore(initial AppState) *AppStateStore`
- `NewStore(initialState T, onChange func(newState T, oldState T)) *any`
- `Snapshot(store *AppStateStore) types.AppStateReader`

## Change Impact

**Exported type references (files that use types from this package):**
- `AppState` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`, `internal/tui/tui_test.go` (test), `internal/tui/update.go`
- `ModelSetting` → `internal/bootstrap/wire.go`, `internal/tui/tui_test.go` (test)

## Dependencies

**Imports:** `internal/config`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
