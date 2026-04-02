# 核心层设计评审报告

> 评审人：Tech Lead
> 评审日期：2026-04-02
> 评审对象：`docs/project/design/core.md`（核心层详细设计）
> 参考文档：`docs/project/architecture.md`、`docs/project/design/tools.md`
> 评审结论：**条件通过（Conditional Pass / APPROVED_WITH_CHANGES）**

---

## 总体评价

核心层设计在架构合理性、接口完整性和 Go 惯用法三个维度上表现优秀，具备成为正式编码基础的条件。`tool.Tool` 接口方法分组清晰、语义明确、边界职责合理，能够支撑 40+ 工具的统一实现。引擎层的 `EngineMsg` channel 协议、权限 ask 双向通道设计、三级压缩策略均有具体落地方案，设计成熟度高。

本次发现 **1 个阻塞项（BLOCKER）、2 个重要项（MAJOR）、6 个次要项（MINOR）**，必须在阻塞项和重要项解决后方可正式锁定契约并开始编码。

---

## 一、已确认的正式契约：`tool.Tool` 接口

以下 Go 接口定义经本次评审确认，**待阻塞项 B-1 修复后即成为正式契约**，Agent-Tools 可据此开始编码。

> ⚠️ 相较于 `core.md` 原稿，此处已包含 **MINOR m-1** 的改动：`InterruptBehavior()` 返回类型由 `string` 改为类型化枚举。Agent-Core 须同步更新实现代码。

```go
// Package tool 定义工具接口契约，供工具层实现、引擎层调用。
// 此包不应依赖 engine/permissions/tui 等其他核心包，避免循环依赖。
package tool

import (
    "context"
    "encoding/json"
)

// Input 表示工具调用的输入参数（JSON 对象）。
type Input = json.RawMessage

// InputSchema 描述工具输入的 JSON Schema（object 类型）。
type InputSchema struct {
    Type       string                     `json:"type"` // 固定为 "object"
    Properties map[string]json.RawMessage `json:"properties,omitempty"`
    Required   []string                   `json:"required,omitempty"`
    Extra      map[string]json.RawMessage `json:"-"`
}

// InterruptBehavior 表示用户发送新消息时工具的中断行为（类型化枚举）。
type InterruptBehavior string

const (
    InterruptBehaviorCancel InterruptBehavior = "cancel" // 停止工具并丢弃结果
    InterruptBehaviorBlock  InterruptBehavior = "block"  // 继续执行，新消息等待
)

// Tool 是所有工具必须实现的接口。
// 方法分为五组：身份/元数据、并发/安全性、权限、执行、序列化。
type Tool interface {
    // ── 身份与元数据 ──────────────────────────────────────

    Name() string
    Aliases() []string
    Description(input Input, permCtx PermissionContext) string
    InputSchema() InputSchema
    Prompt(ctx context.Context, permCtx PermissionContext) (string, error)
    MaxResultSizeChars() int
    SearchHint() string

    // ── 并发与安全性 ──────────────────────────────────────

    IsConcurrencySafe(input Input) bool
    IsReadOnly(input Input) bool
    IsDestructive(input Input) bool
    IsEnabled() bool
    InterruptBehavior() InterruptBehavior // 已改为类型化枚举

    // ── 权限 ─────────────────────────────────────────────

    ValidateInput(input Input, ctx *UseContext) (ValidationResult, error)
    CheckPermissions(input Input, ctx *UseContext) (PermissionResult, error)
    PreparePermissionMatcher(input Input) (func(pattern string) bool, error)

    // ── 执行 ─────────────────────────────────────────────

    // Call 执行工具并返回结果。
    // NOTE: context.Context 通过 ctx.Ctx 传入（见 MAJOR M-1 讨论）。
    // onProgress 可为 nil；工具应容忍 nil 回调。
    Call(input Input, ctx *UseContext, onProgress OnProgressFn) (*Result, error)

    // ── 序列化 ───────────────────────────────────────────

    MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error)
    ToAutoClassifierInput(input Input) string
    UserFacingName(input Input) string
    // NOTE: GetPath 已从必需接口移至可选子接口 PathTool（见 BLOCKER B-1）
}

// PathTool 是文件操作工具的可选子接口（引擎通过类型断言调用）。
type PathTool interface {
    Tool
    GetPath(input Input) string
}

// SearchOrReadTool 是搜索/读取工具的可选子接口（引擎用于 UI 折叠判断）。
type SearchOrReadTool interface {
    Tool
    IsSearchOrRead(input Input) SearchOrReadResult
}

// MCPToolInfo 是 MCP 工具的可选子接口（仅用于元数据展示）。
type MCPToolInfo interface {
    Tool
    MCPInfo() MCPInfo
}
```

