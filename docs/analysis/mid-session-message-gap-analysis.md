# 会话进行中消息处理架构差异分析报告

> **文档状态**：功能规划  
> **创建日期**：2026-04-27  
> **跟踪 Issue**：#TBD（待创建）  
> **相关文档**：[`mid-session-message-architecture.md`](./origin/mid-session-message-architecture.md)

---

## 概述

本文档基于对 Claude Code TypeScript 原版 Mid-Session Message 架构的深入分析（见 `origin/mid-session-message-architecture.md`），
与当前 Go 版本实现进行逐项对比，识别差距并规划改进路径。

原版 Mid-Session Message 架构包含 **6 大核心子系统**（统一命令队列、QueryGuard 并发守卫、Mid-Turn Drain 回合内注入、
Between-Turn Drain 回合间处理、Early Input 启动缓冲、中断与取消机制）和 **完整的优先级调度基础设施**。
当前 Go 版本采用**线性即时派发架构**，所有队列化、优先级调度、消息注入能力几乎全部缺失。

**整体评估**：约 **15-20%** 功能覆盖率。基础输入提交和取消机制可用，但统一队列、优先级调度、Mid-Turn Drain、队列预览 UI 等核心能力均未实现。

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

### 1.1 基础输入提交 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| TextArea 输入组件 | ✅ 完成 | `internal/tui/input.go` |
| Enter 提交处理 | ✅ 完成 | `internal/tui/keys.go:handleSubmit()` |
| 斜杠命令检测与路由 | ✅ 完成 | `internal/tui/keys.go` |
| Vim 模式支持（Insert/Normal/Visual） | ✅ 完成 | `internal/tui/input.go` |
| 输入后清空输入框 | ✅ 完成 | `internal/tui/keys.go` |

**当前提交流程**：
```go
// internal/tui/keys.go
func (m AppModel) handleSubmit() (tea.Model, tea.Cmd) {
    text := m.input.Value()
    if text == "" { return m, nil }
    m.input = m.input.SetValue("")    // 清空输入框
    if IsSlashCommand(text) {
        return m.handleSlashCommand(text)
    }
    m.messages = append(m.messages, newUserMessage(text))
    m.isLoading = true
    queryCmd := startQueryCmd(&m, text) // 立即派发
    return m, queryCmd
}
```

**与原版的关键差异**：Go 版本在 `handleSubmit()` 中**立即派发**查询，不经过队列判断。原版在此处先检查 `queryGuard.isActive`，决定是立即执行还是入队。

### 1.2 流式事件拉取 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| Engine 查询返回事件通道 | ✅ 完成 | `internal/engine/query.go` |
| BubbleTea 拉取事件循环 | ✅ 完成 | `internal/tui/cmds.go:waitForStreamEvent()` |
| 51+ 消息类型分发 | ✅ 完成 | `internal/tui/cmds.go:dispatchEngineMsg()` |
| 通道缓冲区（默认 256，可配置） | ✅ 完成 | `internal/engine/query.go` |

**事件通道架构**：
```go
// internal/engine/query.go
func (e *engineImpl) Query(ctx context.Context, params QueryParams) (<-chan Msg, error) {
    msgCh := make(chan Msg, msgBufSize()) // 默认 256
    go func() {
        defer close(msgCh)
        e.runQueryLoop(ctx, params, msgCh)
    }()
    return msgCh, nil
}
```

### 1.3 基础取消/中断机制 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| Ctrl+C 取消当前查询 | ✅ 完成 | `internal/tui/keys.go:handleKey()` |
| `context.WithCancel` 取消传播 | ✅ 完成 | `internal/tui/cmds.go:startQueryCmd()` |
| `engine.Interrupt()` 中断引擎 | ✅ 完成 | `internal/engine/engine.go` |
| 取消后清理流状态 | ✅ 完成 | `internal/tui/keys.go:doAbort()` |

**当前取消流程**：
```go
// internal/tui/keys.go
func (m AppModel) doAbort() (tea.Model, tea.Cmd) {
    abortCmd := abortQueryCmd(m.queryEngine)
    m.isLoading = false
    m.showSpinner = false
    m.abortFn = nil    // 清除 abort handle
    m.streamCh = nil   // 清除流通道（忽略过期事件）
    return m, abortCmd
}
```

**与原版的关键差异**：Go 版本通过 `context.Cancel()` 直接取消，没有队列化的取消指令。原版的 Escape 键有优先级处理逻辑（先取消运行中任务 → 再弹出队列消息 → 最后退出）。

### 1.4 权限请求阻塞对话框 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 权限请求通道（缓冲 1） | ✅ 完成 | `internal/bootstrap/wire.go` |
| 权限响应通道（缓冲 1） | ✅ 完成 | `internal/bootstrap/wire.go` |
| 阻塞式权限对话框 | ✅ 完成 | `internal/tui/update.go:PermissionRequestMsg` |
| RespFn 回调 | ✅ 完成 | `internal/tui/init.go:listenForPermissionRequest()` |

### 1.5 Agent 事件流 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| Agent 事件通道（缓冲 64） | ✅ 完成 | `internal/bootstrap/wire.go` |
| Agent 进度/状态更新 | ✅ 完成 | `internal/tui/update.go:AgentStatusMsg` |
| Coordinator 面板展示 | ✅ 完成 | `internal/tui/update.go` |
| 30 秒自动清除完成的 Agent | ✅ 完成 | `internal/tui/update.go:agentEvictDelay` |

### 1.6 Agent 消息投递（基础） ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| Agent 收件箱（固定 16 缓冲） | ✅ 完成 | `internal/coordinator/coordinator.go` |
| SendMessage 工具 | ✅ 完成 | `internal/tools/agent/sendmessage.go` |
| 非阻塞投递（满时报错） | ✅ 完成 | `internal/coordinator/coordinator.go:SendMessage()` |

### 1.7 Headless/Print 模式 ⚠️（基础可用）

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 单次查询执行 | ✅ 完成 | `internal/bootstrap/run.go:headlessRun()` |
| 信号取消（SIGINT/SIGTERM） | ✅ 完成 | `internal/bootstrap/run.go` |
| 三种输出格式（text/json/stream-json） | ✅ 完成 | `internal/bootstrap/run.go` |

---

## 二、未实现/不完整功能

### 2.1 ❌ 统一命令队列 — messageQueueManager（最核心架构缺失）

**原版实现**：

```typescript
// src/utils/messageQueueManager.ts — 模块级单例
const commandQueue: QueuedCommand[] = []
let snapshot: readonly QueuedCommand[] = Object.freeze([])
const queueChanged = createSignal()

// QueuedCommand 类型 — 丰富的元数据
type QueuedCommand = {
  value: string | Array<ContentBlockParam>  // 消息内容
  mode: PromptInputMode                     // prompt/bash/orphaned-permission/task-notification
  priority?: QueuePriority                  // now/next/later
  uuid?: UUID                              // 生命周期追踪
  pastedContents?: Record<number, PastedContent>  // 粘贴内容
  skipSlashCommands?: boolean               // 远程消息不触发本地斜杠命令
  bridgeOrigin?: boolean                    // 来自移动端/Web
  isMeta?: boolean                          // 系统消息（UI 隐藏）
  origin?: MessageOrigin                    // 来源标记
  agentId?: AgentId                         // 目标 Agent
}

// 入队
function enqueue(command: QueuedCommand): void {
  commandQueue.push({ ...command, priority: command.priority ?? 'next' })
  notifySubscribers()
}

// 出队 — 优先级最高的先出
function dequeue(filter?): QueuedCommand | undefined {
  // 找到优先级最高（PRIORITY_ORDER 最小值）的命令
  // 同优先级内 FIFO
}

// React 订阅 — useSyncExternalStore 同步订阅
export const subscribeToCommandQueue = queueChanged.subscribe
export function getCommandQueueSnapshot(): readonly QueuedCommand[] { return snapshot }
```

