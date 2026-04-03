# Core Layer Code Review

> **Reviewer**: Tech Lead
> **Date**: 2026-04-03
> **Subject**: 任务 #15 实现代码（Agent-Core）
> **Verdict**: APPROVED_WITH_CHANGES

---

## 1. Overall Assessment

核心层的整体架构是健壮的，主要组件（`internal/engine`、`internal/compact`、`internal/permissions`、`internal/tool`、`internal/hooks`、`internal/commands`）均已落地，模块边界清晰，关键并发安全问题（P0-B abortFn 竞态、P0-H MicroCompactor 接口不匹配）已在本次评审前修复并得到确认。代码可读性好，注释完整，部分实现（如 `tool.Registry`、`BudgetTracker`、`permissions.checker`）质量达到或超过设计规格。

然而，评审发现 **6 个 P0 级别缺陷**，若不修复将导致 Agent 对话历史丢失、工具中断行为不符预期、TUI 渲染错误、上下文无限增长等严重问题。此外存在若干 P1 接口偏差，使实现与设计规格产生较大结构性差距。

**合并建议**：修复所有 P0 后可合并；P1/P2 可作为后续 task 跟进。

---

## 2. Design vs Implementation Delta

| 模块 | 设计规格 | 实现现状 | 差距级别 |
|------|---------|---------|---------|
| `tool.Tool` 接口 | 30+ 方法（含 `IsOpenWorld`、`RequiresUserInteraction`、`IsLSP`、`ShouldDefer`、`AlwaysLoad`、`Strict`、`InputsEquivalent`、`BackfillObservableInput` 等） | ~17 方法，`Execute()` 改名为 `Call()`，`Description` 签名不同 | P1 |
| `tool.UseContext` | 15+ 字段（Options、AbortCh、Messages、AgentID、AgentType、QueryTracking、PermissionCtx、OnProgress、SetStreamMode、FileReadLimits、GlobLimits、ToolDecisions、ToolUseID 等） | 3 个字段 | P1 |
| `Compressor.Compact` | `Compact(ctx, messages []tool.Message, tctx *tool.UseContext, params CompactionParams)` (4 参数) | `Compact(ctx, messages []types.Message, params CompactionParams)` (3 参数，缺少 tctx) | P1 |
| `BaseTool.InterruptBehavior` | 默认值 `InterruptBlock` | 实现返回 `InterruptBehaviorCancel` | **P0** |
| 查询循环压缩流水线 | 每轮开始时依次执行 snip → micro → auto compaction | 完全缺失；压缩器已实现但从未调用 | **P0** |
| `hooks.Executor` 接口 | 定义接口 `Executor`，含 `RunHooks`、`RegisterCallback`、`RegisterCommand` | 只有具体结构体 `Dispatcher`，无接口抽象；方法签名不同 | P1 |
| `AskResponse.Updates` | `Updates []tool.PermissionUpdate` | 字段缺失 | P1 |
| `permissions.checker` 类型依赖 | 使用 `tool.PermissionContext` | 使用 `types.ToolPermissionContext` | P1 |
| `engine` 消息历史持久化 | `GetMessages()` 返回本次查询后的最新历史 | `e.messages` 从未在 `runQueryLoop` 后写回 | **P0** |
| `SnipCompactor` | 实现 `Compressor` 接口 | 独立函数 `SnipCompactIfNeeded`，不实现接口 | P2 |
| `Msg.Progress` 类型 | `*tool.Progress` | `any` | P2 |
| `QueryEngine.SetModel` | 设计规格未提及 | 接口中存在 | P2 |

---

## 3. Strengths

1. **并发安全基础扎实**：`tool.Registry` 使用 `sync.RWMutex` 实现读写分离；`engineImpl.abortMu` 保护 `abortFn` 读写；工具并发批次使用 semaphore 模式（`chan struct{}`）正确限制并发度。

2. **预算追踪器完整**：`BudgetTracker` 中 `CompletionThreshold(0.90)`、`DiminishingThreshold(500)`、`MaxContinuations(3)` 等常量与设计规格一致，nudge 消息注入逻辑正确。

3. **权限决策流水线顺序正确**：`checker.go` 中 bypass → deny rules → validateInput → hook PreToolUse → allow rules → ask rules → mode → tool-specific → default 的 9 步顺序完全符合设计规格。

