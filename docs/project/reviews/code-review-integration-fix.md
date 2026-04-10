# Code Review — Integration Fix

> **Reviewer**: Tech Lead
> **Date**: 2026-04-10
> **Subject**: Unknown-tool ToolResult emit（orchestration.go）& TestE2E_ToolDispatch TurnComplete 断言修复（engine_e2e_test.go）
> **Verdict**: **APPROVED** — 两处修改均正确，无 P0/P1 问题，可直接合并

---

## 1. 评审范围

| 文件 | 变更描述 |
|------|---------|
| `internal/engine/orchestration.go` | `executeOneTool` 的 unknown-tool 早返回路径新增 `MsgTypeToolResult` emit |
| `test/integration/engine_e2e_test.go` | `TestE2E_ToolDispatch` 改用 `filterMsgs` 取最后一个 `MsgTypeTurnComplete` |

评审基础：
- `internal/engine/msg.go` — MsgType 常量定义
- `internal/engine/query.go` — `runQueryLoop`（TurnComplete emit 时机）、`streamResponse`（事件流处理）
- `internal/engine/engine.go` — `engineImpl` 结构与并发模型
- `internal/engine/orchestration_test.go` — 单元测试中的 stubTool 定义
- `docs/project/reviews/code-review-core.md` — 上一轮 Core 层评审结论

---

## 2. Design vs Implementation Delta

本次修改属于**局部行为修复**，不涉及接口契约或设计文档变更。以下仅记录修改前后的行为差异：

| 位置 | 修改前 | 修改后 | 差距影响 |
|------|--------|--------|---------|
| `executeOneTool` unknown-tool 路径 | 静默返回 error ContentBlock，不发任何 Msg | 先 emit `MsgTypeToolResult{IsError:true}` 再返回 ContentBlock | 消除了测试（及消费方）无法观测 unknown-tool 结果的盲区 |
| `TestE2E_ToolDispatch` TurnComplete 断言 | `findMsg`（取第一个，StopReason="tool_use"，断言 `!= "end_turn"` 必然失败） | `filterMsgs`+取最后一个，StopReason="end_turn" | 修复了一个隐性失败的断言 |

---

## 3. 逐项评审

### 3.1 评审要点 1：`orchestration.go` 修改与已知工具路径（279–289 行）的 emit 逻辑是否对称

**结论：✅ 完全对称**

已知工具路径（`executeOneTool` 第 290–301 行）的 emit 结构：
```go
select {
case msgCh <- Msg{
    Type: MsgTypeToolResult,
    ToolResult: &ToolResultMsg{
        ToolUseID: tc.id,
        Content:   contentStr,
        IsError:   isError,
    },
}:
case <-ctx.Done():
}
```

新增的 unknown-tool 路径（第 211–221 行）：
```go
select {
case msgCh <- Msg{
    Type: MsgTypeToolResult,
    ToolResult: &ToolResultMsg{
        ToolUseID: tc.id,
        Content:   errMsg,
        IsError:   true,
    },
}:
case <-ctx.Done():
}
```

两者在以下维度完全一致：
- `select/case/ctx.Done()` 双臂结构
- `MsgType` 均为 `MsgTypeToolResult`
- `ToolResult` 字段填充方式（`*ToolResultMsg`，含 `ToolUseID`、`Content`、`IsError`）
- 均不使用 `default:` 臂（ToolResult 属于必须送达的关键事件，与 `onProgress` 的 `default:` non-blocking 模式的区分是正确的）

`ContentBlock` 返回值的构造方式也对称：unknown-tool 使用 `strPtr(errMsg)` 与已知工具路径的 `&contentStr` 形式等价（均产生堆分配字符串指针，Go escape analysis 正确处理）。

---

### 3.2 评审要点 2：select/case msgCh 是否存在竞态或 channel 阻塞风险

**结论：✅ 无新增并发风险**

**channel 阻塞分析：**

`msgCh` 由 `query.go` 的 `Query()` 创建，缓冲为 `msgBufSize()`（默认 256，可通过环境变量覆盖）。`executeOneTool` 可能从并发 goroutine 中调用（`executeConcurrentBatch`），`DefaultMaxToolUseConcurrency = 10`。

- 极端情形：10 个并发工具同时 emit `MsgTypeToolResult`，需要 10 个 slot。256 的缓冲在正常消费速度下完全充裕。
- 即使缓冲满载，`case <-ctx.Done()` 提供了防止永久阻塞的安全阀。此行为与已知工具路径完全一致，不引入新风险。

**竞态分析：**

Go channel 的发送操作本身是 goroutine 安全的；多个 goroutine 并发 send 到同一 `chan<- Msg` 不存在数据竞争。`-race` 标志下运行不会产生新的 race 报告。

**与已有 emit 模式的对比：**