核心能力：
- **模块级单例**：独立于 React 状态树，非 React 代码可直接读写
- **三级优先级**：`now`（立即中断）> `next`（回合内注入）> `later`（回合间处理）
- **冻结快照**：`Object.freeze()` + 引用相等性，适配 `useSyncExternalStore`
- **丰富元数据**：UUID 生命周期追踪、来源标记、Agent 作用域、粘贴内容
- **过滤出队**：支持按 `agentId` 过滤，主线程和子 Agent 各取各的
- **批量查询**：`getCommandsByMaxPriority()` 按阈值查询
- **信号通知**：轻量级 pub-sub（`signal.ts`）

**当前 Go 实现**：
- **完全没有实现**
- 没有 `commandQueue` 数据结构
- 没有 `QueuedCommand` 类型
- 没有 `enqueue/dequeue` 操作
- 没有优先级概念
- 没有队列订阅机制
- 用户消息在 `handleSubmit()` 中立即派发，无缓冲

**差距分析**：
- 这是整个 Mid-Session Message 架构的**心脏**。没有统一命令队列，后续的所有高级功能（Mid-Turn Drain、Between-Turn Drain、优先级调度、队列预览 UI）都无从实现
- 当前架构下，如果用户在 AI 回复过程中输入新消息，要么消息被丢弃（输入框不可用），要么触发 abort 取消当前回复后重新开始

**建议优先级**：🔴 P0（所有高级功能的根本依赖）

**修复难度**：高（~300-400 行代码，核心架构组件）

---

### 2.2 ❌ QueryGuard — 并发守卫状态机

**原版实现**：

```typescript
// src/utils/QueryGuard.ts — 三态状态机
class QueryGuard {
  private _status: 'idle' | 'dispatching' | 'running' = 'idle'
  private _generation = 0  // 代次计数器

  reserve(): boolean {
    if (this._status !== 'idle') return false
    this._status = 'dispatching'  // 锁定同步间隙
    return true
  }

  cancelReservation(): void {
    if (this._status !== 'dispatching') return
    this._status = 'idle'
  }

  tryStart(): number | null {
    if (this._status === 'running') return null
    this._status = 'running'
    return ++this._generation
  }

  end(generation: number): boolean {
    if (this._generation !== generation) return false  // 防止过期 finally 干扰
    this._status = 'idle'
    return true
  }

  get isActive(): boolean { return this._status !== 'idle' }
}
```

核心设计：
- **三态**：`idle`（空闲）→ `dispatching`（派发中）→ `running`（运行中）
- **`dispatching` 中间态**：覆盖从出队到 `await executeUserInput()` 之间的同步间隙，防止竞态
- **代次（generation）计数器**：防止被取消的查询在 `finally` 中意外清除新查询的状态
- **React 订阅**：`queryGuard.subscribe` + `getSnapshot` 供 `useSyncExternalStore` 使用

**当前 Go 实现**：
- **完全没有实现**
- 使用 `m.isLoading` 布尔值 + `m.abortFn != nil` 判断查询活跃状态
- 没有中间状态保护
- 没有代次计数器防止竞态

```go
// internal/tui/cmds.go — 当前的查询守卫仅靠 context.Cancel
func startQueryCmd(m *AppModel, userText string) tea.Cmd {
    if m.abortFn != nil {
        m.abortFn()         // 直接取消上一个查询
        m.abortFn = nil
    }
    ctx, cancel := context.WithCancel(context.Background())
    m.abortCtx = ctx
    m.abortFn = cancel
    // ... 立即启动查询
}
```

**差距分析**：
- Go 版本在提交新查询时**直接取消**上一个查询，而不是排队等待
- 没有 `dispatching` 中间态保护，存在竞态风险：BubbleTea 的 `Update` 是同步的但 `Cmd` 是异步的
- 没有代次追踪，如果取消的 goroutine 延迟清理可能导致状态不一致
- BubbleTea 的 Elm 架构天然是单线程 Update，减轻了部分竞态问题，但 Cmd 返回的 Msg 仍然可能乱序

**建议优先级**：🔴 P1（依赖统一命令队列 P0）

**修复难度**：中（~150 行代码）

---

### 2.3 ❌ 三级优先级调度系统

**原版实现**：

```typescript
type QueuePriority = 'now' | 'next' | 'later'

const PRIORITY_ORDER: Record<QueuePriority, number> = {
  now: 0,    // 最高 — 立即中断流式传输
  next: 1,   // 中等 — 回合内工具间隙注入
  later: 2,  // 最低 — 回合结束后处理
}
```

各级别行为：

| 优先级 | 触发时机 | 行为 | 使用场景 |
|--------|---------|------|----------|
| `now` | 入队时立即 | `abort('interrupt')` 中断流式响应 | 紧急命令 |
| `next` | Mid-Turn Drain | 让当前工具完成，在下次 API 调用前注入 | 用户追加消息 |
| `later` | Between-Turn Drain | 等整个回合结束，作为新查询处理 | 任务通知（Agent 完成等） |

调度时序：
```
Turn 1 进行中...
  用户输入 A → enqueue(A, 'next')
  任务通知 X → enqueue(X, 'later')
  工具执行完成 → Mid-Turn Drain → 取出 A（next），注入为附件
  Turn 1 结束 → Between-Turn Drain → 取出 X（later），启动 Turn 2
```

**当前 Go 实现**：
- **完全没有实现**
- 没有 `QueuePriority` 类型
- 没有 `PRIORITY_ORDER` 映射
- 所有消息平等对待（没有调度差异）
- 用户消息和系统通知没有优先级区分

**差距影响**：
- 用户无法在 AI 工作时追加消息（除非取消当前回合）
- Agent 完成通知无法延迟到回合结束再处理
- 无法实现"让当前工具跑完再看用户新消息"的精细控制

**建议优先级**：🔴 P1（依赖统一命令队列 P0）

**修复难度**：中（~100 行代码，但与队列和 Drain 机制深度耦合）

---

### 2.4 ❌ Mid-Turn Drain — 回合内消息注入

**原版实现**：

```typescript
// src/query.ts — 在工具执行完成后、下一次 API 调用前的检查点

// ① 确定 drain 优先级阈值
const sleepRan = toolUseBlocks.some(b => b.name === SLEEP_TOOL_NAME)

// ② 获取符合条件的待处理命令快照（不移除）
const queuedCommandsSnapshot = getCommandsByMaxPriority(
  sleepRan ? 'later' : 'next',  // Sleep 后扩大到 later
).filter(cmd => {
  if (isSlashCommand(cmd)) return false            // 斜杠命令不在此处理
  if (isMainThread) return cmd.agentId === undefined  // 主线程只处理自己的
  return cmd.mode === 'task-notification' && cmd.agentId === currentAgentId
})

// ③ 将命令转换为附件消息，注入当前回合
for await (const attachment of getAttachmentMessages(
  null, updatedToolUseContext, null,
  queuedCommandsSnapshot,      // ← 传入快照
  [...messagesForQuery, ...assistantMessages, ...toolResults],
  querySource,
)) {
  yield attachment
  toolResults.push(attachment)
}

// ④ 从队列中移除已消费的命令
removeFromQueue(consumedCommands)
```

核心特性：
- 在 `query.ts` 工具执行循环的**每个工具完成后**设置检查点
- 将用户追加消息作为**附件（attachment）**注入当前回合上下文
- Claude 在**同一回合**中就能看到并响应用户的补充需求
- **Sleep Tool 特殊处理**：Sleep 后将 drain 阈值从 `next` 扩大到 `later`
- **Agent 作用域隔离**：通过 `agentId` 过滤，主线程和子 Agent 各取各的
- **斜杠命令排除**：斜杠命令需要完整的 `processSlashCommand` 路径