4. **AutoCompactor 电路断路器**：`ConsecutiveFailures >= MaxConsecutiveFailures` 后停止尝试压缩，防止反复失败拖慢主循环；`CLAUDE_CODE_AUTO_COMPACT_WINDOW` 环境变量覆盖机制便于调试。

5. **MicroCompactor 浅拷贝策略**：`compact` 方法对消息列表做浅拷贝，只在有修改时替换 Content 切片，内存效率高。

6. **工具批次分区设计合理**：`partitionToolCalls` 将 `IsConcurrencySafe` 的工具分为并发批次，其余串行执行，与设计规格完全一致。

7. **子接口设计（额外加分）**：`PathTool`、`SearchOrReadTool`、`MCPToolInfo` 三个子接口是设计规格之外的合理延伸，有助于类型断言和功能分组。

---

## 4. Issues

### P0 — Must Fix Before Merge

#### P0-1：`e.messages` 从不写回 —— `GetMessages()` 永远返回过期数据

**文件**：`internal/engine/engine.go`，`internal/engine/query.go`

**问题**：`runQueryLoop` 在函数开始时将 `params.Messages` 复制到局部变量 `messages`，循环中所有 `append` 操作都针对该局部变量。函数返回后，`e.messages` 保持在查询开始前的状态。调用 `GetMessages()` 或 `SetMessages()` 后再次调用 `GetMessages()` 均得到错误结果。

```go
// query.go — runQueryLoop 中 messages 是局部变量，从未写回 e.messages
messages := make([]types.Message, len(params.Messages))
copy(messages, params.Messages)
// ... 所有 append 均操作 messages，e.messages 保持不变
```

**修复方向**：在 `runQueryLoop` 返回前将最终 `messages` 写回 `e.messages`，或通过回调/通道通知外层更新。需注意 goroutine 并发访问，应加锁或明确内存所有权。

---

#### P0-2：`BaseTool.InterruptBehavior()` 返回错误默认值

**文件**：`internal/tool/base.go`

**问题**：设计规格明确要求工具的默认中断行为是 `InterruptBlock`（等待当前操作完成后再中断），但实现返回 `InterruptBehaviorCancel`（立即取消）。这意味着所有未覆盖此方法的工具在收到 Ctrl+C 时将被立即中断，可能导致文件写入、网络请求等操作被强制截断，产生数据不一致。

```go
// base.go — 错误的默认值
func (b BaseTool) InterruptBehavior() InterruptBehavior {
    return InterruptBehaviorCancel  // 应为 InterruptBehaviorBlock (或对应 InterruptBlock)
}
```

**修复方向**：将返回值改为 `InterruptBehaviorBlock`（或按实际常量命名调整）。

---

#### P0-3：`input_json_delta` 发送错误消息类型

**文件**：`internal/engine/query.go`

**问题**：在处理 `input_json_delta` 流事件时，代码发送 `MsgTypeToolUseStart` 类型的消息，这与处理 `content_block_start` 时发送的消息类型相同。TUI 层将收到同一工具的多个"开始"事件，导致重复渲染或状态机错误。应发送专用的增量更新类型（如 `MsgTypeToolUseInputDelta`）。

```go
// query.go — 错误的消息类型
case "input_json_delta":
    if id, ok := activeToolUseIDs[d.Index]; ok {
        select {
        case msgCh <- Msg{
            Type:      MsgTypeToolUseStart,  // BUG: 应为 MsgTypeToolUseInputDelta 或类似类型
            ToolUseID: id,
            ToolName:  activeToolNames[d.Index],
            InputDelta: d.Delta.PartialJSON,
        }:
```

**修复方向**：在 `msg.go` 中新增 `MsgTypeToolUseInputDelta` 常量，在此处使用该类型，并更新 TUI 消费端。

---

#### P0-4：`buildRequest` 以 `nil` 调用 `Description(nil, nil)`

**文件**：`internal/engine/query.go`

**问题**：构建 API 请求时，对每个已注册工具调用 `t.Description(nil, nil)`，将 nil 传入可能对 input/permCtx 参数有解引用操作的工具实现，导致 nil pointer panic。即使当前工具实现均做了 nil 守卫，这也是对接口契约的违反，未来任何新工具都可能因此崩溃。