| emit 位置 | 阻塞策略 | 说明 |
|---------|---------|------|
| `onProgress` 回调 | `default:` non-blocking | 进度事件可丢弃 |
| `executeOneTool` 已知工具 ToolResult | `case <-ctx.Done():` | 结果必须送达 |
| `executeOneTool` unknown-tool ToolResult（新增） | `case <-ctx.Done():` | 与已知工具路径策略一致 ✅ |
| `streamResponse` 中所有 Msg emit | `case <-ctx.Done(): return ctx.Err()` | 返回错误，终止流 |
| `runQueryLoop` 中 TurnComplete 等 | `case <-ctx.Done(): return` | 终止循环 |

新增代码选择了与"结果必须送达"语义一致的阻塞策略，完全正确。

**一处细节（不影响正确性）：**

`executeOneTool` 中的 unknown-tool 路径在 ctx 取消时**静默跳过 emit**（`case <-ctx.Done():` 不返回错误，而是继续 `return types.ContentBlock{...}, nil`）。这意味着即使 ctx 已取消，函数仍返回一个有效的 error ContentBlock。调用方 `runTools` → `runQueryLoop` 会在下一轮循环入口的 `select { case <-ctx.Done(): return }` 中捕获取消，不影响正确性。这是现有架构的一贯模式。

---

### 3.3 评审要点 3：测试修改语义——filterMsgs 取最后一个是否符合引擎行为

**结论：✅ 语义正确，修复了原本必然失败的断言**

**引擎行为确认：**

查阅 `query.go` 第 172–183 行，`MsgTypeTurnComplete` 在 `runQueryLoop` 的**每次 LLM API 调用**后无条件 emit：
```go
select {
case msgCh <- Msg{
    Type:       MsgTypeTurnComplete,
    StopReason: stopReason,
    ...
}:
```

`TestE2E_ToolDispatch` 的两轮调用序列：
1. 第 1 轮：LLM 返回 `stop_reason=tool_use` → emit `MsgTypeTurnComplete{StopReason: "tool_use"}` → 执行工具 → continue
2. 第 2 轮：LLM 返回 `stop_reason=end_turn` → emit `MsgTypeTurnComplete{StopReason: "end_turn"}` → return

因此 `msgs` 中包含 **2 个** `MsgTypeTurnComplete`。`filterMsgs` 收集全部 TurnComplete，取最后一个（StopReason="end_turn"）断言是正确的。

**修改前的问题：**

若修改前使用 `findMsg`（取第一个），将得到 StopReason="tool_use"，断言 `tc.StopReason != "end_turn"` 必然触发 `t.Errorf`，测试在有工具调用时无法通过。

**与其他测试的一致性：**

| 测试 | 调用轮次 | TurnComplete 取法 | 断言 StopReason | 正确性 |
|------|---------|-----------------|----------------|-------|
| `TestE2E_FullConversationRoundTrip` | 1 轮（直接 end_turn） | `findMsg`（取第一个） | "end_turn" | ✅ 单轮时两种取法等价 |
| `TestE2E_ToolDispatch`（修改后） | 2 轮 | `filterMsgs` + 取最后一个 | "end_turn" | ✅ 正确 |
| `TestE2E_UnknownTool_GracefulRecovery` | 2 轮 | `findMsg`（取第一个） | 不断言 StopReason | ✅ 仅验证存在性，不检查 StopReason，语义上可接受 |

代码注释（第 476–477 行）明确解释了为何需要 `filterMsgs`：
```go
// Final TurnComplete — take the last one, as the engine emits one per LLM
// call (the first has stopReason="tool_use"; the last has "end_turn").
```
注释准确、完整。✅

---

### 3.4 评审要点 4：整体变更是否符合项目编码规范

**GoDoc / 注释：**
- `executeOneTool` 已有函数级 GoDoc（"runs a single tool call and emits the appropriate Msg events"），新增代码的行内注释（"Unknown tool — emit a ToolResult event so callers/tests can observe the error, then return an error result block so the LLM can recover."）清晰说明了意图。✅
- 测试函数注释精确描述了新测试模式的原因。✅

**错误处理：**
- unknown-tool 路径返回 `nil` error（而非 Go error），依靠 ContentBlock 中的 `IsError: &trueVal` 将错误信息回传给 LLM。这与"未知工具不应终止 Agent 循环"的设计意图一致。✅
- 不存在 panic 或错误静默吞咽。✅

**函数长度：**
- `executeOneTool` 从约 90 行增至约 107 行，超过 CLAUDE.md 规定的 80 行软限制。但增量（~17 行）是结构完整的 unknown-tool 处理块，逻辑自洽；强制拆分反而降低可读性。建议后续重构时考虑将 unknown-tool 分支提取为 `executeUnknownTool` 辅助函数（P2 建议）。
- 测试文件中 `TestE2E_ToolDispatch` 约 113 行，测试函数软限制 160 行，在限制内。✅