**当前 Go 实现**：
- **完全没有实现**
- `runQueryLoop()` 是线性执行，工具完成后直接进入下一轮 API 调用
- 没有在工具执行间隙检查队列的逻辑
- 没有附件消息注入机制

```go
// internal/engine/query.go — 当前的线性循环
func (e *engineImpl) runQueryLoop(ctx context.Context, params QueryParams, msgCh chan<- Msg) {
    for {
        select {
        case <-ctx.Done(): return
        default:
        }
        // 流式 LLM 响应...
        // 执行工具...
        // 直接进入下一轮，没有 drain 检查点
    }
}
```

**差距影响**：
- 这是原版最精巧的设计之一。没有 Mid-Turn Drain，用户在 AI 执行多步工具调用（可能持续数十秒到数分钟）的过程中**无法追加指令**
- 用户只能选择：等待整个回合结束，或者 Ctrl+C 取消重来
- 对于长时间运行的 agentic 任务，体验差距尤为明显

**建议优先级**：🔴 P1（核心交互体验差距）

**修复难度**：高（~200 行代码，需要修改引擎核心循环 + 附件注入机制）

---

### 2.5 ❌ Between-Turn Drain — 回合间队列处理

**原版实现**：

#### REPL 模式（交互式）

```typescript
// src/hooks/useQueueProcessor.ts — React Effect 自动触发
export function useQueueProcessor({ executeQueuedInput, hasActiveLocalJsxUI, queryGuard }) {
  const isQueryActive = useSyncExternalStore(queryGuard.subscribe, queryGuard.getSnapshot)
  const queueSnapshot = useSyncExternalStore(subscribeToCommandQueue, getCommandQueueSnapshot)

  useEffect(() => {
    if (isQueryActive) return           // 有查询在运行 → 等待
    if (hasActiveLocalJsxUI) return     // 有 JSX UI 在显示 → 等待
    if (queueSnapshot.length === 0) return  // 队列为空 → 无事可做
    processQueueIfReady({ executeInput: executeQueuedInput })
  }, [queueSnapshot, isQueryActive, executeQueuedInput, hasActiveLocalJsxUI, queryGuard])
}
```

#### 队列处理策略

```typescript
// src/utils/queueProcessor.ts
function processQueueIfReady({ executeInput }) {
  const next = peek(isMainThread)
  if (!next) return { processed: false }

  // 斜杠命令和 Bash 命令：逐条处理
  if (isSlashCommand(next) || next.mode === 'bash') {
    const cmd = dequeue(isMainThread)
    void executeInput([cmd])
    return { processed: true }
  }

  // 同模式普通命令：批量处理
  const commands = dequeueAllMatching(
    cmd => isMainThread(cmd) && !isSlashCommand(cmd) && cmd.mode === targetMode,
  )
  void executeInput(commands)
  return { processed: true }
}
```

#### Print 模式（非交互式）

```typescript
// src/cli/print.ts
const drainCommandQueue = async () => {
  while ((command = dequeue(isMainThread))) {
    const batch = [command]
    if (command.mode === 'prompt') {
      while (canBatchWith(command, peek(isMainThread))) {
        batch.push(dequeue(isMainThread))
      }
    }
    // 调用 ask() → query() 发起新的 API 调用
  }
}

do {
  for (const event of drainSdkEvents()) { output.enqueue(event) }
  await drainCommandQueue()
} while (waitingForAgents)
```

核心设计决策：
- **斜杠命令逐条处理**：可能修改队列
- **Bash 命令逐条处理**：独立错误隔离
- **同模式普通命令批量处理**：提高吞吐
- **触发条件三合一**：`!isQueryActive && !hasActiveLocalJsxUI && queueSnapshot.length > 0`

**当前 Go 实现**：
- **完全没有实现**
- `StreamDoneMsg` 处理中没有任何队列 drain 逻辑
- 回合结束后直接等待用户新输入
- Print 模式只支持单次查询

```go
// internal/tui/update.go — 回合结束处理
case StreamDoneMsg:
    m.isLoading = false
    m.showSpinner = false
    m.abortFn = nil
    m.streamCh = nil
    // ... 清理流状态
    return m, nil  // ← 没有 drain 逻辑，直接返回等待
```

**差距影响**：
- 用户即使在 AI 工作时成功排队了消息（假设有队列），回合结束后也不会自动处理
- Print 模式不支持多轮对话或后台 Agent 通知处理

**建议优先级**：🔴 P1（依赖统一命令队列 P0 + QueryGuard P1）

**修复难度**：中（~150 行代码）

---

### 2.6 ❌ Early Input — 启动阶段击键缓冲

**原版实现**：

```typescript
// src/utils/earlyInput.ts — 在 CLI 入口尽早调用

export function startCapturingEarlyInput(): void {
  if (!process.stdin.isTTY || isCapturing) return
  isCapturing = true
  earlyInputBuffer = ''
  process.stdin.setEncoding('utf8')
  process.stdin.setRawMode(true)
  process.stdin.ref()

  readableHandler = () => {
    let chunk = process.stdin.read()
    while (chunk !== null) {
      processChunk(chunk)  // 逐字符处理
      chunk = process.stdin.read()
    }
  }
  process.stdin.on('readable', readableHandler)
}

// processChunk 处理：
// - Ctrl+C → process.exit(130)
// - Ctrl+D → EOF，停止捕获
// - Backspace → 删除最后一个字素簇
// - Escape 序列 → 跳过
// - 回车 → 转换为换行
// - 可打印字符 → 添加到缓冲

export function consumeEarlyInput(): string {
  stopCapturingEarlyInput()
  return earlyInputBuffer.trim()
}
```

设计理念：
- 从**进程启动的第一毫秒**开始捕获用户输入
- 在 REPL/React/Ink 初始化完成前的击键不会丢失
- 用户习惯在输入 `claude` 后立即开始打字

**当前 Go 实现**：
- **完全没有实现**
- BubbleTea 应用启动后才开始接受输入
- 启动阶段（配置加载、OAuth、API 客户端初始化等）的用户击键**全部丢失**

```go
// internal/tui/input.go — TextArea 在初始化完成后才可用
func NewInput(vimEnabled bool) InputModel {
    ta := textarea.New()
    ta.Focus()  // 此时 BubbleTea 已完成初始化
    return InputModel{textarea: ta}
}
```

**差距影响**：
- 用户在 `claude` 命令启动过程中（可能持续 1-3 秒，OAuth 预热时更长）输入的文字全部丢失
- 对于高级用户（快速输入习惯）体验较差

**建议优先级**：🟡 P2（体验优化）

**修复难度**：中（~150 行代码，需要在 main.go 入口处尽早捕获 stdin）

---

### 2.7 ❌ 排队消息预览 UI

**原版实现**：

```typescript
// src/components/PromptInput/PromptInputQueuedCommands.tsx
function PromptInputQueuedCommandsImpl(): React.ReactNode {
  const queuedCommands = useCommandQueue()  // 订阅队列变化

  // 过滤出可见的命令（排除 isMeta 的系统消息）
  const visibleCommands = queuedCommands.filter(isQueuedCommandVisible)
  
  // 任务通知最多显示 3 条，超出的合并为摘要
  const processedCommands = processQueuedCommands(visibleCommands)

  return (
    <Box marginTop={1} flexDirection="column">
      {messages.map((message, i) => (
        <QueuedMessageProvider key={i} isFirst={i === 0}>
          <Message message={message} isStatic={true} ... />
        </QueuedMessageProvider>
      ))}
    </Box>
  )
}
```

