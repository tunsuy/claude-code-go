---
package: bootstrap
import_path: internal/bootstrap
layer: cli
generated_at: 2026-08-30T01:47:48Z
source_files: [auth.go, bootstrap.go, mcp.go, misc.go, plugin.go, root.go, run.go, session.go, tools.go, wire.go]
---

# internal/bootstrap

> Layer: **CLI** · Files: 10 · Interfaces: 0 · Structs: 2 · Functions: 8

## Structs

- **AppContainer** — 14 fields: QueryEngine, AppStateStore, ToolRegistry, MCPPool, Settings, PermAskCh, PermRespCh, Coordinator, ...
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

## Design Notes

- **版本输出格式对齐 oracle（2026-08-30，parity A1）**：`--version` 输出从
  `claude 0.1.0` 改为 `0.1.0 (Claude Code)`。为什么：原版 TS 的格式是
  `<semver> (Claude Code)`（无二进制名前缀、带产品后缀），parity Tier-2
  对齐要求两版格式一致（版本号本身允许不同，由测试归一化处理）。
  `appVersionString()` 单独成函数而非内联，是因为 doctor / update 等
  子命令展示版本时未来可复用同一格式，且 ldflags 注入的 `appVersion`
  保持裸 semver 语义（供 User-Agent 等机器可读场景），展示格式只发生在
  这一个出口。
