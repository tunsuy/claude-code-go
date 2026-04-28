---
package: state
import_path: internal/state
layer: infra
generated_at: 2026-04-28T11:59:48Z
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

## Dependencies

**Imports:** `internal/config`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/tui`