核心特性：
- 在输入框下方**实时预览**排队中的消息
- 通过 `QueuedMessageProvider` 提供不同于正常消息的视觉样式（dim 颜色）
- 任务通知溢出处理：最多显示 3 条，超出的合并为 "+N more tasks completed"
- 给用户即时反馈："我的消息已被接收"

**当前 Go 实现**：
- **完全没有实现**
- 没有队列预览 UI 组件
- 用户无法看到排队中的消息
- `internal/tui/view.go` 仅渲染对话历史和流式输出

**建议优先级**：🟡 P2（依赖统一命令队列 P0）

**修复难度**：中（~100 行代码，BubbleTea 渲染组件）

---

### 2.8 ❌ 排队消息撤回（Pop）

**原版实现**：

```typescript
// src/utils/messageQueueManager.ts
function popAllEditable(currentInput, currentCursorOffset) {
  const { editable, nonEditable } = objectGroupBy(
    [...commandQueue],
    cmd => isQueuedCommandEditable(cmd) ? 'editable' : 'nonEditable',
  )
  if (editable.length === 0) return undefined

  const queuedTexts = editable.map(cmd => extractTextFromValue(cmd.value))
  const newInput = [...queuedTexts, currentInput].filter(Boolean).join('\n')

  // 只保留不可编辑的命令
  commandQueue.length = 0
  commandQueue.push(...nonEditable)
  notifySubscribers()

  return { text: newInput, cursorOffset, images }
}
```

设计理念：
- 用户按 Escape（Claude 空闲时）可以**撤回所有排队消息**
- 撤回的文字和当前输入框文字合并
- 不可编辑的命令（如任务通知）保留在队列中
- 允许用户在发送后立即反悔

**当前 Go 实现**：
- **完全没有实现**
- Escape 键在 Go 版本中仅用于取消当前查询

**建议优先级**：🟢 P3（依赖统一命令队列 P0）

**修复难度**：低（~80 行代码）

---

### 2.9 ❌ 可中断工具检测与自动中断

**原版实现**：

```typescript
// src/utils/handlePromptSubmit.ts
if (queryGuard.isActive || isExternalLoading) {
  // 如果当前所有执行中的工具都是可中断的（如 SleepTool），
  // 自动中断当前回合
  if (params.hasInterruptibleToolInProgress) {
    params.abortController?.abort('interrupt')
  }

  enqueue({
    value: finalInput.trim(),
    mode,
    priority: 'next',
    pastedContents: hasImages ? pastedContents : undefined,
  })
}
```

设计理念：
- 某些工具（如 SleepTool）被标记为"可中断的"
- 当用户提交新消息时，如果当前**所有**执行中的工具都是可中断的，自动 abort
- 避免用户等待不必要的 Sleep/等待操作

**当前 Go 实现**：
- **完全没有实现**
- 没有工具的"可中断"标记（`isInterruptible`）
- 新提交总是直接取消上一个查询（更粗暴）

**建议优先级**：🟡 P2

**修复难度**：低（~50 行代码，需要在 Tool 接口上添加 `IsInterruptible()` 方法）

---

### 2.10 ❌ `now` 优先级流式中断

**原版实现**：

```typescript
// src/cli/print.ts — 全局订阅
subscribeToCommandQueue(() => {
  if (abortController && getCommandsByMaxPriority('now').length > 0) {
    abortController.abort('interrupt')
  }
})
```

设计理念：
- 队列中出现 `now` 优先级的命令时，**立即中断**当前的流式响应
- 不等工具完成，不等 API 调用返回
- 等同于用户按 Esc + 发送

**当前 Go 实现**：
- **完全没有实现**
- 没有队列订阅机制
- 没有 `now` 优先级概念
- 中断只能通过手动 Ctrl+C

**建议优先级**：🟡 P2（依赖统一命令队列 P0）

**修复难度**：低（~30 行代码，队列订阅 + abort 触发）

---

### 2.11 ❌ Escape 键优先级处理链

**原版实现**：

```typescript
// src/hooks/useCancelRequest.ts
const handleCancel = useCallback(() => {
  // 优先级 1: 取消正在运行的查询
  if (abortSignal !== undefined && !abortSignal.aborted) {
    setToolUseConfirmQueue(() => [])
    onCancel()
    return
  }

  // 优先级 2: 弹出队列中的消息
  if (hasCommandsInQueue()) {
    if (popCommandFromQueue) {
      popCommandFromQueue()
      return
    }
  }

  // 回退: 无可取消/弹出的内容
  setToolUseConfirmQueue(() => [])
  onCancel()
}, [...])
```

**当前 Go 实现**：
- Escape/Ctrl+C 仅有一个行为：取消当前查询或退出
- 没有优先级处理链
- 没有"弹出队列"的回退行为

```go
// internal/tui/keys.go
case tea.KeyCtrlC:
    if m.isLoading {
        return m.doAbort()  // 仅取消当前查询
    }
    return m, tea.Quit      // 退出程序
```

**建议优先级**：🟡 P2

**修复难度**：低（~50 行代码）

---

### 2.12 ❌ 双击确认杀死后台 Agent

**原版实现**：

```typescript
// src/hooks/useCancelRequest.ts
const handleKillAgents = useCallback(() => {
  const now = Date.now()
  const elapsed = now - lastKillAgentsPressRef.current
  if (elapsed <= KILL_AGENTS_CONFIRM_WINDOW_MS) {  // 3 秒确认窗口
    // 第二次按下 → 真正杀死
    clearCommandQueue()
    killAllAgentsAndNotify()
  } else {
    // 第一次按下 → 显示确认提示
    lastKillAgentsPressRef.current = now
    addNotification({
      text: `Press ${shortcut} again to stop background agents`,
      timeoutMs: KILL_AGENTS_CONFIRM_WINDOW_MS,
    })
  }
}, [...])
```

**当前 Go 实现**：
- **完全没有实现**
- 没有 `killAllAgents` 功能
- 没有 3 秒确认窗口机制
- 没有通知提示

**建议优先级**：🟢 P3

**修复难度**：低（~60 行代码）

---

### 2.13 ❌ Print 模式多消息处理与流式中断

**原版实现**：

```typescript
// src/cli/print.ts — 完整的非交互式队列处理
do {
  for (const event of drainSdkEvents()) {
    output.enqueue(event)
  }
  runPhase = 'draining_commands'
  await drainCommandQueue()   // drain 并批量处理队列中的命令
} while (waitingForAgents)    // 后台 Agent 运行时持续循环

// 流式中断监听
subscribeToCommandQueue(() => {
  if (abortController && getCommandsByMaxPriority('now').length > 0) {
    abortController.abort('interrupt')
  }
})
```

核心特性：
- Print 模式下也支持命令队列的 drain 循环
- 支持等待后台 Agent 完成
- 支持 `now` 优先级的流式中断
- SDK 事件和命令队列双重处理

**当前 Go 实现**：

```go
// internal/bootstrap/run.go — 仅支持单次查询
func headlessRun(container *AppContainer, prompt string, ...) error {
    msgCh, err := container.QueryEngine.Query(ctx, params)
    switch f.outputFormat {
    case "json":    return consumeJSON(ctx, msgCh, container)
    case "stream-json": return consumeStreamJSON(ctx, msgCh)
    default:        return consumeText(ctx, msgCh)
    }
    // 查询完成后直接返回，没有 drain 循环
}
```

**差距分析**：
- 不支持多消息处理（仅单次 `prompt` 参数）
- 不支持后台 Agent 等待循环
- 不支持命令队列的 drain
- 不支持流式中断

**建议优先级**：🟡 P2

**修复难度**：中（~150 行代码）

---

### 2.14 ❌ Signal / 轻量级事件原语

**原版实现**：