### 接口方法速查表

| 方法 | 必须实现 | 零值/默认 |
|------|---------|---------|
| `Name()` | ✅ | — |
| `Aliases()` | ✅ | 返回 `nil` |
| `Description()` | ✅ | — |
| `InputSchema()` | ✅ | — |
| `Prompt()` | ✅ | 返回 `("", nil)` |
| `MaxResultSizeChars()` | ✅ | 返回 `-1`（不限制） |
| `SearchHint()` | ✅ | 返回 `""` |
| `IsConcurrencySafe()` | ✅ | 驱动引擎批次分组 |
| `IsReadOnly()` | ✅ | 用于权限推断 |
| `IsDestructive()` | ✅ | 不可逆操作返回 `false` |
| `IsEnabled()` | ✅ | Feature flag 门控 |
| `InterruptBehavior()` | ✅ | 返回类型化枚举常量 |
| `ValidateInput()` | ✅ | 无需验证时返回 `ValidationResult{OK: true}` |
| `CheckPermissions()` | ✅ | 委托时返回 `PermissionResult{Behavior: PermissionPassthrough}` |
| `PreparePermissionMatcher()` | ✅ | 不参与模式匹配时返回 `nil` |
| `Call()` | ✅ | 核心执行方法 |
| `MapResultToToolResultBlock()` | ✅ | 输出有效 Anthropic API 内容块 |
| `ToAutoClassifierInput()` | ✅ | 跳过分类器时返回 `""` |
| `UserFacingName()` | ✅ | 简单工具可返回 `Name()` |

> **建议**：Agent-Tools 提供 `BaseTool` 嵌入 struct，为 `Aliases`、`SearchHint`、`IsDestructive`、`MaxResultSizeChars`、`ToAutoClassifierInput`、`Prompt` 等可合理零值的方法提供默认实现，减少样板代码。

---

## 二、评审维度详细结论

### 2.1 Tool 接口完整性

**结论：条件通过（含 BLOCKER B-1）**

接口覆盖了 TS 原版 `Tool` 类型的全部核心能力。方法按五组划分逻辑清晰。与 `tools.md` 中 Agent-Tools 预期的简化接口（8 方法）相比，正式接口扩展为 19 方法（B-1 修复后移除 `GetPath`），扩展有充分理由：

| 新增方法组 | 方法数 | 必要性 |
|-----------|--------|--------|
| 序列化辅助（`MapResultToToolResultBlock`、`UserFacingName`） | 2 | 引擎统一序列化，避免各工具重复实现 |
| 安全性标记（`IsDestructive`、`InterruptBehavior`） | 2 | 权限系统和中断逻辑需精确语义区分 |
| 元数据增强（`Aliases`、`MaxResultSizeChars`、`SearchHint`、`Prompt`） | 4 | 大型结果处理、工具搜索、系统提示注入均需此信息 |
| 权限细化（`ValidateInput`、`PreparePermissionMatcher`） | 2 | 输入校验与 Hook 模式匹配的解耦设计 |

**BLOCKER B-1**：`GetPath` 在 `Tool` 接口（§1.2）和 `PathTool` 子接口（§7.1）中同时定义，形成矛盾——必需方法不能同时是可选子接口方法。**须从主接口 `Tool` 中移除 `GetPath`，仅保留在 `PathTool` 子接口中**（引擎通过类型断言调用）。这是阻塞项，会导致 Agent-Tools 实现者写出错误的工具。

---

### 2.2 QueryEngine 接口

**结论：通过**

