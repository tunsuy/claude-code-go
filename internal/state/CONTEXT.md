---
package: state
import_path: internal/state
layer: infra
generated_at: 2026-09-23T11:03:32Z
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

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。