```typescript
// src/utils/signal.ts — 44 行，极简 pub-sub
export function createSignal() {
  const listeners = new Set<() => void>()
  return {
    subscribe: (fn: () => void) => {
      listeners.add(fn)
      return () => { listeners.delete(fn) }
    },
    emit: () => { listeners.forEach(fn => fn()) },
  }
}
```

用途：
- `messageQueueManager` 的队列变更通知
- `QueryGuard` 的状态变更通知
- 替代 React Context 传播的延迟问题

**当前 Go 实现**：
- **没有等效的轻量级 pub-sub 原语**
- Go 的 channel 可以实现类似功能，但缺少 "多订阅者广播" 能力
- BubbleTea 的 Msg 机制是单向的（通道 → Update），不支持外部状态订阅

**建议优先级**：🟡 P2（作为统一命令队列的基础设施）

**修复难度**：低（~60 行代码，可使用 `sync.Cond` 或自定义广播 channel）

---

### 2.15 ❌ 后台 Agent 任务消息队列（完整版）

**原版实现**：

```typescript
// src/tasks/LocalAgentTask/LocalAgentTask.tsx
type LocalAgentTaskState = {
  pendingMessages: string[]       // 通过 SendMessage 排队
  status: 'running' | 'completed' | 'killed'
  abortController?: AbortController
  isBackgrounded: boolean
  retain: boolean
}

// 排队
function queuePendingMessage(taskId, msg, setAppState) {
  // 添加到 pendingMessages
}

// 消费（在 Agent 工具轮次边界）
function drainPendingMessages(taskId, getAppState, setAppState) {
  // 取出所有 pendingMessages
  // 附加到 Agent 对话记录
  // 清空 pendingMessages
}

// Agent 完成通知 → 通过统一队列发给主线程
enqueuePendingNotification({
  value: summary,
  mode: 'task-notification',
  priority: 'later',
  agentId: undefined,  // 发给主线程
})
```

核心特性：
- Agent 有独立的 `pendingMessages` 缓冲
- 消息在工具轮次边界被 drain
- 完成通知通过统一队列的 `later` 优先级发给主线程
- 主线程的 Mid-Turn Drain 通过 `agentId` 过滤

**当前 Go 实现**：
- Coordinator 有 16 缓冲的 `inboxCh`（基础可用）
- 但没有 `pendingMessages` drain 机制
- 没有完成通知的队列化路径
- SendMessage 是 fire-and-forget，没有回复处理

**差距分析**：
- Go 的 `inboxCh` 是底层传输机制，但缺少高层的 drain 语义
- Agent 完成时没有通过统一队列通知主线程
- 主线程无法在 Mid-Turn 中看到 Agent 的完成通知

**建议优先级**：🟡 P2（依赖统一命令队列 P0 + Agent 系统完善）

**修复难度**：高（~200 行代码，跨 coordinator/engine 两层）

---

### 2.16 ❌ AbortController 完整传播链

**原版实现**：

```
handlePromptSubmit
  ↓ createAbortController()
  ↓ setAbortController(abortController)
REPL.tsx
  ↓ abortController 存入 state
  ↓ 传给 CancelRequestHandler
onQuery(newMessages, abortController, ...)
  ↓
query.ts
  ↓ toolUseContext.abortController
  ↓ 传给 callModel()（作为 signal）
  ↓ 传给 streamingToolExecutor
print.ts
  ↓ subscribeToCommandQueue 监听 'now'
  ↓ → abortController.abort('interrupt')
```

**当前 Go 实现**：
```
startQueryCmd
  ↓ context.WithCancel()
  ↓ m.abortFn = cancel
engine.Query
  ↓ ctx 传入 goroutine
  ↓ select { case <-ctx.Done(): return }
doAbort
  ↓ engine.Interrupt()
```

**差距分析**：
- Go 使用 `context.Context` 取消链，功能上等效于 AbortController
- 但没有 abort reason（Go 的 `context.Cancel()` 不携带原因）
- 没有在 print 模式中通过队列触发 abort
- 缺少 `abort('interrupt')` vs `abort('cancel')` 的语义区分

**建议优先级**：🟢 P3

**修复难度**：低（Go 1.20+ 的 `context.WithCancelCause` 可携带原因）

---

### 2.17 ❌ 用户输入过程中查询状态锁定（Input Disabled While Loading）

**原版实现**：
- 输入框**始终可用**，即使 AI 正在回复
- 用户随时可以打字，消息被排入队列

**当前 Go 实现**：
- `handleSubmit()` 在 `isLoading` 时没有入队逻辑
- BubbleTea 的 textarea 在技术上仍然接受输入，但提交时会**直接取消**上一个查询再启动新查询
- 没有"排队"选项——只有"替换"

```go
// internal/tui/keys.go — handleSubmit 没有对 isLoading 的入队分支
func (m AppModel) handleSubmit() (tea.Model, tea.Cmd) {
    text := m.input.Value()
    // 没有检查 m.isLoading → 没有入队分支
    // startQueryCmd 内部会 cancel 上一个查询
    queryCmd := startQueryCmd(&m, text)
    return m, queryCmd
}
```

**差距影响**：
- 用户在 AI 回复时提交新消息会**丢失正在进行的回复**
- 原版允许消息排队而不打断 AI 的工作

**建议优先级**：🔴 P1（核心 UX 差距，与统一命令队列 P0 耦合）

**修复难度**：低（一旦有统一命令队列，仅需在 handleSubmit 中添加入队分支）

---

## 三、架构差异总结

| 维度 | 原版 TypeScript | 当前 Go | 完成度 |
|------|----------------|---------|--------|
| **统一命令队列** | 模块级单例 + 优先级排序 + 订阅 | 无 | ❌ 0% |
| **QueryGuard 并发守卫** | 三态状态机 + 代次追踪 | `isLoading` 布尔 + `context.Cancel` | ⚠️ 15% |
| **三级优先级** | `now` > `next` > `later` | 无 | ❌ 0% |
| **Mid-Turn Drain** | 工具间隙注入 + 附件消息 | 无 | ❌ 0% |
| **Between-Turn Drain** | REPL Effect + Print 循环 | 无 | ❌ 0% |
| **Early Input 缓冲** | 启动即捕获 + raw mode | 无 | ❌ 0% |
| **队列预览 UI** | 实时预览 + 溢出合并 | 无 | ❌ 0% |
| **消息撤回（Pop）** | Escape 弹出 + 合并到输入框 | 无 | ❌ 0% |
| **可中断工具检测** | `isInterruptible` 标记 + 自动 abort | 无 | ❌ 0% |
| **Now 优先级中断** | 队列订阅 + 立即 abort | 无 | ❌ 0% |
| **Escape 优先级链** | 取消查询 → 弹出队列 → 退出 | 仅取消或退出 | ⚠️ 30% |
| **双击杀死 Agent** | 3 秒确认窗口 + 通知 | 无 | ❌ 0% |
| **Print 模式队列** | drain 循环 + Agent 等待 + 中断 | 单次查询 | ⚠️ 20% |
| **Signal pub-sub** | 轻量级广播原语 | 无 | ❌ 0% |
| **Agent 消息队列** | pendingMessages + drain + 通知 | 16 缓冲 inbox（基础） | ⚠️ 25% |
| **Abort 传播链** | AbortController + reason | context.Cancel（无 reason） | ⚠️ 60% |
| **输入状态锁定** | 始终可输入 → 排队 | 提交 = 替换当前查询 | ⚠️ 20% |
| **基础取消机制** | 队列化 cancel 指令 | `context.Cancel()` | ✅ 70% |
| **权限对话框** | 阻塞式 + 队列 | 阻塞式（缓冲 1） | ✅ 85% |
| **Agent 事件流** | 统一队列 + 通知 | 独立通道（缓冲 64） | ⚠️ 60% |