`Query(ctx, params) (<-chan EngineMsg, error)` 的 channel 流式设计与 Bubble Tea 的 `tea.Cmd` 模型兼容。`EngineMsg` 封闭接口（私有 `engineMsg()` 方法）防止外部注入未知消息类型，是合理的封装策略。14 种 Msg 类型完整覆盖流式文本、工具生命周期、权限、会话状态、压缩、预算等全部事件面。

**MINOR m-4**：`EngineMsg` 封闭设计意味着新增消息类型是破坏性变更（需更新 TUI 的 switch 语句）。应在 `msg.go` 和设计文档中明确说明此约束。

**建议优化 A**：在 `QueryEngine.Query` 注释中补充"调用方必须持续消费 channel 直至关闭，不得阻塞消费"；channel 缓冲 256 在高并发场景（10 工具 × 进度事件）可能不足，建议支持 `CLAUDE_CODE_ENGINE_MSG_BUF_SIZE` 环境变量覆盖。

---

### 2.3 权限系统

**结论：通过（含重要项 M-1 说明）**

三级决策（alwaysDeny → alwaysAllow → alwaysAsk → tool.CheckPermissions → mode 默认）语义清晰，`PermissionDecisionReason` 提供完整溯源链。`PermissionAskMsg` 内嵌 `chan<- PermissionResponse` 是地道的 Go 请求-回复模式。

`PermissionBehavior` 四值枚举（allow/deny/ask/passthrough）明确区分工具层 passthrough 与权限层最终决策，比用 nil 表示"无意见"更清晰。

**MAJOR M-1**：`UseContext` 将 `context.Context` 嵌入结构体（字段 `Ctx`）违反了 Go 惯用法（context 应作为函数首参）。存在两个风险：(a) 工具派生 goroutine 时可能捕获 `uctx.Ctx` 而不能正确响应取消；(b) `ContextModifier` 修改 `*UseContext` 时 `Ctx` 字段是否跟随变化语义不明。**必须明确处理**：推荐方案为 `Call(ctx context.Context, input Input, uctx *UseContext, onProgress OnProgressFn) (*Result, error)`（将 context 提为首参，从 `UseContext` 移除 `Ctx`）。

60s 超时 deny 须使用 `context.WithTimeout` 实现：

```go
askCtx, cancel := context.WithTimeout(parentCtx, 60*time.Second)
defer cancel()
select {
case resp := <-responseCh:
    return resp, nil
case <-askCtx.Done():
    return denyResponse, nil
}
```

---

### 2.4 Hooks 系统

**结论：通过**

`hooks.Executor` 的五个方法与 TS hook 事件一一对应，`AggregatedHookResult` 涵盖 block/allow/deny/input-mutation/context-injection 等全部结果类型。Subprocess JSON 协议成熟，进程隔离保证 hook 无法直接污染引擎状态。

**MINOR m-3**：`AggregatedHookResult.PermissionBehavior` 字段类型为 `string`，与系统其余部分使用的 `tool.PermissionBehavior` 不一致。`hooks` 包已引用 `tool` 包，无循环依赖风险，应统一类型。

**建议优化 B**：12 个 `HookEvent` 枚举值中只有 5 个有对应 `Executor` 方法，建议在注释中说明各事件的 MVP 支持状态。

---

### 2.5 并发模型

**结论：通过（含重要项 M-2）**

`partitionToolCalls` 算法（连续只读工具并发、写工具独占串行、批次间严格有序）与 TS 原版逻辑等价，保证了外部状态一致性。10 并发工具上限（可通过环境变量覆盖）防止资源耗尽。

**MAJOR M-2**：`tool.Message` 类型注释提到"完整类型在 types/message 包"，但 `types/message` 包在设计文档中未定义，两者关系不明。`QueryEngine.History()`、`UseContext.Messages`、`Result.NewMessages` 均使用 `[]tool.Message`，是 agentic loop 的关键路径。**必须在实现前明确**：要么以 `tool.Message` 为规范类型并删除对 `types/message` 的引用，要么定义 `types/message` 包并说明映射关系。

**MINOR m-2**：`BudgetTracker` 设计文档中所有字段为导出字段，实现时应改为非导出字段并提供线程安全方法（`AddTokens`、`ShouldCompact` 等），以 `sync.Mutex` 保护。

