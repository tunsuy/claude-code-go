# Claude Code 会话进行中接收用户消息的架构设计深度分析

> 本文档深入分析 Claude Code CLI 如何在一次会话（turn）正在执行（Claude 正在生成回复或执行工具）的过程中，仍然能够接收、缓冲、路由和处理用户发送的新消息。涵盖设计理念、核心技术实现、完整的数据流以及与各子系统的集成方式。

---

## 目录

1. [设计理念与宏观架构](#1-设计理念与宏观架构)
2. [核心组件总览](#2-核心组件总览)
3. [统一命令队列 — messageQueueManager](#3-统一命令队列--messagequeumanager)
4. [QueryGuard — 并发守卫状态机](#4-queryguard--并发守卫状态机)
5. [用户输入的完整生命周期](#5-用户输入的完整生命周期)
6. [优先级系统与三级调度策略](#6-优先级系统与三级调度策略)
7. [Mid-Turn Drain — 回合内消息注入](#7-mid-turn-drain--回合内消息注入)
8. [Between-Turn Drain — 回合间队列处理](#8-between-turn-drain--回合间队列处理)
9. [中断与取消机制](#9-中断与取消机制)
10. [UI 层的实时呈现](#10-ui-层的实时呈现)
11. [Early Input — 启动阶段的击键捕获](#11-early-input--启动阶段的击键捕获)
12. [后台 Agent 任务的消息队列](#12-后台-agent-任务的消息队列)
13. [与 CLI Print 模式的集成](#13-与-cli-print-模式的集成)
14. [关键文件索引](#14-关键文件索引)
15. [数据流时序图](#15-数据流时序图)
16. [总结](#16-总结)

---

## 1. 设计理念与宏观架构

### 1.1 核心问题

在传统的 CLI 交互模型中，用户输入和 AI 回复是严格串行的：用户发送 → AI 回复 → 用户再发送。但在 Claude Code 这样的复杂工具中，一次 "turn"（回合）可能持续数十秒甚至数分钟（执行多个工具调用、文件读写、命令执行等）。如果用户必须等待回合完全结束才能追加输入，体验将会极差。

### 1.2 设计理念

Claude Code 采用了以下核心设计理念来解决这个问题：

**① 永不丢失输入（Never Lose Input）**
- 从进程启动的第一个毫秒开始就在捕获用户输入（Early Input）
- 在 REPL 初始化完成前的击键也会被缓冲
- 在 AI 回复过程中的输入也会被排队

**② 优先级驱动的统一队列（Priority-Based Unified Queue）**
- 所有待处理的命令——无论是用户手动输入、系统任务通知、还是孤立的权限请求——都通过同一个队列管理
- 通过三级优先级（`now` > `next` > `later`）实现精细的调度控制

**③ 非阻塞 UI（Non-Blocking UI）**
- 基于 React/Ink 的终端 UI 框架天然支持异步渲染
- 输入框组件（TextInput）始终可用，不会因为后台处理而被锁定
- 使用 `useSyncExternalStore` 绕过 React Context 传播延迟

**④ 多点注入（Multi-Point Injection）**
- 用户消息不仅可以在回合之间处理，还可以在回合进行中的工具执行间隙被注入
- 高优先级消息（`now`）甚至可以中断当前正在进行的流式响应

**⑤ 渐进式响应（Progressive Response）**
- 排队的消息会在 UI 中实时预览，让用户知道消息已被接收
- 消息按优先级从队列中取出，保证紧急的先被处理

### 1.3 宏观架构图

```
                      ┌──────────────────────────────────────────────┐
                      │              Terminal (stdin)                 │
                      └──────────────┬───────────────────────────────┘
                                     │ 键盘输入
                      ┌──────────────▼───────────────────────────────┐
                      │         Early Input Buffer                    │
                      │    (启动阶段: 在 REPL 就绪前捕获击键)          │
                      └──────────────┬───────────────────────────────┘
                                     │ consumeEarlyInput()
                      ┌──────────────▼───────────────────────────────┐
                      │    TextInput / PromptInput (React/Ink)        │
                      │    用户输入框 — 始终可用，永不被锁定              │
                      └──────────────┬───────────────────────────────┘
                                     │ onSubmit → handlePromptSubmit()
                      ┌──────────────▼───────────────────────────────┐
                      │         handlePromptSubmit()                  │
                      │   ┌──────────────────────────────────────┐    │
                      │   │  queryGuard.isActive?                │    │
                      │   │   ├─ YES → enqueue() 入队列            │    │
                      │   │   └─ NO  → executeUserInput() 直接执行 │    │
                      │   └──────────────────────────────────────┘    │
                      └────────┬─────────────────────┬───────────────┘
                               │                     │
                   ┌───────────▼─────────┐  ┌───────▼──────────────┐
                   │  Unified Command    │  │  executeUserInput()   │
                   │  Queue (模块级单例)  │  │  → processUserInput() │
                   │                     │  │  → onQuery()          │
                   │  优先级排序:          │  └───────┬──────────────┘
                   │  now > next > later  │          │
                   └────┬────────────────┘  ┌───────▼──────────────┐
                        │                   │     query.ts          │
         ┌──────────────┤                   │  API 调用循环          │
         │              │                   │                       │
         │    ┌─────────▼─────────┐         │  ┌─────────────────┐  │
         │    │ useQueueProcessor │         │  │ Mid-Turn Drain  │  │
         │    │ (回合间触发)       │         │  │ (回合内注入)     │  │
         │    │ 条件:             │         │  │ 工具执行间隙检查  │  │
         │    │ - 无活跃查询      │         │  │ 队列中的消息     │  │
         │    │ - 队列非空        │         │  └────────┬────────┘  │
         │    │ - 无活跃 JSX UI   │         │           │           │
         │    └──────────────────┘         └───────────┘           │
         │                                                          │
         │    ┌─────────────────────────────────────────────┐       │
         └────► print.ts subscribeToCommandQueue()          │       │
              │ 监听 'now' 优先级 → abort('interrupt')      │       │
              └─────────────────────────────────────────────┘
```

---

## 2. 核心组件总览

| 组件 | 文件 | 职责 |
|------|------|------|
| **统一命令队列** | `src/utils/messageQueueManager.ts` | 模块级单例，存储所有待处理命令，支持优先级排序 |
| **QueryGuard** | `src/utils/QueryGuard.ts` | 三态状态机，防止并发查询竞争 |
| **handlePromptSubmit** | `src/utils/handlePromptSubmit.ts` | 输入提交入口，决定立即执行还是入队 |
| **queueProcessor** | `src/utils/queueProcessor.ts` | 出队逻辑：决定批处理还是逐条处理 |
| **useQueueProcessor** | `src/hooks/useQueueProcessor.ts` | React Hook，在回合间自动触发队列处理 |
| **CancelRequestHandler** | `src/hooks/useCancelRequest.ts` | Escape/Ctrl+C 处理，优先级中断 |
| **PromptInputQueuedCommands** | `src/components/PromptInput/PromptInputQueuedCommands.tsx` | 队列预览 UI，展示已排队的消息 |
| **earlyInput** | `src/utils/earlyInput.ts` | 启动阶段击键缓冲 |
| **query.ts** | `src/query.ts` | API 调用循环，包含 mid-turn drain 逻辑 |
| **print.ts** | `src/cli/print.ts` | CLI 打印模式，包含流式中断和 between-turn drain |
| **Signal** | `src/utils/signal.ts` | 轻量级事件发布-订阅原语 |

---

## 3. 统一命令队列 — messageQueueManager

### 3.1 核心设计

文件: `src/utils/messageQueueManager.ts`

这是整个消息排队系统的心脏。它是一个**模块级单例**（module-level singleton），完全独立于 React 状态树，这意味着：

- 无论 React 组件是否已渲染，队列始终可用
- 非 React 代码（如 `print.ts` 的流式循环）可以直接读写
- React 组件通过 `useSyncExternalStore` 订阅变化

```typescript
// 核心数据结构
const commandQueue: QueuedCommand[] = []                    // 可变数组，真实存储
let snapshot: readonly QueuedCommand[] = Object.freeze([])  // 冻结快照，用于 React
const queueChanged = createSignal()                         // 变更信号

function notifySubscribers(): void {
  snapshot = Object.freeze([...commandQueue])  // 每次变更都创建新的冻结快照
  queueChanged.emit()                          // 通知所有订阅者
}
```

**为什么使用冻结快照？**

React 的 `useSyncExternalStore` 要求 `getSnapshot` 返回的值在没有变化时保持引用相等（referentially equal）。通过在每次变更时创建新的 `Object.freeze([...commandQueue])`，确保：
- 无变更时返回同一个引用 → React 不重渲染
- 有变更时返回新的引用 → React 触发重渲染

### 3.2 QueuedCommand 类型

```typescript
type QueuedCommand = {
  value: string | Array<ContentBlockParam>  // 消息内容（文本或富内容块）
  mode: PromptInputMode                     // 'bash' | 'prompt' | 'orphaned-permission' | 'task-notification'
  priority?: QueuePriority                  // 'now' | 'next' | 'later'
  uuid?: UUID                              // 唯一标识，用于生命周期追踪
  pastedContents?: Record<number, PastedContent>  // 粘贴的图片等
  preExpansionValue?: string                // 占位符展开前的原始输入
  skipSlashCommands?: boolean               // 远程消息不触发本地斜杠命令
  bridgeOrigin?: boolean                    // 来自移动端/Web 的桥接消息
  isMeta?: boolean                          // 系统生成的消息（对模型可见但在 UI 中隐藏）
  origin?: MessageOrigin                    // 来源标记（keyboard/channel/task-notification等）
  workload?: string                         // 计费工作负载标签
  agentId?: AgentId                         // 目标 Agent（undefined = 主线程）
}
```

### 3.3 关键操作

**入队（enqueue）：**
```typescript
export function enqueue(command: QueuedCommand): void {
  commandQueue.push({ ...command, priority: command.priority ?? 'next' })
  notifySubscribers()
  logOperation('enqueue', ...)
}
```
- 用户输入默认优先级为 `'next'`
- 任务通知通过 `enqueuePendingNotification()` 入队，默认优先级为 `'later'`

**出队（dequeue）：**
```typescript
export function dequeue(
  filter?: (cmd: QueuedCommand) => boolean,
): QueuedCommand | undefined {
  // 在所有命令中找到优先级最高的（可选过滤器）
  let bestIdx = -1
  let bestPriority = Infinity
  for (let i = 0; i < commandQueue.length; i++) {
    const cmd = commandQueue[i]!
    if (filter && !filter(cmd)) continue
    const priority = PRIORITY_ORDER[cmd.priority ?? 'next']
    if (priority < bestPriority) {
      bestIdx = i
      bestPriority = priority
    }
  }
  if (bestIdx === -1) return undefined
  const [dequeued] = commandQueue.splice(bestIdx, 1)
  notifySubscribers()
  return dequeued
}
```
- 总是取出优先级最高的命令
- 同优先级内保持 FIFO 顺序
- 支持可选过滤器（例如只取主线程的命令）

**按优先级批量获取（不移除）：**
```typescript
export function getCommandsByMaxPriority(
  maxPriority: QueuePriority,
): QueuedCommand[] {
  const threshold = PRIORITY_ORDER[maxPriority]
  return commandQueue.filter(
    cmd => PRIORITY_ORDER[cmd.priority ?? 'next'] <= threshold,
  )
}
```
- `'now'` → 只返回 now 优先级
- `'next'` → 返回 now + next
- `'later'` → 返回所有

### 3.4 useSyncExternalStore 接口

```typescript
export const subscribeToCommandQueue = queueChanged.subscribe

export function getCommandQueueSnapshot(): readonly QueuedCommand[] {
  return snapshot
}
```

React 组件通过以下方式订阅：
```typescript
// src/hooks/useCommandQueue.ts
export function useCommandQueue(): readonly QueuedCommand[] {
  return useSyncExternalStore(subscribeToCommandQueue, getCommandQueueSnapshot)
}
```

**为什么选择 `useSyncExternalStore` 而不是 `useState` + `useEffect`？**

在 Ink（React 的终端渲染器）中，Context 的传播存在延迟。`useSyncExternalStore` 提供了同步的状态订阅，确保：
1. 状态变化立即反映在 React 渲染中
2. 不会因为 React 批处理导致错过队列变化
3. 在 `useEffect` 中读取到的永远是最新状态

---

## 4. QueryGuard — 并发守卫状态机

文件: `src/utils/QueryGuard.ts`

### 4.1 问题背景

当用户在 AI 回复过程中发送消息时，`handlePromptSubmit` 会被再次调用。如果不做任何控制，可能会出现两个 `executeUserInput` 同时运行的情况，导致消息顺序混乱、abort controller 冲突等问题。

### 4.2 三态设计

```
idle（空闲）→ dispatching（派发中）→ running（运行中）→ idle
                   ↓
              idle（取消保留）
```

```typescript
export class QueryGuard {
  private _status: 'idle' | 'dispatching' | 'running' = 'idle'
  private _generation = 0  // 代次计数器，防止过期的 finally 清理

  reserve(): boolean {
    if (this._status !== 'idle') return false
    this._status = 'dispatching'
    this._notify()
    return true
  }

  cancelReservation(): void {
    if (this._status !== 'dispatching') return
    this._status = 'idle'
    this._notify()
  }

  tryStart(): number | null {
    if (this._status === 'running') return null
    this._status = 'running'
    ++this._generation
    this._notify()
    return this._generation
  }

  end(generation: number): boolean {
    if (this._generation !== generation) return false
    if (this._status !== 'running') return false
    this._status = 'idle'
    this._notify()
    return true
  }

  get isActive(): boolean {
    return this._status !== 'idle'
  }
}
```

**为什么需要 `dispatching` 中间态？**

从队列处理器（`useQueueProcessor`）出队到 `executeUserInput` 的第一个 `await` 之间有一个同步间隙。如果没有 `dispatching` 状态，另一个 `handlePromptSubmit` 调用可能在这个间隙中看到 `isActive === false`，从而也开始执行。`reserve()` 在出队前锁定状态，确保这个间隙被覆盖。

**为什么需要 `generation` 代次？**

当用户取消一个查询时（`forceEnd`），被取消的查询可能仍有一个 `finally` 块在排队等待运行。如果 `finally` 里的 `end()` 不检查代次，它可能会把新启动的查询也结束掉。`generation` 确保只有当前代次的查询才能结束自己。

---

## 5. 用户输入的完整生命周期

### 5.1 场景一：系统空闲时的直接提交

```
用户按下 Enter
    ↓
handlePromptSubmit(input, mode, ...)
    ↓
queryGuard.isActive === false → 直接执行
    ↓
构建 QueuedCommand { value: expandedInput, mode, pastedContents }
    ↓
executeUserInput([cmd])
    ↓
queryGuard.reserve()              ← 锁定守卫
    ↓
processUserInput() 循环           ← 处理斜杠命令/Bash/普通文本
    ↓
生成 newMessages[]
    ↓
onQuery(newMessages, abortController, shouldQuery, ...)
    ↓
query.ts 的 API 循环开始
    ↓
... Claude 生成回复、执行工具等 ...
    ↓
queryGuard.end(generation)        ← 解锁守卫
```

### 5.2 场景二：系统忙碌时的排队

```
用户按下 Enter（此时 Claude 正在处理上一个请求）
    ↓
handlePromptSubmit(input, mode, ...)
    ↓
queryGuard.isActive === true → 需要排队
    ↓
检查 hasInterruptibleToolInProgress?
    ├─ YES → abortController.abort('interrupt') 中断当前工具
    └─ NO  → 不中断
    ↓
enqueue({
  value: finalInput.trim(),
  mode,
  priority: 'next',        ← 用户输入默认 next 优先级
  pastedContents,
})
    ↓
清除输入框、重置光标、清空粘贴内容
    ↓
UI 中立即在输入框下方显示排队消息预览
    ↓
[等待当前回合结束]
    ↓
useQueueProcessor 检测到:
  - isQueryActive === false ← queryGuard 变为 idle
  - queueSnapshot.length > 0
  - !hasActiveLocalJsxUI
    ↓
processQueueIfReady({ executeInput })
    ↓
从队列中取出最高优先级命令并执行
```

### 5.3 关键代码: handlePromptSubmit 的排队路径

```typescript
// src/utils/handlePromptSubmit.ts

if (queryGuard.isActive || isExternalLoading) {
  // 只允许 prompt 和 bash 模式的命令被排队
  if (mode !== 'prompt' && mode !== 'bash') {
    return
  }

  // 如果当前所有执行中的工具都是可中断的（如 SleepTool），
  // 中断当前回合
  if (params.hasInterruptibleToolInProgress) {
    params.abortController?.abort('interrupt')
  }

  // 入队：字符串值 + 原始粘贴内容。图片会在执行时调整大小。
  enqueue({
    value: finalInput.trim(),
    preExpansionValue: input.trim(),
    mode,
    pastedContents: hasImages ? pastedContents : undefined,
    skipSlashCommands,
    uuid,
  })

  // 立即清除输入框（给用户即时反馈）
  onInputChange('')
  setCursorOffset(0)
  setPastedContents({})
  resetHistory()
  clearBuffer()
  return
}
```

---

## 6. 优先级系统与三级调度策略

### 6.1 三级优先级定义

```typescript
type QueuePriority = 'now' | 'next' | 'later'

const PRIORITY_ORDER: Record<QueuePriority, number> = {
  now: 0,    // 最高优先级
  next: 1,   // 中等优先级
  later: 2,  // 最低优先级
}
```

### 6.2 各级别的语义

#### `now` — 立即中断并执行

- **行为**: 中止当前的工具调用，立即执行此命令（等同于用户按 Esc + 发送）
- **触发**: `print.ts` 中的 `subscribeToCommandQueue` 监听器检测到 `now` 优先级命令时，调用 `abortController.abort('interrupt')` 中断流式响应
- **使用场景**: 紧急命令

```typescript
// src/cli/print.ts — 'now' 优先级的中断监听
subscribeToCommandQueue(() => {
  if (abortController && getCommandsByMaxPriority('now').length > 0) {
    abortController.abort('interrupt')
  }
})
```

#### `next` — 回合内/回合间处理

- **行为**: 让当前工具调用完成，然后在下一次 API 调用之前注入此消息
- **触发**: `query.ts` 的 mid-turn drain 点（工具执行完成后、递归前）
- **默认适用**: 用户手动输入的消息
- **使用场景**: 用户在 AI 执行工具时追加的指令

```typescript
// src/query.ts — mid-turn drain（回合内注入点）
const queuedCommandsSnapshot = getCommandsByMaxPriority(
  sleepRan ? 'later' : 'next',  // Sleep 后扩大到 later
).filter(cmd => {
  if (isSlashCommand(cmd)) return false
  if (isMainThread) return cmd.agentId === undefined
  return cmd.mode === 'task-notification' && cmd.agentId === currentAgentId
})
```

#### `later` — 回合结束后处理

- **行为**: 等待当前完整回合结束，然后作为新查询处理
- **默认适用**: 系统任务通知（后台 Agent 完成通知等）
- **使用场景**: 不紧急的系统消息，避免饿死用户输入

```typescript
// 任务通知以 'later' 优先级入队
export function enqueuePendingNotification(command: QueuedCommand): void {
  commandQueue.push({ ...command, priority: command.priority ?? 'later' })
  notifySubscribers()
}
```

### 6.3 优先级驱动的调度时序

```
时间轴 ──────────────────────────────────────────────────►

                   ┌─────── Turn 1（API 调用 + 工具执行）───────┐
                   │                                            │
用户输入 A ────────┤► enqueue(A, priority: 'next')              │
                   │                                            │
Task通知 X ────────┤► enqueue(X, priority: 'later')             │
                   │                                            │
                   │    ┌─ 工具执行完成 ─┐                       │
                   │    │ Mid-Turn Drain │                       │
                   │    │ 检查 'next' 级 │                       │
                   │    │ → 找到 A       │                       │
                   │    │ → A 作为附件   │                       │
                   │    │   注入本回合   │                       │
                   │    └────────────────┘                       │
                   │                                            │
                   └────────────────────────────────────────────┘
                                                                │
                   ┌─────── Between-Turn Drain ──────────────────┤
                   │ useQueueProcessor 触发                      │
                   │ 检查队列 → 找到 X (later)                   │
                   │ → 作为新查询启动 Turn 2                     │
                   └────────────────────────────────────────────┘
```

---

## 7. Mid-Turn Drain — 回合内消息注入

### 7.1 概念

Mid-Turn Drain 是 Claude Code 最精巧的设计之一。它允许在一次 API 调用回合**正在进行中**（工具已执行完毕，即将发起下一次 API 调用）时，检查队列中是否有待处理的消息，并将这些消息作为**附件（attachment）**注入到当前回合的上下文中。

这意味着 Claude 不需要等到当前回合结束再看到用户的追加消息——它可以在工具执行的间隙就看到。

### 7.2 实现位置

文件: `src/query.ts`，在工具执行完成之后、递归继续下一次 API 调用之前

### 7.3 核心代码

```typescript
// ① 确定 drain 的优先级阈值
const sleepRan = toolUseBlocks.some(b => b.name === SLEEP_TOOL_NAME)
const isMainThread =
  querySource.startsWith('repl_main_thread') || querySource === 'sdk'
const currentAgentId = toolUseContext.agentId

// ② 获取符合条件的待处理命令的快照（不移除）
const queuedCommandsSnapshot = getCommandsByMaxPriority(
  sleepRan ? 'later' : 'next',  // Sleep 后扩大 drain 范围
).filter(cmd => {
  if (isSlashCommand(cmd)) return false            // 斜杠命令不在此处理
  if (isMainThread) return cmd.agentId === undefined  // 主线程只处理自己的
  return cmd.mode === 'task-notification' && cmd.agentId === currentAgentId
})

// ③ 将队列中的命令转换为附件消息
for await (const attachment of getAttachmentMessages(
  null,
  updatedToolUseContext,
  null,
  queuedCommandsSnapshot,       // ← 传入快照
  [...messagesForQuery, ...assistantMessages, ...toolResults],
  querySource,
)) {
  yield attachment
  toolResults.push(attachment)
}

// ④ 从队列中移除已消费的命令
const consumedCommands = queuedCommandsSnapshot.filter(
  cmd => cmd.mode === 'prompt' || cmd.mode === 'task-notification',
)
if (consumedCommands.length > 0) {
  for (const cmd of consumedCommands) {
    if (cmd.uuid) {
      consumedCommandUuids.push(cmd.uuid)
      notifyCommandLifecycle(cmd.uuid, 'started')
    }
  }
  removeFromQueue(consumedCommands)
}
```

### 7.4 技术要点

**Sleep Tool 的特殊处理**: 当 SleepTool 执行后（自主/主动模式下），drain 的阈值从 `'next'` 扩大到 `'later'`。这是因为 Sleep 意味着"我现在没事做"，所以即使是低优先级的任务通知也应该被处理。

**Agent 作用域隔离**: 队列是进程全局单例，但主线程和子 Agent 共享它。Mid-turn drain 通过 `agentId` 过滤确保每个循环只处理自己的命令：
- 主线程只 drain `agentId === undefined` 的命令
- 子 Agent 只 drain 地址指向自己的 `task-notification`

**斜杠命令排除**: 斜杠命令不在 mid-turn drain 中处理，因为它们需要走完整的 `processSlashCommand` 路径（可能涉及 UI 交互）。

---

## 8. Between-Turn Drain — 回合间队列处理

### 8.1 REPL 模式（交互式）

文件: `src/hooks/useQueueProcessor.ts`

```typescript
export function useQueueProcessor({
  executeQueuedInput,
  hasActiveLocalJsxUI,
  queryGuard,
}: UseQueueProcessorParams): void {
  // 订阅 QueryGuard 状态
  const isQueryActive = useSyncExternalStore(
    queryGuard.subscribe,
    queryGuard.getSnapshot,
  )

  // 订阅命令队列
  const queueSnapshot = useSyncExternalStore(
    subscribeToCommandQueue,
    getCommandQueueSnapshot,
  )

  useEffect(() => {
    if (isQueryActive) return           // 有查询在运行 → 等待
    if (hasActiveLocalJsxUI) return     // 有 JSX UI 在显示 → 等待
    if (queueSnapshot.length === 0) return  // 队列为空 → 无事可做

    processQueueIfReady({ executeInput: executeQueuedInput })
  }, [queueSnapshot, isQueryActive, executeQueuedInput, hasActiveLocalJsxUI, queryGuard])
}
```

**触发条件**: 三个条件全部满足时触发：
1. 没有活跃的查询（`isQueryActive === false`）
2. 队列中有待处理的命令
3. 没有活跃的本地 JSX UI（如 `/model` 选择器）

### 8.2 队列处理器的批处理逻辑

文件: `src/utils/queueProcessor.ts`

```typescript
export function processQueueIfReady({
  executeInput,
}: ProcessQueueParams): ProcessQueueResult {
  // 只处理主线程的命令
  const isMainThread = (cmd: QueuedCommand) => cmd.agentId === undefined

  const next = peek(isMainThread)
  if (!next) return { processed: false }

  // 斜杠命令和 Bash 命令：逐条处理
  if (isSlashCommand(next) || next.mode === 'bash') {
    const cmd = dequeue(isMainThread)!
    void executeInput([cmd])
    return { processed: true }
  }

  // 其他命令：同模式批量处理
  const targetMode = next.mode
  const commands = dequeueAllMatching(
    cmd => isMainThread(cmd) && !isSlashCommand(cmd) && cmd.mode === targetMode,
  )
  if (commands.length === 0) return { processed: false }

  void executeInput(commands)
  return { processed: true }
}
```

**设计决策**:
- **斜杠命令逐条处理**: 因为每个斜杠命令可能修改队列（比如产生新的命令）
- **Bash 命令逐条处理**: 需要独立的错误隔离、退出码和进度 UI
- **同模式普通命令批量处理**: 所有 `prompt` 模式的命令被一次性取出，作为数组传给 `executeInput`。每个命令会变成一条独立的用户消息（有自己的 UUID），但它们在同一个 API 调用中发送

### 8.3 Print 模式（非交互式/CI）

文件: `src/cli/print.ts`

```typescript
const drainCommandQueue = async () => {
  while ((command = dequeue(isMainThread))) {
    const batch: QueuedCommand[] = [command]
    if (command.mode === 'prompt') {
      // 贪心批量合并同类命令
      while (canBatchWith(command, peek(isMainThread))) {
        batch.push(dequeue(isMainThread)!)
      }
    }
    // 处理批次...
    // 调用 ask() → query() 发起新的 API 调用
  }
}

// 主循环
do {
  for (const event of drainSdkEvents()) {
    output.enqueue(event)
  }
  runPhase = 'draining_commands'
  await drainCommandQueue()
} while (waitingForAgents)
```

---

## 9. 中断与取消机制

文件: `src/hooks/useCancelRequest.ts`

### 9.1 Escape 键优先级处理

```typescript
const handleCancel = useCallback(() => {
  // 优先级 1: 如果有正在运行的任务，先取消它
  if (abortSignal !== undefined && !abortSignal.aborted) {
    setToolUseConfirmQueue(() => [])
    onCancel()
    return
  }

  // 优先级 2: 弹出队列（Claude 空闲时从队列中移除最后一条）
  if (hasCommandsInQueue()) {
    if (popCommandFromQueue) {
      popCommandFromQueue()
      return
    }
  }

  // 回退: 没有可取消/弹出的内容
  setToolUseConfirmQueue(() => [])
  onCancel()
}, [abortSignal, popCommandFromQueue, ...])
```

### 9.2 `popCommandFromQueue` — 撤回排队消息

当用户按 Escape 且 Claude 处于空闲状态时，调用 `popAllEditable()`。这个函数会：
1. 从队列中取出所有可编辑的命令（非任务通知的命令）
2. 将它们的文本和当前输入框文本合并
3. 将合并后的文本放回输入框
4. 恢复光标位置

```typescript
export function popAllEditable(
  currentInput: string,
  currentCursorOffset: number,
): PopAllEditableResult | undefined {
  const { editable = [], nonEditable = [] } = objectGroupBy(
    [...commandQueue],
    cmd => (isQueuedCommandEditable(cmd) ? 'editable' : 'nonEditable'),
  )

  if (editable.length === 0) return undefined

  const queuedTexts = editable.map(cmd => extractTextFromValue(cmd.value))
  const newInput = [...queuedTexts, currentInput].filter(Boolean).join('\n')

  // 只保留不可编辑的命令在队列中
  commandQueue.length = 0
  commandQueue.push(...nonEditable)
  notifySubscribers()

  return { text: newInput, cursorOffset, images }
}
```

### 9.3 Ctrl+C（app:interrupt）

```typescript
const handleInterrupt = useCallback(() => {
  if (isViewingTeammate) {
    killAllAgentsAndNotify()    // 杀死所有后台 Agent
    exitTeammateView(setAppState)  // 退出队友视图
  }
  if (canCancelRunningTask || hasQueuedCommands) {
    handleCancel()
  }
}, [...])
```

### 9.4 chat:killAgents — 双击确认杀死所有后台 Agent

```typescript
const handleKillAgents = useCallback(() => {
  const now = Date.now()
  const elapsed = now - lastKillAgentsPressRef.current
  if (elapsed <= KILL_AGENTS_CONFIRM_WINDOW_MS) {  // 3 秒窗口
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

### 9.5 AbortController 的传播链

```
handlePromptSubmit
  ↓ createAbortController()
  ↓ setAbortController(abortController)
  ↓
REPL.tsx
  ↓ abortController 存入 state
  ↓ 传给 CancelRequestHandler（onCancel 调用 abort）
  ↓
onQuery(newMessages, abortController, ...)
  ↓
query.ts
  ↓ toolUseContext.abortController
  ↓ 传给 callModel()（作为 signal）
  ↓ 传给 streamingToolExecutor
  ↓
print.ts
  ↓ subscribeToCommandQueue 监听 'now' 优先级
  ↓ → abortController.abort('interrupt')
```

---

## 10. UI 层的实时呈现

### 10.1 PromptInputQueuedCommands — 队列预览组件

文件: `src/components/PromptInput/PromptInputQueuedCommands.tsx`

当用户在 Claude 处理过程中发送消息后，消息会被排队。为了给用户即时反馈（"我的消息已经被接收了"），UI 会在输入框下方显示队列中的消息预览。

```typescript
function PromptInputQueuedCommandsImpl(): React.ReactNode {
  const queuedCommands = useCommandQueue()  // 订阅队列变化
  const viewingAgent = useAppState(s => !!s.viewingAgentTaskId)

  // 使用 useMemo 避免 UUID 重新生成导致的闪烁
  const messages = useMemo(() => {
    if (queuedCommands.length === 0) return null

    // 过滤出可见的命令（排除 isMeta 的系统消息）
    const visibleCommands = queuedCommands.filter(isQueuedCommandVisible)
    if (visibleCommands.length === 0) return null

    // 任务通知最多显示 3 条，超出的合并为摘要
    const processedCommands = processQueuedCommands(visibleCommands)

    return normalizeMessages(
      processedCommands.map(cmd => {
        let content = cmd.value
        if (cmd.mode === 'bash' && typeof content === 'string') {
          content = `<bash-input>${content}</bash-input>`
        }
        return createUserMessage({ content })
      }),
    )
  }, [queuedCommands])

  // 在查看 Agent 时不显示主线程的排队命令
  if (viewingAgent || messages === null) return null

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

### 10.2 任务通知的溢出处理

当后台有大量 Agent 完成时，可能产生大量任务通知。为了不淹没 UI：

```typescript
const MAX_VISIBLE_NOTIFICATIONS = 3

function processQueuedCommands(queuedCommands: QueuedCommand[]): QueuedCommand[] {
  // 分离任务通知和其他命令
  const taskNotifications = filteredCommands.filter(cmd => cmd.mode === 'task-notification')
  const otherCommands = filteredCommands.filter(cmd => cmd.mode !== 'task-notification')

  // 如果通知不超过上限，全部显示
  if (taskNotifications.length <= MAX_VISIBLE_NOTIFICATIONS) {
    return [...otherCommands, ...taskNotifications]
  }

  // 显示前 2 条 + 一条 "+N more tasks completed" 摘要
  const visibleNotifications = taskNotifications.slice(0, MAX_VISIBLE_NOTIFICATIONS - 1)
  const overflowCount = taskNotifications.length - (MAX_VISIBLE_NOTIFICATIONS - 1)
  const overflowCommand: QueuedCommand = {
    value: createOverflowNotificationMessage(overflowCount),
    mode: 'task-notification',
  }
  return [...otherCommands, ...visibleNotifications, overflowCommand]
}
```

### 10.3 QueuedMessageProvider — 样式上下文

排队的消息通过 `QueuedMessageProvider` 包裹，提供不同于正常消息的视觉样式（例如 dim 颜色、不同的缩进），让用户能够区分"正在处理"和"已排队等待"。

---

## 11. Early Input — 启动阶段的击键捕获

文件: `src/utils/earlyInput.ts`

### 11.1 问题

用户经常在终端中输入 `claude` 后立即开始打字。但在 CLI 启动、React 渲染、Ink 初始化完成之前，这些击键如果不被捕获就会丢失。

### 11.2 解决方案

在 CLI 入口文件尽早调用 `startCapturingEarlyInput()`，将 stdin 切换为 raw 模式并开始缓冲。

```typescript
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
      if (typeof chunk === 'string') {
        processChunk(chunk)
      }
      chunk = process.stdin.read()
    }
  }
  process.stdin.on('readable', readableHandler)
}
```

### 11.3 输入处理

`processChunk` 逐字符处理，正确处理：
- **Ctrl+C** (code 3) → 直接 `process.exit(130)` 退出
- **Ctrl+D** (code 4) → EOF，停止捕获
- **Backspace** (code 127/8) → 删除最后一个字素簇（grapheme cluster）
- **Escape 序列** (code 27) → 跳过（箭头键、功能键等）
- **其他控制字符** → 跳过（Tab 和换行除外）
- **回车** (code 13) → 转换为换行
- **可打印字符** → 添加到缓冲

### 11.4 消费

当 REPL 准备就绪时，调用 `consumeEarlyInput()` 获取缓冲的文本并自动停止捕获：

```typescript
export function consumeEarlyInput(): string {
  stopCapturingEarlyInput()
  const input = earlyInputBuffer.trim()
  earlyInputBuffer = ''
  return input
}
```

---

## 12. 后台 Agent 任务的消息队列

文件: `src/tasks/LocalAgentTask/LocalAgentTask.tsx`

### 12.1 Agent 的独立消息缓冲

后台 Agent 有自己的 `pendingMessages` 缓冲区：

```typescript
type LocalAgentTaskState = {
  pendingMessages: string[]       // 通过 SendMessage 排队的消息
  status: 'running' | 'completed' | 'killed'
  abortController?: AbortController
  isBackgrounded: boolean
  retain: boolean
  // ...
}
```

### 12.2 排队和消费

**排队**: 当主线程使用 SendMessage 工具向后台 Agent 发送消息时：

```typescript
function queuePendingMessage(taskId, msg, setAppState) {
  // 将消息添加到 Agent 的 pendingMessages 数组
}
```

**消费**: 在 Agent 的工具轮次边界处 drain：

```typescript
function drainPendingMessages(taskId, getAppState, setAppState) {
  // 取出所有 pendingMessages
  // 附加到 Agent 的对话记录中
  // 清空 pendingMessages
}
```

### 12.3 与统一队列的关系

后台 Agent 的任务完成通知通过统一队列发送给主线程：

```typescript
// Agent 完成时
enqueuePendingNotification({
  value: summary,
  mode: 'task-notification',
  priority: 'later',          // 不紧急，等当前回合结束
  agentId: undefined,         // 发给主线程
})
```

主线程的 mid-turn drain 会根据 `agentId` 过滤，只处理属于自己的通知：
```typescript
const queuedCommandsSnapshot = getCommandsByMaxPriority(...).filter(cmd => {
  if (isMainThread) return cmd.agentId === undefined  // 主线程只处理无 agentId 的
  return cmd.agentId === currentAgentId               // 子 Agent 只处理自己的
})
```

---

## 13. 与 CLI Print 模式的集成

文件: `src/cli/print.ts`

### 13.1 Print 模式的特殊性

Print 模式（`claude -p "prompt"`）没有交互式 REPL，不渲染 React 组件。但它仍然需要：
- 监听队列中的高优先级中断
- 在回合之间 drain 待处理命令
- 支持后台 Agent 的任务通知

### 13.2 流式中断监听

```typescript
// 全局订阅：当队列中出现 'now' 优先级命令时，中断当前流
subscribeToCommandQueue(() => {
  if (abortController && getCommandsByMaxPriority('now').length > 0) {
    abortController.abort('interrupt')
  }
})
```

### 13.3 Between-Turn Drain 循环

```typescript
do {
  // 处理 SDK 事件
  for (const event of drainSdkEvents()) {
    output.enqueue(event)
  }
  // Drain 命令队列
  runPhase = 'draining_commands'
  await drainCommandQueue()
} while (waitingForAgents)  // 有后台 Agent 还在运行时继续循环
```

`drainCommandQueue()` 使用贪心批量合并同类 prompt 命令，然后逐批调用 `ask()` → `query()`。

---

## 14. 关键文件索引

| 文件 | 行数 | 核心职责 |
|------|------|----------|
| `src/utils/messageQueueManager.ts` | ~548 | 统一命令队列：入队、出队、优先级排序、快照、订阅 |
| `src/utils/QueryGuard.ts` | ~122 | 三态并发守卫：idle/dispatching/running + 代次管理 |
| `src/utils/handlePromptSubmit.ts` | ~610 | 输入提交入口：判断入队还是直接执行 |
| `src/utils/queueProcessor.ts` | ~96 | 出队策略：逐条 vs 批量、斜杠命令隔离 |
| `src/hooks/useQueueProcessor.ts` | ~68 | React Effect：回合间自动触发队列处理 |
| `src/hooks/useCommandQueue.ts` | ~15 | React Hook：订阅队列快照 |
| `src/hooks/useCancelRequest.ts` | ~277 | Escape/Ctrl+C/Kill Agents 处理 |
| `src/components/PromptInput/PromptInputQueuedCommands.tsx` | ~117 | 队列预览 UI 组件 |
| `src/utils/earlyInput.ts` | ~192 | 启动阶段击键缓冲 |
| `src/utils/signal.ts` | ~44 | 轻量级事件发布-订阅原语 |
| `src/query.ts` | ~1700+ | API 调用循环 + Mid-Turn Drain |
| `src/cli/print.ts` | ~2400+ | CLI Print 模式 + Between-Turn Drain + 流式中断 |
| `src/screens/REPL.tsx` | ~3500+ | 主 REPL 屏幕：AbortController + onQuery + Queue Processor |
| `src/types/textInputTypes.ts` | ~388 | QueuedCommand / QueuePriority / PromptInputMode 类型定义 |
| `src/tasks/LocalAgentTask/LocalAgentTask.tsx` | - | 后台 Agent 任务：pendingMessages + drain |

---

## 15. 数据流时序图

### 15.1 正常排队流程

```
时间 ─────────────────────────────────────────────────────────────►

User        PromptInput     handlePromptSubmit    Queue     QueryGuard    query.ts
  │              │                 │                 │           │            │
  │─ 键入文字 ──►│                 │                 │           │            │
  │              │                 │                 │           │            │
  │─ 按下 Enter─►│                 │                 │           │            │
  │              │─ onSubmit() ───►│                 │           │            │
  │              │                 │─ isActive? ────►│           │            │
  │              │                 │◄─ YES ──────────│           │            │
  │              │                 │                 │           │            │
  │              │                 │─ enqueue(cmd) ─►│           │            │
  │              │                 │                 │─ notify()─┤            │
  │              │◄─ clear input ──│                 │           │            │
  │              │                 │                 │           │            │
  │  [UI 显示排队消息预览]          │                 │           │            │
  │              │                 │                 │           │            │
  │              │                 │                 │           │ ─ turn 结束─┤
  │              │                 │                 │           │◄─ end() ───┤
  │              │                 │                 │           │            │
  │              │  useQueueProcessor Effect 触发     │           │            │
  │              │  isActive=false, queue.length>0    │           │            │
  │              │                 │                 │           │            │
  │              │    processQueueIfReady()           │           │            │
  │              │                 │◄─ dequeue() ────│           │            │
  │              │                 │                 │           │            │
  │              │                 │─ reserve() ────►│           │            │
  │              │                 │─ executeUserInput() ───────►│            │
  │              │                 │                 │           │──► query() │
  │              │                 │                 │           │            │
```

### 15.2 Mid-Turn Drain 流程

```
时间 ─────────────────────────────────────────────────────────────►

User        Queue         query.ts                    API
  │           │              │                          │
  │           │              │── callModel() ──────────►│
  │           │              │◄── 流式响应 + tool_use ──│
  │           │              │                          │
  │           │              │── runTools() 执行工具     │
  │           │              │                          │
  │─ 追加消息─►│              │                          │
  │  enqueue() │              │                          │
  │           │              │                          │
  │           │              │── 工具执行完成             │
  │           │              │                          │
  │           │              │── [MID-TURN DRAIN POINT]  │
  │           │◄─ getCommandsByMaxPriority('next') ─────│
  │           │─► 返回快照 [用户追加的消息] ─────────────►│
  │           │              │                          │
  │           │              │── getAttachmentMessages() │
  │           │              │   将消息转为附件           │
  │           │              │                          │
  │           │◄─ removeFromQueue(consumed) ────────────│
  │           │              │                          │
  │           │              │── callModel() ──────────►│
  │           │              │   (包含用户追加的附件)     │
  │           │              │◄── Claude 看到追加消息 ──│
  │           │              │   并在回复中响应           │
```

### 15.3 'now' 优先级中断流程

```
时间 ─────────────────────────────────────────────────────────────►

User        Queue        print.ts/REPL    AbortController    query.ts    API
  │           │              │                   │               │          │
  │           │              │                   │               │── 流式──►│
  │           │              │                   │               │◄── 流式──│
  │           │              │                   │               │          │
  │─ 紧急消息─►│              │                   │               │          │
  │  enqueue   │─ notify() ─►│                   │               │          │
  │  (now)     │              │                   │               │          │
  │           │              │─ check: 'now'>0? │               │          │
  │           │              │─ YES ────────────►│               │          │
  │           │              │  abort('interrupt')│               │          │
  │           │              │                   │── signal ────►│          │
  │           │              │                   │               │── 中止──►│
  │           │              │                   │               │          │
  │           │              │                   │               │◄─ 返回   │
  │           │              │                   │               │          │
  │           │              │   [turn 中止完成，进入 drain 阶段]  │          │
```

---

## 16. 总结

### 16.1 核心技术创新点

1. **模块级单例队列 + React 外部存储模式**: 将核心状态独立于 React 状态树，既保证了非 React 代码（`print.ts`）的直接访问，又通过 `useSyncExternalStore` 实现了 React 组件的同步订阅，避免了 Ink 终端渲染框架中 Context 传播的延迟问题。

2. **三级优先级调度**: `now`/`next`/`later` 三级优先级精准控制消息处理时机——`now` 立即中断流式传输，`next` 在工具执行间隙注入，`later` 在回合结束后处理。这不是简单的 FIFO 队列，而是根据消息的紧急程度和来源智能调度。

3. **Mid-Turn Drain**: 通过在 `query.ts` 的工具执行循环中设置检查点，在不中断当前回合的情况下将用户的追加消息作为附件注入。这意味着 Claude 可以在同一个回合中看到并响应用户的补充需求，无需等待回合结束。

4. **QueryGuard 三态状态机**: `idle → dispatching → running → idle` 的三态设计精确覆盖了从出队到执行之间的同步间隙，防止并发竞争。代次（generation）计数器确保被取消的查询不会干扰新查询的生命周期。

5. **Early Input 缓冲**: 从进程的第一毫秒开始，raw mode 下的 stdin 捕获确保了用户在 CLI 启动阶段的击键不会丢失。

6. **Agent 作用域隔离**: 共享的进程全局队列通过 `agentId` 字段实现逻辑隔离，每个消费者（主线程或子 Agent）只处理属于自己的消息。

### 16.2 一句话总结

Claude Code 通过一个优先级驱动的统一命令队列、一个三态并发守卫、多点消息注入机制（mid-turn drain + between-turn drain + now-priority abort），以及基于 `useSyncExternalStore` 的高性能 React 订阅，实现了"AI 正在工作的同时，用户可以随时追加消息"的无缝交互体验。