**整体评估**：约 **15-20%** 功能覆盖率。基础的查询执行、取消、权限对话框功能可用，但整个消息队列化体系和用户交互体验优化几乎全部缺失。

---

## 四、优先改进计划

### 🔴 P0 — 核心基础设施

| # | 功能 | 工作量估算 | 影响范围 | 说明 |
|---|------|-----------|----------|------|
| 1 | **统一命令队列（MessageQueueManager）** | 3-4 天 | 全局 | 所有高级功能的根本依赖；包含 `QueuedCommand` 类型、`enqueue/dequeue` + 优先级排序、`Signal` pub-sub 广播 |

**P0 总工作量**：~3-4 天

### 🔴 P1 — 核心交互能力

| # | 功能 | 工作量估算 | 前置依赖 |
|---|------|-----------|----------|
| 2 | QueryGuard 并发守卫 | 1.5 天 | P0（需要队列状态判断） |
| 3 | 三级优先级调度 | 1 天 | P0 |
| 4 | 输入状态排队（isLoading → enqueue） | 0.5 天 | P0 |
| 5 | Mid-Turn Drain（引擎核心修改） | 3 天 | P0 + P1-2 + 附件注入机制 |
| 6 | Between-Turn Drain（REPL + Print） | 2 天 | P0 + P1-2 |

**P1 总工作量**：~8 天

### 🟡 P2 — 体验完善

| # | 功能 | 工作量估算 |
|---|------|-----------|
| 7 | Early Input 启动缓冲 | 1.5 天 |
| 8 | 排队消息预览 UI | 1 天 |
| 9 | Signal / pub-sub 原语 | 0.5 天 |
| 10 | 可中断工具检测 + 自动 abort | 0.5 天 |
| 11 | `now` 优先级流式中断 | 0.5 天 |
| 12 | Escape 键优先级处理链 | 0.5 天 |
| 13 | Print 模式多消息处理 | 1.5 天 |
| 14 | Agent 消息队列完善 | 2 天 |

**P2 总工作量**：~8 天

### 🟢 P3 — 高级功能

| # | 功能 | 工作量估算 |
|---|------|-----------|
| 15 | 排队消息撤回（Pop） | 1 天 |
| 16 | 双击确认杀死 Agent | 0.5 天 |
| 17 | Abort 传播链完善（携带 reason） | 0.5 天 |
| 18 | 命令生命周期追踪（UUID） | 1 天 |

**P3 总工作量**：~3 天

---

## 五、实现建议

### 5.1 统一命令队列（P0）

```go
// 建议位置: internal/msgqueue/queue.go

// QueuePriority 定义消息优先级
type QueuePriority int

const (
    PriorityNow  QueuePriority = iota // 最高 — 立即中断流式传输
    PriorityNext                       // 中等 — 回合内工具间隙注入
    PriorityLater                      // 最低 — 回合结束后处理
)

// QueuedCommand 表示一个排队中的命令
type QueuedCommand struct {
    Value       string         // 消息内容
    Mode        CommandMode    // prompt/bash/task-notification
    Priority    QueuePriority  // 优先级
    UUID        string         // 生命周期追踪
    AgentID     string         // 目标 Agent（空 = 主线程）
    IsMeta      bool           // 系统消息（UI 隐藏）
    Origin      string         // 来源标记
    CreatedAt   time.Time      // 创建时间
}

// CommandMode 定义命令模式
type CommandMode string

const (
    ModePrompt           CommandMode = "prompt"
    ModeBash             CommandMode = "bash"
    ModeTaskNotification CommandMode = "task-notification"
)

// MessageQueue 是统一命令队列（进程级单例）
type MessageQueue struct {
    mu       sync.RWMutex
    commands []QueuedCommand
    signal   *Signal  // 变更通知广播
}

// Enqueue 将命令加入队列
func (q *MessageQueue) Enqueue(cmd QueuedCommand) {
    q.mu.Lock()
    if cmd.Priority == 0 {
        cmd.Priority = PriorityNext // 默认 next
    }
    if cmd.UUID == "" {
        cmd.UUID = uuid.New().String()
    }
    cmd.CreatedAt = time.Now()
    q.commands = append(q.commands, cmd)
    q.mu.Unlock()
    q.signal.Emit()
}

// Dequeue 取出优先级最高的命令
func (q *MessageQueue) Dequeue(filter func(QueuedCommand) bool) (QueuedCommand, bool) {
    q.mu.Lock()
    defer q.mu.Unlock()

    bestIdx := -1
    bestPriority := QueuePriority(999)
    for i, cmd := range q.commands {
        if filter != nil && !filter(cmd) {
            continue
        }
        if cmd.Priority < bestPriority {
            bestIdx = i
            bestPriority = cmd.Priority
        }
    }
    if bestIdx == -1 {
        return QueuedCommand{}, false
    }

    cmd := q.commands[bestIdx]
    q.commands = append(q.commands[:bestIdx], q.commands[bestIdx+1:]...)
    q.signal.Emit()
    return cmd, true
}

// GetByMaxPriority 获取不超过指定优先级的所有命令（不移除）
func (q *MessageQueue) GetByMaxPriority(maxPriority QueuePriority) []QueuedCommand {
    q.mu.RLock()
    defer q.mu.RUnlock()

    var result []QueuedCommand
    for _, cmd := range q.commands {
        if cmd.Priority <= maxPriority {
            result = append(result, cmd)
        }
    }
    return result
}

// Len 返回队列长度
func (q *MessageQueue) Len() int {
    q.mu.RLock()
    defer q.mu.RUnlock()
    return len(q.commands)
}

// Snapshot 返回队列的只读快照
func (q *MessageQueue) Snapshot() []QueuedCommand {
    q.mu.RLock()
    defer q.mu.RUnlock()
    snap := make([]QueuedCommand, len(q.commands))
    copy(snap, q.commands)
    return snap
}

// Subscribe 订阅队列变更事件
func (q *MessageQueue) Subscribe(fn func()) func() {
    return q.signal.Subscribe(fn)
}
```

### 5.2 Signal 广播原语（P2，但 P0 基础设施需要）

```go
// 建议位置: internal/msgqueue/signal.go

// Signal 是轻量级多订阅者广播原语
type Signal struct {
    mu        sync.RWMutex
    listeners map[int]func()
    nextID    int
}

// NewSignal 创建新的 Signal
func NewSignal() *Signal {
    return &Signal{listeners: make(map[int]func())}
}

// Subscribe 注册一个监听器，返回取消订阅函数
func (s *Signal) Subscribe(fn func()) func() {
    s.mu.Lock()
    id := s.nextID
    s.nextID++
    s.listeners[id] = fn
    s.mu.Unlock()

    return func() {
        s.mu.Lock()
        delete(s.listeners, id)
        s.mu.Unlock()
    }
}

// Emit 通知所有订阅者
func (s *Signal) Emit() {
    s.mu.RLock()
    for _, fn := range s.listeners {
        fn()
    }
    s.mu.RUnlock()
}
```

### 5.3 QueryGuard 并发守卫（P1）

