---
package: bootstrap
import_path: internal/bootstrap
layer: cli
generated_at: 2026-09-05T09:11:12Z
source_files: [auth.go, bootstrap.go, mcp.go, mcp_health.go, mcp_help.go, mcp_list.go, mcp_parse.go, mcp_render.go, mcp_run.go, misc.go, plugin.go, plugin_help.go, plugin_install.go, plugin_marketplace.go, plugin_render.go, plugin_run.go, plugin_validate.go, root.go, run.go, session.go, tools.go, wire.go]
---

# internal/bootstrap

> Layer: **CLI** · Files: 22 · Interfaces: 0 · Structs: 2 · Functions: 8

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

- **mcp/plugin 命令树共用 commander 语义解析器（2026-09-05，parity B2/B5）**：oracle 的
  CLI 错误与帮助格式来自 commander.js（`error: unknown command 'x'`、`(Did you mean …?)`、
  `error: option '-s, --scope <scope>' argument missing`），与 pflag/cobra 原生格式完全不同。
  因此两棵子树的每个节点都 `DisableFlagParsing: true`，RunE 收到原始 argv 后交给共享的
  parseMCPFlags（mcp_parse.go）按 commander 语义解析。mcp_ 前缀的错误类型
  （mcpUsageError/errMCPExit 等）是共享基础设施而非 mcp 专属——plugin 树直接复用；
  保留 mcp 前缀是因为 B2 先落地，重命名会无谓扰动已测代码。
- **cobra Use 必须是裸命令名，别名走 Aliases（2026-09-05，B5）**：cobra 的 Name() 取 Use
  的首个空白分隔 token，`Use: "install|i …"` 会让主名 install 永远无法路由到该子命令
  （别名机制也不覆盖裸主名），参数落回父命令被父级 flag 解析拦截。测试里专门有
  TestPluginCmdTreeShape 钉住子命令名集合，防止回归。
- **失败自打印 + ErrSilent 静默退出（2026-09-05，B2/B5）**：run 层失败自行向 stderr 输出
  （✘ 前缀、无 "Error:" 包装），随后返回 ErrSilent 让 main 静默 exit 1。oracle 的约定是
  错误文本自带全部上下文，cobra 默认再包一层前缀会破坏字节级对齐；errMCPExit 则表示
  诊断已打印、只关心退出码（plugin validate 的失败路径）。
- **已记录的分歧（B5，见 test/parity/cases.md）**：远程 marketplace 源（owner/repo、
  https://、npm:、--claudeai）与 `plugin details/eval/init/prune/tag` 未实现——前者给出
  诚实报错，后者从帮助行与建议名中整体移除；JS JSON 解析错误措辞与 Go 不同（外层
  `Invalid JSON syntax: JSON Parse error: …` 格式一致）；hooks 模块文件只做存在性检查，
  不按 JS 加载。