**命名规范：**
- `trueVal`、`errMsg` 命名符合 camelCase 约定。✅
- 测试中 `tcMsgs`（TurnComplete messages）命名清晰。✅

**Import 分组：**
- `orchestration.go` 的 import 分组（stdlib → 无三方依赖 → internal）符合规范。✅
- 测试文件 import 分组正确。✅

---

## 4. Issues

### P0 — Must Fix Before Merge

*无*

---

### P1 — Should Fix Soon

*无*

---

### P2 — Minor / Suggestions

#### P2-1：`executeOneTool` 函数长度超过 80 行软限制

**文件**：`internal/engine/orchestration.go`

修改后函数约 107 行，超过 CLAUDE.md 规定的 80 行软限制（约 34%）。当前结构为"unknown-tool 早返回块 + 校验块 + 执行块 + emit 块"，逻辑自洽但稍显冗长。

**建议**（非阻塞）：考虑在后续重构 Pass 中将 unknown-tool 路径（第 207–229 行）提取为 `emitUnknownToolError(ctx, tc, msgCh)` 辅助函数，函数逻辑更一目了然。

---

#### P2-2：`TestE2E_ToolDispatch` 未断言 TurnComplete 数量

**文件**：`test/integration/engine_e2e_test.go`

当前只验证"最后一个 TurnComplete 的 StopReason 是 end_turn"，但未验证共有 2 个 TurnComplete（一个 tool_use，一个 end_turn）。若引擎将来意外合并或跳过某个 TurnComplete emit，此测试不会报告回归。

**建议**（非阻塞）：在 `filterMsgs` 之后增加数量断言：
```go
if len(tcMsgs) != 2 {
    t.Errorf("expected 2 MsgTypeTurnComplete events (tool_use + end_turn), got %d", len(tcMsgs))
}
```

---

#### P2-3：`TestE2E_UnknownTool_GracefulRecovery` 的 TurnComplete 断言模式与 `TestE2E_ToolDispatch` 不一致

**文件**：`test/integration/engine_e2e_test.go`（第 1219–1221 行）

`TestE2E_UnknownTool_GracefulRecovery` 同样是两轮调用场景，但使用 `findMsg`（取第一个 TurnComplete）且不断言 StopReason，语义上仅验证"至少有一个 TurnComplete"，测试目的是确认引擎不崩溃——这是合理的。但与 `TestE2E_ToolDispatch` 的新注释相比，读者可能困惑为何同类场景用法不同。

**建议**（非阻塞）：在 `TestE2E_UnknownTool_GracefulRecovery` 的 TurnComplete 断言处添加简短注释，说明此处仅需验证存在性（不验证最终 StopReason），以区分两种用法的意图差异。

---

#### P2-4：`orchestration_test.go` 中 stubTool.InterruptBehavior() 与 BaseTool 默认值不一致

**文件**：`internal/engine/orchestration_test.go`（第 31 行）

单元测试 stub 显式返回 `InterruptBehaviorCancel`，而上一轮 P0-CR-2 修复已将 `BaseTool.InterruptBehavior()` 默认值改为 `InterruptBehaviorBlock`。当前测试场景均不测试中断行为，所以这不影响测试正确性，但日后若增加中断相关测试且继承此 stub，可能引发混淆。

**建议**（非阻塞）：将 `orchestration_test.go` 中 stubTool 的 `InterruptBehavior()` 改为返回 `InterruptBehaviorBlock`，与生产代码的默认值保持一致。（注：`engine_e2e_test.go` 中 stubTool 的同字段也返回 `Cancel`，同上建议。）

---

## 5. Summary

| 优先级 | 数量 | 说明 |
|--------|------|------|
| P0（本次发现） | 0 | 无 |
| P1（本次发现） | 0 | 无 |
| P2（本次发现） | 4 | 全部为非阻塞建议 |

**整体结论**：

两处修改质量良好：

1. **`orchestration.go`**：unknown-tool emit 路径在结构、并发安全性、语义三个维度上与已知工具路径完全对称，正确填补了之前的观测盲区（消费方无法通过 Msg channel 观察到 unknown-tool 错误）。`select/case ctx.Done()` 策略选择合理。

2. **`engine_e2e_test.go`**：`filterMsgs` + 取最后一个的模式准确反映了引擎"每次 LLM 调用 emit 一个 TurnComplete"的行为，修复了原来 `findMsg` 在多轮场景下必然取到 StopReason="tool_use" 导致断言失败的问题。注释清晰，逻辑无误。

**合并建议**：可直接合并。P2 建议作为技术债记录，在下次 engine 层 refactor Pass 中统一处理。

---

*Report generated by Tech Lead review pass — 2026-04-10*