```go
// 建议位置: internal/msgqueue/guard.go

// QueryGuardStatus 表示守卫状态
type QueryGuardStatus int

const (
    GuardIdle        QueryGuardStatus = iota // 空闲
    GuardDispatching                          // 派发中（同步间隙保护）
    GuardRunning                              // 运行中
)

// QueryGuard 是并发查询守卫（三态状态机）
type QueryGuard struct {
    mu         sync.Mutex
    status     QueryGuardStatus
    generation int
    signal     *Signal
}

// NewQueryGuard 创建新的 QueryGuard
func NewQueryGuard() *QueryGuard {
    return &QueryGuard{signal: NewSignal()}
}

// Reserve 预留（idle → dispatching），返回是否成功
func (g *QueryGuard) Reserve() bool {
    g.mu.Lock()
    defer g.mu.Unlock()
    if g.status != GuardIdle {
        return false
    }
    g.status = GuardDispatching
    g.signal.Emit()
    return true
}

// CancelReservation 取消预留（dispatching → idle）
func (g *QueryGuard) CancelReservation() {
    g.mu.Lock()
    defer g.mu.Unlock()
    if g.status != GuardDispatching {
        return
    }
    g.status = GuardIdle
    g.signal.Emit()
}

// TryStart 尝试启动（dispatching → running），返回代次
func (g *QueryGuard) TryStart() (int, bool) {
    g.mu.Lock()
    defer g.mu.Unlock()
    if g.status == GuardRunning {
        return 0, false
    }
    g.status = GuardRunning
    g.generation++
    g.signal.Emit()
    return g.generation, true
}

// End 结束（running → idle），需匹配代次
func (g *QueryGuard) End(generation int) bool {
    g.mu.Lock()
    defer g.mu.Unlock()
    if g.generation != generation || g.status != GuardRunning {
        return false
    }
    g.status = GuardIdle
    g.signal.Emit()
    return true
}

// ForceEnd 强制结束（任意状态 → idle），推进代次
func (g *QueryGuard) ForceEnd() {
    g.mu.Lock()
    defer g.mu.Unlock()
    g.status = GuardIdle
    g.generation++
    g.signal.Emit()
}

// IsActive 返回是否有活跃的查询
func (g *QueryGuard) IsActive() bool {
    g.mu.Lock()
    defer g.mu.Unlock()
    return g.status != GuardIdle
}

// Subscribe 订阅状态变更
func (g *QueryGuard) Subscribe(fn func()) func() {
    return g.signal.Subscribe(fn)
}
```

### 5.4 Mid-Turn Drain 引擎修改（P1）

```go
// 建议修改: internal/engine/query.go — runQueryLoop 内部

func (e *engineImpl) runQueryLoop(ctx context.Context, params QueryParams, msgCh chan<- Msg) {
    for {
        select {
        case <-ctx.Done(): return
        default:
        }

        // 流式 LLM 响应...
        // 执行工具...

        // ★ Mid-Turn Drain 检查点（工具执行完成后、下次 API 调用前）
        if e.msgQueue != nil {
            sleepRan := hasSleepToolRun(toolResults)
            maxPriority := PriorityNext
            if sleepRan {
                maxPriority = PriorityLater // Sleep 后扩大 drain 范围
            }

            queued := e.msgQueue.GetByMaxPriority(maxPriority)
            // 过滤：排除斜杠命令，仅处理目标 Agent 匹配的
            filtered := filterForDrain(queued, params.AgentID)

            if len(filtered) > 0 {
                // 将排队消息转为用户消息附件注入当前回合
                for _, cmd := range filtered {
                    userMsg := types.Message{
                        Role:    types.RoleUser,
                        Content: []types.ContentBlock{
                            {Type: types.ContentTypeText, Text: strPtr(cmd.Value)},
                        },
                    }
                    messages = append(messages, userMsg)
                    sendMsg(ctx, msgCh, Msg{
                        Type:    MsgTypeUserMessage,
                        UserMsg: &userMsg,
                    })
                }
                // 从队列中移除已消费的命令
                e.msgQueue.RemoveByUUIDs(extractUUIDs(filtered))
            }
        }

        // 继续下一轮 API 调用...
    }
}
```

### 5.5 Between-Turn Drain（TUI 层修改）（P1）

```go
// 建议修改: internal/tui/update.go — StreamDoneMsg 处理

case StreamDoneMsg:
    m.isLoading = false
    m.showSpinner = false
    m.abortFn = nil
    m.streamCh = nil
    // ... 原有清理逻辑 ...

    // ★ Between-Turn Drain：检查队列中是否有待处理的命令
    if m.msgQueue != nil && m.msgQueue.Len() > 0 && m.activeDialog == dialogNone {
        return m, processQueueCmd(m.msgQueue, m.queryEngine, m.memdirPrompt, m.agentCoordinator)
    }
    return m, nil

// processQueueCmd 处理队列中的下一个命令
func processQueueCmd(q *msgqueue.MessageQueue, qe engine.QueryEngine, ...) tea.Cmd {
    return func() tea.Msg {
        // 主线程过滤器
        isMainThread := func(cmd msgqueue.QueuedCommand) bool {
            return cmd.AgentID == ""
        }

        cmd, ok := q.Dequeue(isMainThread)
        if !ok {
            return nil
        }

        // 斜杠命令逐条处理
        if isSlashCommand(cmd.Value) {
            return SlashCommandMsg{Text: cmd.Value}
        }

        // 普通命令 → 启动新查询
        return QueuedSubmitMsg{Commands: []msgqueue.QueuedCommand{cmd}}
    }
}
```

### 5.6 handleSubmit 入队分支（P1）

```go
// 建议修改: internal/tui/keys.go — handleSubmit

func (m AppModel) handleSubmit() (tea.Model, tea.Cmd) {
    text := m.input.Value()
    if text == "" { return m, nil }
    m.input = m.input.SetValue("")

    if IsSlashCommand(text) {
        return m.handleSlashCommand(text)
    }

    // ★ 如果有活跃查询，入队而不是直接执行
    if m.isLoading && m.msgQueue != nil {
        m.msgQueue.Enqueue(msgqueue.QueuedCommand{
            Value:    text,
            Mode:     msgqueue.ModePrompt,
            Priority: msgqueue.PriorityNext,
        })
        // 可选：如果当前工具可中断，自动 abort
        return m, nil // 不启动新查询，消息已入队
    }

    // 系统空闲 → 直接执行
    m.messages = append(m.messages, newUserMessage(text))
    m.isLoading = true
    queryCmd := startQueryCmd(&m, text)
    return m, queryCmd
}
```

### 5.7 Early Input 缓冲（P2）

```go
// 建议位置: internal/earlyinput/capture.go

var (
    mu       sync.Mutex
    buffer   strings.Builder
    capturing bool
)

// StartCapturing 在 main() 入口处尽早调用
func StartCapturing() {
    mu.Lock()
    defer mu.Unlock()
    if capturing { return }

    // 检查 stdin 是否为终端
    if !term.IsTerminal(int(os.Stdin.Fd())) { return }

    capturing = true
    oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
    if err != nil { return }

    go func() {
        defer term.Restore(int(os.Stdin.Fd()), oldState)
        buf := make([]byte, 1)
        for {
            mu.Lock()
            if !capturing {
                mu.Unlock()
                return
            }
            mu.Unlock()

            n, err := os.Stdin.Read(buf)
            if err != nil || n == 0 { return }

            b := buf[0]
            switch {
            case b == 3: // Ctrl+C
                os.Exit(130)
            case b == 4: // Ctrl+D
                return
            case b == 127 || b == 8: // Backspace
                mu.Lock()
                s := buffer.String()
                if len(s) > 0 {
                    buffer.Reset()
                    buffer.WriteString(s[:len(s)-1])
                }
                mu.Unlock()
            case b == 13: // Enter → newline
                mu.Lock()
                buffer.WriteByte('\n')
                mu.Unlock()
            case b == 27: // Escape sequence → skip
                continue
            case b < 32: // Other control chars → skip
                continue
            default:
                mu.Lock()
                buffer.WriteByte(b)
                mu.Unlock()
            }
        }
    }()
}

// Consume 返回缓冲的文本并停止捕获
func Consume() string {
    mu.Lock()
    defer mu.Unlock()
    capturing = false
    result := strings.TrimSpace(buffer.String())
    buffer.Reset()
    return result
}
```

