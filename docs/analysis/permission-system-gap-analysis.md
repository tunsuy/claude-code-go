# 权限系统差异分析报告

> **文档状态**：功能规划  
> **创建日期**：2026-04-23  
> **跟踪 Issue**：#TBD（待创建）  
> **相关文档**：[`tool-permission-design.md`](./tool-permission-design.md)

---

## 概述

本文档基于对 Claude Code TypeScript 原版权限系统的深入分析（见 `tool-permission-design.md`），
与当前 Go 版本实现进行对比，识别差距并规划改进路径。

---

## 目录

- [一、已实现功能](#一已实现功能)
- [二、未实现/不完整功能](#二未实现不完整功能)
- [三、架构差异总结](#三架构差异总结)
- [四、优先改进计划](#四优先改进计划)
- [五、实现建议](#五实现建议)
- [六、代码质量问题](#六代码质量问题)

---

## 一、已实现功能

### 1.1 基础权限类型系统 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 七种权限模式（`PermissionMode`） | ✅ 完成 | `pkg/types/permissions.go` |
| 三元组权限行为（`PermissionBehavior`） | ✅ 完成 | `pkg/types/permissions.go` |
| 多层规则来源（`PermissionRuleSource`） | ✅ 完成 | `pkg/types/permissions.go` |
| 权限规则类型定义 | ✅ 完成 | `pkg/types/permissions.go` |

**支持的权限模式**：
- `default` - 标准模式，敏感操作需要确认
- `plan` - 规划模式，只读不写
- `acceptEdits` - 自动接受文件编辑
- `dontAsk` - 自动批准所有操作（需用户主动启用）
- `bypassPermissions` - 绕过权限检查（危险）
- `auto` - AI 分类器判断（未完全实现）
- `bubble` - 冒泡到父级（Agent 场景）

### 1.2 基本权限检查管道（9 层决策链）✅

```go
// internal/permissions/checker.go - CanUseTool 实现
1. bypassPermissions mode → 无条件 allow
2. alwaysDenyRules → deny
3. ValidateInput（工具输入验证）
4. PreToolUse Hooks
5. alwaysAllowRules → allow
6. alwaysAskRules → ask
7. PermissionMode 默认决策
8. Tool-specific CheckPermissions
9. Default mode 兜底
```

### 1.3 TUI 权限交互 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 权限对话框（`PermissionDialog`） | ✅ 完成 | `internal/tui/permissions.go` |
| 三种用户选择 | ✅ 完成 | Yes / Always Allow / No |
| 通道通信机制 | ✅ 完成 | `AskRequest` / `AskResponse` |

### 1.4 配置加载 ✅

| 功能 | 状态 | 说明 |
|------|------|------|
| 多层配置合并 | ✅ 完成 | User → Project → Local → Policy |
| `settings.json` 权限配置解析 | ✅ 完成 | - |

---

## 二、未实现/不完整功能

### 2.1 ❌ YOLO/Auto 模式 AI 分类器（最大差距）

**原版实现**：
```typescript
// 原版 src/utils/permissions/yoloClassifier.ts
- 使用独立 AI 模型判断命令安全性
- 两阶段分类器（fast stage + thinking stage）
- 投机性分类器（speculative classifier）—— 预先启动以减少延迟
- 分类器结果携带 confidence 级别
```

**当前 Go 实现**：
```go
// internal/tools/tool.go
ToAutoClassifierInput(input Input) string  // 只定义了接口，返回 ""

// internal/tools/base.go
func (b BaseTool) ToAutoClassifierInput(_ Input) string { return "" }  // 默认空实现
```

**差距分析**：
- 完全没有实现 AI 分类器逻辑
- `auto` 模式虽然定义了但无法实际使用
- 缺少投机性分类器的并行执行优化
- 缺少两阶段判断机制

**建议优先级**：🔴 P1（高级安全功能）

---

### 2.2 ❌ 拒绝追踪与自动降级机制

**原版实现**：
```typescript
// 原版 src/utils/permissions/denialTracking.ts
export const DENIAL_LIMITS = {
  maxConsecutive: 3,   // 连续拒绝 3 次
  maxTotal: 20,        // 总共拒绝 20 次
}
// 超过阈值后自动降级回用户交互模式
```

**当前 Go 实现**：
```go
// internal/permissions/denial.go
type DenialTrackingState struct {
    DenialCount    int
    LastDeniedAt   time.Time
    RecentDenials  []DenialRecord
}
// 只记录，没有降级逻辑
```

**差距分析**：
- 只实现了计数功能，没有实现 `shouldFallbackToPrompting` 降级判断
- 没有 `maxConsecutive` 和 `maxTotal` 阈值配置

**建议优先级**：🟡 P2

**修复难度**：低（~50 行代码）

---

### 2.3 ❌ Bash 命令 AST 解析与安全分析

**原版实现**：
```typescript
// 原版 src/tools/BashTool/bashPermissions.ts
import { checkSemantics, parseForSecurityFromAst } from '../../utils/bash/ast.js'
import { getCommandSubcommandPrefix, splitCommand_DEPRECATED } from '../../utils/bash/commands.js'
// AST 解析、前缀提取、危险模式检测、子命令拆分、沙箱判定
```

**当前 Go 实现**：
```go
// internal/tools/shell/bash.go
// 完全没有命令解析和安全分析
cmd := exec.CommandContext(timeoutCtx, "bash", "-c", in.Command)  // 直接执行
```

**差距分析**：
- 没有 bash AST 解析器
- 没有危险命令模式检测（如 `python *`、`eval`、`sudo`）
- 没有子命令分割（`&&`、`||`、`;` 等复合命令）
- 没有沙箱执行支持

**建议优先级**：🔴 P0（关键安全功能）

**修复难度**：高（需要实现 bash 解析器或集成第三方库）

---

### 2.4 ❌ 危险文件/目录保护（文件系统权限）

**原版实现**：
```typescript
// 原版 src/utils/permissions/filesystem.ts
export const DANGEROUS_FILES = [
  '.gitconfig', '.gitmodules',
  '.bashrc', '.bash_profile', '.zshrc', '.zprofile', '.profile',
  '.ripgreprc', '.mcp.json', '.claude.json',
] as const

export const DANGEROUS_DIRECTORIES = [
  '.git', '.vscode', '.idea', '.claude',
] as const
```

**当前 Go 实现**：
- **完全没有**类似的危险文件/目录列表
- 没有路径遍历攻击检测
- 没有对敏感配置文件的额外保护

**建议优先级**：🔴 P0（关键安全功能）

**修复难度**：低（~100 行代码）

---

### 2.5 ❌ 权限持久化机制（"始终允许"功能）

**原版实现**：
```typescript
// 原版 PermissionContext.ts
async handleUserAllow(updatedInput, permissionUpdates, feedback?) {
  await this.persistPermissions(permissionUpdates)  // 持久化到磁盘
  setToolPermissionContext(applyPermissionUpdates(..., updates))  // 更新内存
}
```

**当前 Go 实现**：
```go
// internal/tui/permissions.go
PermissionChoiceAlwaysAllow  // 虽然有选项，但...

// internal/permissions/checker.go
// 没有 persistPermissions 实现
// 没有 applyPermissionUpdates 实现
```

**差距分析**：
- 用户选择 "Always Allow" 后没有实际持久化到 `settings.json`
- 权限上下文是静态快照，不支持动态更新

**建议优先级**：🔴 P0（核心功能缺失）

**修复难度**：中（~200 行代码）

---

### 2.6 ❌ 权限建议（Suggestions）系统

**原版实现**：
```typescript
// PermissionAskDecision 可以携带 suggestions
suggestions: PermissionUpdate[]  // 建议的规则更新选项
```

**当前 Go 实现**：
```go
// internal/permissions/ask.go
type AskRequest struct {
    Suggestions []tools.PermissionResult  // 类型不对，且未使用
}
```

**差距分析**：
- `Suggestions` 字段类型不正确（应该是 `PermissionUpdate[]`）
- TUI 层没有渲染建议选项
- 用户无法一键应用建议的规则

**建议优先级**：🟡 P2

**修复难度**：中

---

### 2.7 ❌ Shell 规则通配符匹配引擎

**原版实现**：
```typescript
// 原版 src/utils/permissions/shellRuleMatching.ts
- git * → 匹配所有 git 命令
- npm test → 精确匹配
- prefix:* 风格支持
- glob 通配符
```

**当前 Go 实现**：
```go
// internal/permissions/checker.go
func matchPattern(pattern, toolName string, matcherFn func(string) bool) bool {
    // 只有基本的 ToolName(content) 解析
    // 依赖 matcherFn（通常为 nil）
}
```

**差距分析**：
- 通配符匹配功能不完整
- 没有专门的 shell 命令模式匹配器
- `prefix:*` 和 glob 语法不支持

**建议优先级**：🟡 P1

**修复难度**：中（~150 行代码）

---

### 2.8 ❌ PermissionRequest Hooks

**原版实现**：
```
PreToolUse Hook → checkPermissions → PermissionRequest Hook → 用户确认 → PostToolUse Hook
```

**当前 Go 实现**：
- ✅ PreToolUse Hook（已实现）
- ❌ **PermissionRequest Hook**（在用户看到弹窗前/后触发）—— 未实现
- ⚠️ PostToolUse Hook（部分）

**建议优先级**：🟡 P2

---

### 2.9 ❌ 危险规则检测与 Auto 模式剥离

**原版实现**：
```typescript
// 原版 permissionSetup.ts
isDangerousBashPermission(toolName, ruleContent)
// Auto 模式启用时自动剥离过于宽泛的规则
strippedDangerousRules  // 存储被剥离的规则以便恢复
```

**当前 Go 实现**：
```go
// pkg/types/permissions.go
// 没有 StrippedDangerousRules 字段
// 没有危险规则检测函数
```

**建议优先级**：🟡 P2

---

### 2.10 ❌ 并发权限请求处理

**原版实现**：
```typescript
// PermissionConfirmQueue
// 多个工具调用的权限请求并发管理
// 逐个展示给用户
```

**当前 Go 实现**：
```go
// 单通道设计
askCh  chan<- AskRequest
respCh <-chan AskResponse
// 没有队列管理机制
```

**建议优先级**：🟡 P2

---

### 2.11 ❌ 规则冲突/遮蔽检测

**原版实现**：
```typescript
// shadowedRuleDetection.ts
// 检测冗余/冲突的规则配置
```

**当前 Go 实现**：
- 没有类似功能

**建议优先级**：🟢 P3

---

### 2.12 ❌ 远程 Killswitch

**原版实现**：
```typescript
bypassPermissionsKillswitch  // 远程紧急关闭 bypass 模式
```

**当前 Go 实现**：
- 没有远程控制机制

**建议优先级**：🟢 P3（可选安全功能）

---

### 2.13 ⚠️ 沙箱执行

**原版实现**：
```typescript
// SandboxManager
// 决定是否在沙箱中运行命令
```

**当前 Go 实现**：
```go
// internal/tools/shell/bash.go
// TODO(dep): Full sandbox implementation requires Agent-Core sandbox manager.
DangerouslyDisableSandbox bool  // 只有字段定义，没有实际沙箱
```

**建议优先级**：🟡 P2

---

### 2.14 ❌ Legacy 工具名映射

**原版实现**：
```typescript
// 旧名称（如 Task → Agent）自动映射
// 防止通过旧名称绕过规则
```

**当前 Go 实现**：
- 没有工具名映射机制

**建议优先级**：🟢 P3

---

### 2.15 ❌ MCP 工具命名空间

**原版实现**：
```typescript
// mcp__server__tool 前缀格式
// 防止与内置工具冲突
// 支持 server 级别的规则匹配（mcp__server1 匹配所有该 server 工具）
```

**当前 Go 实现**：
- MCP 工具基础结构存在，但权限规则匹配不完整

**建议优先级**：🟡 P2

---

## 三、架构差异总结

| 维度 | 原版 TypeScript | 当前 Go | 完成度 |
|------|----------------|---------|--------|
| **权限决策管道** | 完整 10+ 层 | 9 层基础实现 | ⚠️ 80% |
| **AI 分类器（Auto mode）** | 完整实现 | 只有接口定义 | ❌ 0% |
| **Bash 安全分析** | AST 解析 + 多重检查 | 直接执行 | ❌ 0% |
| **权限持久化** | 完整持久化 + 动态更新 | 只读快照 | ❌ 20% |
| **文件系统保护** | 危险文件/目录列表 | 无 | ❌ 0% |
| **Shell 规则匹配** | 完整通配符引擎 | 基础匹配 | ⚠️ 30% |
| **降级机制** | 自动降级 | 只有计数 | ⚠️ 40% |
| **并发权限处理** | 队列管理 | 单通道 | ⚠️ 50% |
| **Hooks 系统** | 4 种 hooks | 1.5 种 | ⚠️ 40% |
| **TUI 权限交互** | 丰富的 UI 组件 | 基础对话框 | ⚠️ 60% |

**整体评估**：约 40-50% 功能覆盖率

---

## 四、优先改进计划

### 🔴 P0 — 关键安全功能（必须优先实现）

| # | 功能 | 工作量估算 | 影响范围 |
|---|------|-----------|----------|
| 1 | 危险文件/目录保护列表 | 0.5 天 | `internal/tools/fileops/` |
| 2 | Bash 命令基础安全检查 | 2 天 | `internal/tools/shell/` |
| 3 | 权限持久化（Always Allow 生效） | 1 天 | `internal/permissions/`, `internal/config/` |

**P0 总工作量**：~3.5 天

### 🟡 P1 — 功能完整性

| # | 功能 | 工作量估算 |
|---|------|-----------|
| 4 | 拒绝降级机制 | 0.5 天 |
| 5 | Shell 规则通配符匹配 | 1 天 |
| 6 | PermissionRequest Hook | 1 天 |

**P1 总工作量**：~2.5 天

### 🟢 P2 — 高级功能

| # | 功能 | 工作量估算 |
|---|------|-----------|
| 7 | YOLO/Auto 分类器基础框架 | 3 天 |
| 8 | 沙箱执行 | 5 天 |
| 9 | 规则冲突检测 | 1 天 |
| 10 | MCP 工具命名空间完善 | 1 天 |

**P2 总工作量**：~10 天

---

## 五、实现建议

### 5.1 危险文件/目录保护（P0）

```go
// 建议位置：pkg/types/security.go 或 internal/permissions/filesystem.go

// DangerousFiles 定义不允许修改的敏感文件
var DangerousFiles = []string{
    ".gitconfig",
    ".gitmodules",
    ".bashrc",
    ".bash_profile",
    ".zshrc",
    ".zprofile",
    ".profile",
    ".ripgreprc",
    ".mcp.json",
    ".claude.json",
    "settings.json",  // Claude 配置
}

// DangerousDirectories 定义敏感目录
var DangerousDirectories = []string{
    ".git",
    ".vscode",
    ".idea",
    ".claude",
    ".ssh",
    ".gnupg",
}

// IsDangerousPath 检查路径是否涉及敏感文件/目录
func IsDangerousPath(path string) bool {
    // 实现逻辑...
}
```

### 5.2 Bash 安全检查（P0）

```go
// 建议位置：internal/tools/shell/security.go

// DangerousPatterns 危险命令模式
var DangerousPatterns = []string{
    `^sudo\s`,           // sudo 命令
    `^su\s`,             // su 命令
    `\brm\s+-rf\s+/`,    // rm -rf /
    `\beval\s`,          // eval 命令
    `>\s*/etc/`,         // 写入 /etc
    `\bchmod\s+777\s`,   // 危险权限
    `\bdd\s+if=`,        // dd 命令
}

// AnalyzeCommand 分析 bash 命令的安全风险
func AnalyzeCommand(cmd string) (riskLevel RiskLevel, reasons []string) {
    // 实现逻辑...
}
```

### 5.3 权限持久化（P0）

```go
// 建议修改：internal/permissions/checker.go

// PersistPermission 持久化权限规则到 settings.json
func (c *PermissionsChecker) PersistPermission(rule PermissionRule, scope PermissionRuleSource) error {
    // 1. 读取当前配置
    // 2. 合并新规则
    // 3. 写回配置文件
    // 4. 更新内存中的规则缓存
}
```

---

## 六、代码质量问题

在审查过程中发现的需要清理的问题：

### 6.1 调试代码残留

```go
// internal/permissions/checker.go
// 需要移除的 DEBUG 日志
if f, ferr := os.OpenFile("/tmp/claude-code-debug.log", ...); ferr == nil {
    fmt.Fprintf(f, "[DEBUG] CanUseTool step 7: ...")
    f.Close()
}
```

**建议**：使用统一的日志框架替代临时调试代码。

### 6.2 类型定义不一致

```go
// internal/permissions/ask.go
type AskRequest struct {
    Suggestions []tools.PermissionResult  // ❌ 类型不对
}

// 应该改为
type AskRequest struct {
    Suggestions []PermissionUpdate  // ✅ 正确类型
}
```

---

## 相关 Issue

完成本文档后，建议创建以下 GitHub Issues：

1. **[Epic] 权限系统完善** — 跟踪整体进度
2. **[P0] 实现危险文件/目录保护** — 安全关键
3. **[P0] 实现 Bash 命令安全检查** — 安全关键
4. **[P0] 实现权限持久化（Always Allow）** — 核心功能
5. **[P1] 实现拒绝降级机制** — 用户体验
6. **[P1] 完善 Shell 规则通配符匹配** — 功能完整性

---

## 变更历史

| 日期 | 版本 | 变更内容 |
|------|------|----------|
| 2026-04-23 | v1.0 | 初始版本，完成差异分析 |
