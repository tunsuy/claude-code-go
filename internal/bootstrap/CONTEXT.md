---
package: bootstrap
import_path: internal/bootstrap
layer: cli
generated_at: 2026-04-29T02:31:52Z
source_files: [auth.go, bootstrap.go, mcp.go, misc.go, plugin.go, root.go, run.go, session.go, tools.go, wire.go]
---

# internal/bootstrap

> Layer: **CLI** · Files: 10 · Interfaces: 0 · Structs: 2 · Functions: 8

## Structs

- **AppContainer** — 13 fields: QueryEngine, AppStateStore, ToolRegistry, MCPPool, Settings, PermAskCh, PermRespCh, Coordinator, ...
- **ContainerOptions** — 6 fields: HomeDir, WorkingDir, ModelOverride, Verbose, Debug, DebugFile

## Functions

- `BuildContainer(opts ContainerOptions) (*AppContainer, error)`
- `BuildContainerWithClient(opts ContainerOptions, client api.Client) (*AppContainer, error)`
- `BuildHeadlessContainer(opts ContainerOptions) (*AppContainer, error)`
- `Execute() error`
- `HandleFastPath(args []string) bool`
- `RegisterBuiltinTools(reg *tools.Registry)`
- `Run(args []string) error`
- `RunHeadless(container *AppContainer, prompt string, outputFormat string, maxTurns int) error`

## Dependencies

**Imports:** `internal/agentctx`, `internal/agenttype`, `internal/api`, `internal/config`, `internal/coordinator`, `internal/engine`, `internal/hooks`, `internal/mcp`, `internal/memdir`, `internal/msgqueue`, `internal/oauth`, `internal/permissions`, `internal/session`, `internal/state`, `internal/tools`, `internal/tools/agent`, `internal/tools/fileops`, `internal/tools/interact`, `internal/tools/mcp`, `internal/tools/memory`, `internal/tools/misc`, `internal/tools/shell`, `internal/tools/tasks`, `internal/tools/web`, `internal/tui`, `pkg/types`, `pkg/utils/fs`

**Imported by:** `cmd/claude`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