---

## 六、代码质量问题

### 6.1 handleSubmit 缺少查询活跃状态检查

```go
// internal/tui/keys.go
func (m AppModel) handleSubmit() (tea.Model, tea.Cmd) {
    text := m.input.Value()
    if text == "" { return m, nil }
    // ❌ 没有检查 m.isLoading — 会直接取消上一个查询
    queryCmd := startQueryCmd(&m, text) // startQueryCmd 内部 cancel 上一个
    return m, queryCmd
}
```

**问题**：用户在 AI 回复时提交新消息会**静默取消**正在进行的回复，没有提示或确认。

**建议**：添加 `m.isLoading` 检查分支，在有统一队列时入队，否则至少显示确认提示。

### 6.2 startQueryCmd 的竞态风险

```go
// internal/tui/cmds.go
func startQueryCmd(m *AppModel, userText string) tea.Cmd {
    if m.abortFn != nil {
        m.abortFn()      // ① 取消上一个查询
        m.abortFn = nil   // ② 清除 handle
    }
    ctx, cancel := context.WithCancel(context.Background())
    m.abortCtx = ctx      // ③ 设置新的 ctx
    m.abortFn = cancel    // ④ 设置新的 cancel
    // ⑤ 启动新的 goroutine
}
```

**问题**：步骤 ① 取消上一个查询后，其 goroutine 可能仍在运行并向已关闭的 `streamCh` 发送消息。虽然 `doAbort()` 中设置了 `m.streamCh = nil` 来忽略过期事件（P1-D fix），但在 `handleSubmit()` 路径中没有这个保护。

**建议**：在 `startQueryCmd` 中也清除 `m.streamCh`，或引入 QueryGuard 的代次机制。

### 6.3 doAbort 后的状态不一致

```go
// internal/tui/keys.go
func (m AppModel) doAbort() (tea.Model, tea.Cmd) {
    abortCmd := abortQueryCmd(m.queryEngine)
    m.isLoading = false
    m.showSpinner = false
    m.spinner = m.spinner.Reset()
    m.abortFn = nil
    m.streamCh = nil
    // ❌ 没有保存当前正在生成的流式文本到消息历史
    // ❌ 没有添加 "[cancelled]" 标记
    return m, abortCmd
}
```

**问题**：取消后正在流式生成的回复内容丢失，用户看不到 AI 已经生成的部分内容。

**建议**：在 `doAbort()` 中检查 `m.streamingText`，如果非空则保存为截断的消息。

### 6.4 权限通道缓冲区过小

```go
// internal/bootstrap/wire.go
askCh := make(chan permissions.AskRequest, 1)   // 缓冲 1
respCh := make(chan permissions.AskResponse, 1) // 缓冲 1
```

**问题**：如果工具批量并发执行（最多 10 个），多个工具同时请求权限会阻塞。缓冲区为 1 意味着只能有 1 个待处理 + 1 个正在显示。

**建议**：将缓冲区增大到与最大并发工具数一致（10），或实现权限请求队列。

### 6.5 Agent 收件箱满时的错误处理不够友好

```go
// internal/coordinator/coordinator.go
func (c *coordinatorImpl) SendMessage(_ context.Context, to AgentID, message string) error {
    select {
    case entry.inboxCh <- message:
        return nil
    default:
        return fmt.Errorf("coordinator: agent %s inbox is full (capacity %d)", to, inboxBufferSize)
    }
}
```

**问题**：收件箱满时直接返回错误给 LLM，LLM 可能不知道如何处理。

**建议**：
- 考虑阻塞等待（带超时）而不是立即失败
- 或增大缓冲区
- 或实现溢出策略（如丢弃最旧的消息）

---

## 关键依赖关系图

```
                  统一命令队列 (P0)
                 /       |        \
                /        |         \
    QueryGuard (P1)  优先级调度 (P1)  Signal pub-sub (P2)
         |              |              |
         v              v              v
    输入状态排队    Mid-Turn Drain   队列预览 UI
      (P1)           (P1)           (P2)
         \              |              /
          \             v             /
           →   Between-Turn Drain  ←
                    (P1)
                     |
         ┌───────────┼───────────┐
         v           v           v
    Print 模式    Escape 优先   Agent 消息
    多消息 (P2)   级链 (P2)    队列 (P2)
```

**实现顺序建议**：
1. `P0` Signal pub-sub 原语（统一命令队列的基础设施）
2. `P0` 统一命令队列（MessageQueue）
3. `P1` QueryGuard 并发守卫
4. `P1` 三级优先级调度（与队列一同实现）
5. `P1` handleSubmit 入队分支（最小可用改动）
6. `P1` Between-Turn Drain（REPL 层）
7. `P1` Mid-Turn Drain（引擎层，最复杂）
8. `P2` Early Input 缓冲
9. `P2` 队列预览 UI
10. `P2` 其余功能按优先级实现

---

## Go 与 TypeScript 架构适配说明

### 为什么 Go 版本不能简单翻译

1. **并发模型差异**：TypeScript 是单线程事件循环 + Promise，Go 是多线程 goroutine + channel。TypeScript 中 `dispatching` 中间态保护的"同步间隙"在 Go 中需要不同的处理方式（`sync.Mutex` + `sync.Cond`）。

2. **UI 框架差异**：TypeScript 使用 React/Ink（声明式组件 + `useSyncExternalStore`），Go 使用 BubbleTea（Elm 架构 + `tea.Msg`）。BubbleTea 的 `Update` 是同步的单线程消息处理，天然避免了部分竞态问题，但也不能像 React 那样通过 Effect 自动触发队列处理。

3. **状态管理差异**：TypeScript 使用模块级单例（`commandQueue`）+ React 外部存储，Go 需要通过 DI（`AppContainer`）或全局单例 + BubbleTea 的 `tea.Msg` 通知机制。

4. **取消机制差异**：TypeScript 的 `AbortController` 支持 reason，Go 的 `context.Context` 在 1.20+ 才通过 `WithCancelCause` 支持原因。

### 推荐的 Go 适配策略

- **统一命令队列**：使用 `sync.RWMutex` 保护的切片 + `Signal` 广播，通过 BubbleTea 的 `tea.Cmd` 轮询队列变更
- **QueryGuard**：直接翻译为 `sync.Mutex` 保护的状态机，代次机制同样适用
- **Mid-Turn Drain**：在引擎的 `runQueryLoop` 中添加检查点，通过注入的 `*MessageQueue` 引用访问队列
- **Between-Turn Drain**：在 `StreamDoneMsg` 处理中检查队列并返回新的 `tea.Cmd`
- **Early Input**：使用 `golang.org/x/term` 包的 `MakeRaw` + goroutine 读取

---

## 相关 Issue

完成本文档后，建议创建以下 GitHub Issues：

1. **[Epic] Mid-Session 消息处理架构** — 跟踪整体进度
2. **[P0] 实现统一命令队列（MessageQueue）** — 核心基础设施
3. **[P1] 实现 QueryGuard 并发守卫** — 查询状态管理
4. **[P1] 实现 handleSubmit 入队分支** — 最小可用改动
5. **[P1] 实现 Mid-Turn Drain** — 引擎层消息注入
6. **[P1] 实现 Between-Turn Drain** — 回合间队列处理
7. **[P2] 实现 Early Input 缓冲** — 启动阶段输入捕获
8. **[P2] 实现排队消息预览 UI** — 用户反馈
9. **[P2] 完善 Print 模式多消息处理** — 非交互式支持

---

## 变更历史

| 日期 | 版本 | 变更内容 |
|------|------|----------|
| 2026-04-27 | v1.0 | 初始版本，完成差异分析（覆盖 17 个功能点、6 个代码质量问题） |