**建议优化 C**：`Result.ContextModifier` 应在引擎编排层断言"若 `IsConcurrencySafe(input) == true` 且 `result.ContextModifier != nil`，则记录 warning 并丢弃 modifier"，防止并发场景下数据竞争，现有文档仅有注释约束。

---

### 2.6 Go 惯用法

**结论：通过（含 M-1 改动后）**

符合 Go 惯用法的设计点：
- `Input = json.RawMessage` 类型别名清晰
- `(result, error)` 二元组错误处理
- `engineMsg()` 私有方法封闭接口
- `sync.RWMutex` 保护 Registry
- `UseContext` 按调用传入不长持
- `PermissionPassthrough` 哨兵值优于 nil

**MINOR m-5**：`Coordinator.SendMessage` 支持 `agentID = "swarm"` 广播，但 `AgentStatus.State` 未覆盖广播部分失败场景，`WaitAll` 的聚合错误语义未定义。建议定义 `SwarmError` 类型包装各 Agent 错误，并明确 `WaitAll` 返回第一个错误还是聚合所有错误。

**MINOR m-6**：`IMAGE_MAX_TOKEN_SIZE=2000` 命名暗示仅控制图片，但实际也适用于 Bash/FileRead/Grep/Glob 等工具输出。建议在 Go 实现中重命名为 `MicroCompactToolResultMaxTokens`。

---

## 三、问题清单

| # | 严重程度 | 位置 | 问题描述 | 建议处理方式 |
|---|---------|------|---------|------------|
| **B-1** | **BLOCKER 🚨** | `core.md §1.2` + `§7.1` | `GetPath` 同时出现在必需的 `Tool` 接口和可选的 `PathTool` 子接口，形成矛盾，会导致 Agent-Tools 实现混乱 | 从 `Tool` 接口移除 `GetPath`，仅保留在 `PathTool` 子接口（类型断言） |
| **M-1** | **MAJOR 🔴** | `core.md §1.2` `UseContext.Ctx` | `context.Context` 嵌入 struct 违反 Go 惯用法，cancel 传播语义不明，`ContextModifier` 并发修改 `*UseContext` 时 `Ctx` 字段行为未定义 | 推荐：`Call(ctx context.Context, input Input, uctx *UseContext, ...)` 将 context 提为首参，从 `UseContext` 移除 `Ctx` |
| **M-2** | **MAJOR 🔴** | `core.md §1.2` `tool.Message` | `types/message` 包未定义，`tool.Message` 与其关系不明，影响 agentic loop 关键路径 | 明确以 `tool.Message` 为规范类型，或发布 `types/message` 包设计文档 |
| m-1 | MINOR 🟡 | `core.md §1.2` `InterruptBehavior()` | 返回 `string`，无类型约束，拼写错误在编译期无法发现 | 新增 `type InterruptBehavior string` + 常量枚举（已在第一节正式契约中体现） |
| m-2 | MINOR 🟡 | `core.md §2.7` `BudgetTracker` | 所有字段为导出字段，并发修改可能引入数据竞争 | 实现时改为非导出字段 + 线程安全方法 + `sync.Mutex` |
| m-3 | MINOR 🟡 | `core.md §5.3` `AggregatedHookResult` | `PermissionBehavior` 字段类型为 `string`，与系统其余部分使用 `tool.PermissionBehavior` 不一致 | 改为 `tool.PermissionBehavior` |
| m-4 | MINOR 🟢 | `core.md §2.3` `EngineMsg` | 封闭接口的破坏性变更约束（新增消息类型须更新 TUI switch）未在文档中明确 | 在 `msg.go` 和设计文档中补充约束说明 |
| m-5 | MINOR 🟢 | `core.md §6.2` `Coordinator` | Swarm 广播部分失败场景未建模，`WaitAll` 聚合错误语义未定义 | 定义 `SwarmError` 类型；明确 `WaitAll` 返回语义 |
| m-6 | MINOR 🟢 | `core.md §4.4` `IMAGE_MAX_TOKEN_SIZE` | 命名暗示仅图片，实际适用于多种工具输出，是 TS 命名遗留 | Go 实现中重命名为 `MicroCompactToolResultMaxTokens` |

---

## 四、通过条件