```go
// query.go — 危险的 nil 调用
toolSchemas = append(toolSchemas, api.ToolSchema{
    Name:        t.Name(),
    Description: t.Description(nil, nil),  // BUG: 传入 nil
    InputSchema: schemaRaw,
})
```

**修复方向**：传入合法的 `PermissionContext`（可以是空值结构体而非 nil 指针），或调整 `Description` 接口签名使参数为可选。

---

#### P0-5：`FallbackModel` 无条件替换主模型

**文件**：`internal/engine/query.go`

**问题**：`buildRequest` 中，只要 `params.FallbackModel != ""`，就会使用 fallback 模型替换主模型，完全无视主模型是否已失败。这意味着设置了 `FallbackModel` 的调用方从第一轮起就使用 fallback 模型，主模型永远不会被使用。

```go
// query.go — 错误的 fallback 逻辑
model := e.model
if params.FallbackModel != "" {
    model = params.FallbackModel  // BUG: 应仅在主模型失败（context limit 等）时切换
}
```

**修复方向**：将 `FallbackModel` 作为主模型因上下文超限（HTTP 400 / 特定错误码）失败后的重试模型，而非在第一次请求就直接替换。

---

#### P0-6：查询循环完全缺失压缩流水线

**文件**：`internal/engine/query.go`

**问题**：设计规格要求在每轮 LLM 请求前依次检查并执行 snip → micro → auto compaction，以防止上下文窗口溢出。当前 `runQueryLoop` 中没有任何压缩调用。`AutoCompactor`、`MicroCompactor`、`SnipCompactIfNeeded` 均已实现但处于"死代码"状态，上下文会无限增长直至 API 报错。

```go
// query.go — runQueryLoop 中完全没有压缩逻辑
for {
    // 缺失：snip compaction 检查
    // 缺失：micro compaction 检查
    // 缺失：auto compaction 检查
    req, err := e.buildRequest(params, messages)
    // ...
}
```

**修复方向**：在 `buildRequest` 调用前，按设计规格顺序插入三层压缩检查。Auto compaction 需要异步 LLM 调用，需注意不要阻塞主循环过久（可考虑在 context 窗口阈值触发时执行）。

---

### P1 — Should Fix Soon

#### P1-1：`tool.Tool` 接口缺少 13+ 设计方法

**文件**：`internal/tool/tool.go`

设计规格定义了 30+ 方法的完整接口，当前实现约 17 个，缺失方法包括：`IsOpenWorld`、`RequiresUserInteraction`、`IsLSP`、`ShouldDefer`、`AlwaysLoad`、`Strict`、`InputsEquivalent`、`BackfillObservableInput`。此外 `Execute()` 改名为 `Call()`，`Description` 和 `ValidateInput` 签名不同。随着工具实现增加，这些方法的缺失将阻碍高级功能（延迟执行、LSP 集成、幂等检测等）的落地。

**修复方向**：参照设计规格补全接口方法，在 `BaseTool` 中提供合理的默认实现，避免破坏现有工具实现。

---

#### P1-2：`tool.UseContext` 字段严重不足

**文件**：`internal/tool/tool.go`

当前 `UseContext` 只有 3 个字段，设计规格要求 15+。缺失字段包括 `Options`、`AbortCh`、`Messages`（对话历史访问）、`AgentID`、`AgentType`、`QueryTracking`、`PermissionCtx`、`OnProgress`、`SetStreamMode`、`FileReadLimits`、`GlobLimits`、`ToolDecisions`、`ToolUseID`。工具无法访问这些上下文将严重限制实现复杂工具行为（如进度上报、资源限制、流式输出切换等）。

---

#### P1-3：`Compressor.Compact` 缺少 `tctx *tool.UseContext` 参数

**文件**：`internal/compact/compact.go`

设计规格的 `Compact` 签名为 4 参数（含 `tctx`），当前实现为 3 参数。Auto compaction 在构建 LLM 摘要请求时需要访问工具使用上下文（如 Agent 类型、abort 信号等），缺少此参数将限制 Auto compaction 的能力。

---

#### P1-4：`hooks` 包缺少 `Executor` 接口抽象

**文件**：`internal/hooks/hooks.go`

