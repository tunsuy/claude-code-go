---
package: permissions
import_path: internal/permissions
layer: services
generated_at: 2026-08-29T07:13:26Z
source_files: [ask.go, checker.go, denial.go, filesystem.go, persist.go]
---

# internal/permissions

> Layer: **Services** · Files: 5 · Interfaces: 1 · Structs: 9 · Functions: 3

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
- **AskResponse** — 4 fields: ID, Decision, AlwaysAllow, UserModified
- **CheckerConfig** — 6 fields: PermCtx, Dispatcher, Registry, AskCh, RespCh, Persist
- **DangerousPathResult** — 3 fields: Dangerous, Matched, Reason
- **DenialLimits** — 2 fields: MaxConsecutive, MaxTotal
- **DenialRecord** — 4 fields: ToolName, ToolUseID, Reason, DeniedAt
- **DenialTrackingState** — 4 fields: DenialCount, LastDeniedAt, RecentDenials
- **PermissionRequest** — 4 fields: ToolName, ToolUseID, Input, ToolResult
- **PersistConfig** — 1 fields: ProjectDir

## Functions

- `CheckDangerousPath(path string) DangerousPathResult`
- `IsDangerousPath(path string) bool`
- `NewChecker(cfg CheckerConfig) Checker`

## Change Impact

**Exported type references (files that use types from this package):**
- `AskRequest` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `AskResponse` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `Checker` → `internal/bootstrap/wire.go`, `internal/engine/engine.go`, `internal/engine/orchestration.go`
- `CheckerConfig` → `internal/bootstrap/wire.go`
- `PermissionRequest` → `internal/engine/orchestration.go`
- `PersistConfig` → `internal/bootstrap/wire.go`

## Dependencies

**Imports:** `internal/config`, `internal/hooks`, `internal/tools`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/engine`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **拒绝降级是行为路由，不是新的 PermissionMode**（T-063）：达到阈值（连续 3 次 / 累计 20 次）后，deny 结果被改路到 askUser 让用户直接确认，而不是引入一种新模式。原因：PermissionMode 是用户显式选择的会话级语义，混入"系统自动降级"会让模式切换来源不可追溯；行为路由只需在 RequestPermission 的 Deny 分支插入判断，CanUseTool 的九层决策链保持纯函数语义（无副作用、无交互）。
- **拒绝计数与许可判定分离**：DenialTrackingState 只在 RequestPermission（交互层）记录，CanUseTool（判定层）不写状态 —— 判定结果可重放、可测试，计数属于会话运行时状态。
- **审批重置连续计数但不重置累计数**：一次用户批准证明会话重新对齐了意图，连续拒绝的"卡死"信号消失；但累计数保留，防止长期反复拒绝的会话被无限次降级打扰用户。
- **persistAllowRule 需要 mu 写锁**：用户在 TUI 点"always allow"时，engine goroutine 可能正在并发跑 CanUseTool 读规则表 —— snapshot 读（RLock）+ 追加规则（Lock）保证规则表并发安全。
- **内容模式匹配下沉到工具**（T-062/T-064）：checker 的 matchPattern 只负责解析 `Tool(content)` 结构并剥离括号，content 语义交给 `Tool.PreparePermissionMatcher` 返回的闭包判断。权限包无法穷尽每种工具的输入语义（Bash 的命令分段、FileRead 的路径 glob 各不相同），下沉让 checker 与具体工具解耦，BaseTool 默认返回 nil 保持旧工具零改动。