在以下 **3 个必须修复项**完成并更新 `core.md` 后，本次评审升级为**完全通过**，`tool.Tool` 接口正式冻结：

**必修项 B-1（Impact：Agent-Tools）**：从 `tool.Tool` 接口移除 `GetPath`，仅保留在 `PathTool` 可选子接口中。更新接口定义文档和接口速查表。

**必修项 M-1（Impact：Agent-Tools、Agent-Core 引擎层）**：明确 `context.Context` 在 `Call` 中的传递方式。推荐在 `Call` 签名中增加 `ctx context.Context` 首参，同步更新 `UseContext`；若选择保留 `UseContext.Ctx` 方案，须在文档中明确说明：`UseContext.Ctx` 是工具执行的规范取消 context，工具不得在 `Call` 返回后持有 `*UseContext`。

**必修项 M-2（Impact：Agent-Tools、Agent-Engine）**：明确 `tool.Message` 是规范类型（删除对 `types/message` 包的引用），或在 7 天内发布 `types/message` 包的设计文档并说明映射关系。

---

## 五、架构一致性确认

本次评审确认以下架构约束得到遵守：

1. **严格单向依赖**：`internal/tool` 包零外部依赖（不引用 `engine`、`permissions`、`tui`），循环依赖风险为零
2. **Tool 接口位置**：定义于 `internal/tool/tool.go`，Agent-Tools 通过 import 此包实现接口，符合架构图"Agent-Tools → Agent-Core"的依赖方向
3. **渲染逻辑分离**：`Tool` 接口无任何渲染方法，UI 展示由 TUI 层按工具名分发至独立渲染器
4. **UseContext 传值语义**：引擎每次调用构造新 `UseContext`，工具不应长持引用
5. **MCP 工具统一处理**：MCP 工具实现 `tool.Tool` 接口，通过 `MCPToolInfo` 可选子接口区分，引擎统一调度
6. **包初始化顺序**：`tool` → `permissions/hooks/compact` → `engine` → `coordinator`，依赖方向唯一，无逆向依赖

---

## 六、实现注意事项（Agent-Core）

1. **`partitionToolCalls` 须有表驱动单元测试**：设计文档示例 `[Read, Read, Write, Read, Write]` → 四批次，应作为第一个引擎测试用例
2. **权限 ask 超时**：必须用 `context.WithTimeout` 实现（见 §2.3），而非 `time.Sleep`，确保 Ctrl-C 能级联取消等待
3. **Auto-compact 连续失败计数器**：须在成功压缩后重置为 0，文档未明确此细节
4. **`Result.ContextModifier` 防护**：在 `orchestrator.go` 中断言并发安全工具的 `ContextModifier` 为 nil，违反时记录 warning 并丢弃（防数据竞争）
5. **MCP 工具须完整实现 `tool.Tool`**：不得绕过接口直接调用，以确保经过相同的权限/hooks/并发管道

---

## 七、最终结论与行动指令

### 对 Agent-Tools

**条件开始编码**。当前接口（含 MINOR m-1 改动）已足够开始实现只读工具：

- **优先级 P0（立即可开始）**：`Read`、`Glob`、`Grep`、`WebFetch`、`WebSearch`
  — 只读工具，`CheckPermissions` 返回 `PermissionPassthrough`，`ContextModifier` 为 nil，不依赖 M-1/M-2/B-1 修复
- **优先级 P1（等待 B-1 + M-1 + M-2 完成）**：`Write`、`Edit`、`Bash`（文件路径工具需 `PathTool` 子接口确定）
- **优先级 P2（等待 `coordinator` 就绪）**：`Agent`、`SendMessage`、任务系列

`tools.md` 中的简化接口预期（8 方法）**已被本次评审的正式契约替代**，请以本文档第一节为准。

### 对 Agent-Core

1. 完成 B-1、M-1、M-2 三个必修项后，更新 `core.md` 并在项目频道发布**"Tool 接口冻结"通知**
2. **接口冻结后的变更须经 Tech Lead 审批**，并提前至少 1 个工作日通知 Agent-Tools
3. 建议在 `internal/tool` 包内补充 `README.md`，链接本评审报告，说明接口冻结状态与变更流程

---

*评审人：Tech Lead | 2026-04-02*