设计规格要求 `hooks.Executor` 接口支持注入 mock 实现用于测试。当前只有具体结构体 `Dispatcher`，方法签名 `Run(ctx, hookType, input map[string]any)` 与设计的 `RunHooks(ctx, event, HookInput, toolUseID)` 不同，且缺少 `RegisterCallback`、`RegisterCommand` 方法。

---

#### P1-5：`AskResponse` 缺少 `Updates` 字段

**文件**：`internal/permissions/ask.go`

设计规格要求 `AskResponse.Updates []tool.PermissionUpdate` 字段用于传递用户批准后的权限更新集合（如"永久允许此工具"）。缺少此字段导致权限持久化无法通过 ask 流程实现。

---

#### P1-6：`permissions.checker` 跨包耦合

**文件**：`internal/permissions/checker.go`

`checker` 直接使用 `types.ToolPermissionContext`（来自 `pkg/types`）而非 `tool.PermissionContext`（来自 `internal/tool`），打破了层间边界。`pkg/types` 应该是最底层无业务逻辑的类型包，权限上下文的业务语义应在 `internal/tool` 层定义。

---

### P2 — Minor / Suggestions

#### P2-1：`InterruptBehavior` 常量命名与设计规格不一致

**文件**：`internal/tool/tool.go`，`internal/tool/base.go`

实现使用 `InterruptBehaviorCancel`/`InterruptBehaviorBlock`，设计规格使用 `InterruptCancel`/`InterruptBlock`。虽然功能等价，但会导致与设计文档、其他团队成员代码之间的混淆。建议统一命名。

---

#### P2-2：`strPtr` 辅助函数重复定义

**文件**：`internal/compact/auto.go`，`internal/engine/orchestration.go`

`strPtr` 函数在两处独立定义，建议提取到共享的 `internal/util` 包或 `internal/types` 包中。

---

#### P2-3：`commands.Result.NewMessages` 类型不安全

**文件**：`internal/commands/registry.go`

`Result.NewMessages` 使用 `interface{}` 类型以避免循环导入，TUI 层需要类型断言。这是一个脆弱的接口契约，建议用专用类型或回调函数替代。

---

#### P2-4：`SnipCompactor` 不实现 `Compressor` 接口

**文件**：`internal/compact/snip.go`

`SnipCompactIfNeeded` 是独立函数而非实现 `Compressor` 接口的结构体。若未来需要统一压缩流水线配置或替换策略，独立函数的形式会造成不一致。建议封装为实现 `Compressor` 的 `SnipCompactor` 结构体。

---

#### P2-5：并发批次中多个工具错误被丢弃

**文件**：`internal/engine/orchestration.go`

`executeConcurrentBatch` 在收集结果时只保留第一个非 nil 错误，其余工具的错误静默丢弃。建议使用 `errors.Join`（Go 1.20+）聚合所有错误，便于调试。

---

#### P2-6：`QueryEngine` 接口包含设计规格未定义的 `SetModel` 方法

**文件**：`internal/engine/engine.go`

`SetModel(model string)` 在接口中存在但设计规格未提及。若此方法确有必要，应补充到设计文档；否则建议移出公共接口，改为构造时配置。

---

#### P2-7：`Msg.Progress` 类型过于宽松

**文件**：`internal/engine/msg.go`

设计规格中 `Progress` 字段类型为 `*tool.Progress`，实现使用 `any`。TUI 层消费此字段时需要类型断言，失去编译期类型安全。建议改为具体类型。

---

## 5. Summary

| 优先级 | 数量 | 状态 |
|--------|------|------|
| P0（已知，修复前） | 2 | ✅ 已修复（P0-B, P0-H） |
| P0（本次发现） | 6 | ❌ 待修复 |
| P1 | 6 | ⚠️ 应尽快跟进 |
| P2 | 7 | 💡 建议改进 |

核心层已具备可运行的骨架，关键并发安全已修复，权限流水线、预算追踪、工具注册表等核心模块质量良好。**但在 P0-1（对话历史丢失）、P0-6（压缩流水线缺失）修复前，Agent 不具备生产级别的稳定性**——前者会导致 `GetMessages()` 返回错误数据使外层无法正确管理会话，后者会导致长对话因上下文溢出而崩溃。

建议本次迭代内优先修复 6 个 P0，P1 接口对齐工作可在工具层实现阶段同步推进，P2 作为技术债跟踪。

---

*Report generated by Tech Lead review pass — 2026-04-03*
