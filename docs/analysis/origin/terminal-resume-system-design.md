# Claude Code 终端恢复系统设计深度分析

> 基于 `claude-code-main` 源码的完整逆向分析，涵盖架构设计理念、核心代码实现、中断机制、状态持久化、会话恢复流程及工作流集成。

---

## 目录

1. [宏观设计理念](#1-宏观设计理念)
2. [系统架构总览](#2-系统架构总览)
3. [核心组件详解](#3-核心组件详解)
   - 3.1 [会话存储层 (Session Storage)](#31-会话存储层)
   - 3.2 [中断/信号层 (Interrupt & Signal)](#32-中断信号层)
   - 3.3 [优雅关闭层 (Graceful Shutdown)](#33-优雅关闭层)
   - 3.4 [会话恢复层 (Session Restore)](#34-会话恢复层)
   - 3.5 [清理层 (Cleanup)](#35-清理层)
   - 3.6 [文件历史层 (File History)](#36-文件历史层)
   - 3.7 [工具结果持久化 (Tool Result Storage)](#37-工具结果持久化)
4. [中断触发机制](#4-中断触发机制)
   - 4.1 [用户主动中断](#41-用户主动中断)
   - 4.2 [系统内部中断](#42-系统内部中断)
   - 4.3 [外部信号中断](#43-外部信号中断)
5. [状态持久化机制](#5-状态持久化机制)
   - 5.1 [JSONL 追加式日志](#51-jsonl-追加式日志)
   - 5.2 [实时流式写入](#52-实时流式写入)
   - 5.3 [元数据管理](#53-元数据管理)
   - 5.4 [文件历史备份](#54-文件历史备份)
6. [会话恢复流程](#6-会话恢复流程)
   - 6.1 [CLI 恢复路径](#61-cli-恢复路径)
   - 6.2 [交互式恢复路径](#62-交互式恢复路径)
   - 6.3 [子代理恢复](#63-子代理恢复)
   - 6.4 [跨项目恢复](#64-跨项目恢复)
   - 6.5 [远程环境恢复 (Teleport)](#65-远程环境恢复)
7. [工作流集成](#7-工作流集成)
8. [关键技术亮点](#8-关键技术亮点)
9. [数据流图](#9-数据流图)

---

## 1. 宏观设计理念

Claude Code 的终端恢复系统建立在以下核心设计原则之上：

### 1.1 "永不丢失工作" 原则

整个系统的首要目标是：**无论进程以何种方式终止（用户取消、系统信号、崩溃、SSH断连），用户的工作都不应丢失**。为此，系统采用了多层防御策略：

- **实时追加式持久化**：每条消息在生成时即刻写入磁盘（JSONL 格式），而非在退出时批量保存
- **写入队列 + 定时刷新**：消息写入通过内部写入队列缓冲，每100ms自动刷新到磁盘
- **退出时强制刷新**：gracefulShutdown 确保在进程退出前将所有缓冲数据写盘
- **元数据尾部追加**：会话元数据在退出时重新追加到文件末尾，确保读取时始终可见

### 1.2 "确定性重建" 原则

恢复不是简单的"加载上次保存的状态"，而是**从日志中确定性重建完整会话上下文**：

- 使用 `parentUuid` 链表结构重建对话链（支持分支、压缩等复杂拓扑）
- 通过快照重建文件修改历史
- 通过工具结果引用重建大型输出
- 支持中断检测和自动续行

### 1.3 "最小侵入" 原则

恢复系统的设计目标是对正常工作流的影响最小：

- 写入操作全部异步，不阻塞主交互循环
- 使用缓冲队列批量写入，减少 I/O 开销
- 清理注册模式（Cleanup Registry）实现关注点分离
- AbortController 层级传播实现细粒度取消

### 1.4 "渐进式加载" 原则

大型会话文件可能达到数百 MB，系统采用渐进式加载策略：

- **Lite 模式**：仅读取文件头尾各64KB，提取摘要信息用于会话列表
- **Full 模式**：仅在用户选择恢复时才读取完整文件
- **尾部元数据**：关键元数据（标题、标签等）被重复追加到文件末尾，确保 Lite 读取可见

---

## 2. 系统架构总览

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          用户交互层 (UI Layer)                          │
│  ┌───────────────┐  ┌──────────────────┐  ┌────────────────────────┐   │
│  │ REPL.tsx      │  │ ResumeConversation│  │ InterruptedByUser.tsx │   │
│  │ (主交互循环)  │  │ .tsx (恢复UI)     │  │ (中断提示UI)          │   │
│  └──────┬────────┘  └────────┬─────────┘  └───────────┬────────────┘   │
│         │                    │                         │               │
├─────────┼────────────────────┼─────────────────────────┼───────────────┤
│         │            命令层 (Command Layer)             │               │
│  ┌──────┴──────────┐  ┌──────┴────────┐  ┌────────────┴──────────┐   │
│  │ /resume 命令    │  │ /session 命令 │  │ /rewind 命令         │   │
│  │ (resume.tsx)    │  │ (session.tsx) │  │ (rewind.ts)          │   │
│  └──────┬──────────┘  └──────┬────────┘  └────────────┬──────────┘   │
│         │                    │                         │               │
├─────────┼────────────────────┼─────────────────────────┼───────────────┤
│         │               Hook 层 (Hooks Layer)          │               │
│  ┌──────┴──────────┐  ┌──────┴────────┐  ┌────────────┴──────────┐   │
│  │useCancelRequest │  │useExitOnCtrlCD│  │useSessionBackgrounding│   │
│  │(取消请求hook)   │  │(退出处理hook) │  │(后台会话hook)         │   │
│  └──────┬──────────┘  └──────┬────────┘  └────────────┬──────────┘   │
│         │                    │                         │               │
├─────────┼────────────────────┼─────────────────────────┼───────────────┤
│                          核心层 (Core Layer)                           │
│  ┌───────────────┐  ┌──────────────────┐  ┌──────────────────────┐   │
│  │sessionRestore │  │conversationReco- │  │abortController.ts    │   │
│  │.ts (恢复逻辑) │  │very.ts (恢复入口)│  │(取消信号管理)        │   │
│  └──────┬────────┘  └────────┬─────────┘  └──────────┬───────────┘   │
│         │                    │                        │               │
├─────────┼────────────────────┼────────────────────────┼───────────────┤
│                        持久化层 (Persistence Layer)                     │
│  ┌───────────────┐  ┌──────────────────┐  ┌──────────────────────┐   │
│  │sessionStorage │  │toolResultStorage │  │fileHistory.ts        │   │
│  │.ts (会话存储) │  │.ts (工具结果存储)│  │(文件修改历史)        │   │
│  └──────┬────────┘  └────────┬─────────┘  └──────────┬───────────┘   │
│         │                    │                        │               │
├─────────┼────────────────────┼────────────────────────┼───────────────┤
│                     基础设施层 (Infrastructure Layer)                    │
│  ┌───────────────┐  ┌──────────────────┐  ┌──────────────────────┐   │
│  │gracefulShut-  │  │cleanupRegistry   │  │crossProjectResume    │   │
│  │down.ts (关闭) │  │.ts (清理注册)    │  │.ts (跨项目恢复)      │   │
│  └───────────────┘  └──────────────────┘  └──────────────────────┘   │
│                                                                       │
│  磁盘: ~/.claude/projects/<project-hash>/<session-id>.jsonl           │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 核心组件详解

### 3.1 会话存储层

**核心文件**: `src/utils/sessionStorage.ts` (~5100行, 180KB)

这是整个恢复系统的基石，负责会话数据的实时持久化和读取。

#### 3.1.1 存储格式 — JSONL 追加式日志

会话数据以 **JSONL (JSON Lines)** 格式存储在 `~/.claude/projects/<project-hash>/<session-id>.jsonl` 文件中。每行是一个独立的 JSON 对象（Entry），类型包括：

| Entry 类型 | 说明 |
|---|---|
| `user` | 用户消息 |
| `assistant` | AI 助手回复 |
| `attachment` | 附件（文件、目录、技能等） |
| `system` | 系统消息（压缩边界、轮次时长等） |
| `file-history-snapshot` | 文件修改历史快照 |
| `content-replacement` | 大型工具结果的磁盘引用 |
| `custom-title` | 用户自定义会话标题 |
| `tag` | 会话标签 |
| `agent-name` / `agent-color` / `agent-setting` | Agent 配置 |
| `mode` | 协调器/普通模式 |
| `worktree-state` | Git Worktree 状态 |
| `last-prompt` | 最后一条用户提示 |
| `summary` | 会话摘要 |

#### 3.1.2 消息链结构 — parentUuid DAG

每条转录消息（TranscriptMessage）包含：
- `uuid`: 唯一标识
- `parentUuid`: 指向父消息的链接
- `isSidechain`: 是否为侧链（分支对话）
- `sessionId`, `timestamp`, `cwd`, `gitBranch` 等上下文信息

这种设计形成了一个 **有向无环图 (DAG)**，支持：
- **线性对话**: 简单的 parent → child 链
- **分支对话**: 多个 leaf 从同一 parent 分出
- **压缩后的对话**: compact_boundary 断开旧链，新链从 boundary 开始
- **Snip 操作**: 删除中间段后通过 parentUuid 重新链接

```
buildConversationChain(messages, leafMessage):
  从 leafMessage 出发
  沿 parentUuid 链向上遍历
  收集所有消息（检测环路）
  反转得到 root → leaf 的有序对话链
  调用 recoverOrphanedParallelToolResults 恢复并行工具结果
```

#### 3.1.3 Project 类 — 写入引擎

`sessionStorage.ts` 中的 `Project` 类（单例）是写入引擎的核心：

```typescript
class Project {
  sessionFile: string | null = null      // 当前会话文件路径
  pendingEntries: Entry[] = []            // 文件未创建前的缓冲
  writeQueues: Map<string, Array<...>>    // 按文件分组的写入队列
  flushTimer: ReturnType<setTimeout>      // 100ms 定时刷新
  FLUSH_INTERVAL_MS = 100                 // 刷新间隔
  MAX_CHUNK_BYTES = 100 * 1024 * 1024     // 单次写入上限 100MB
}
```

**关键设计**: 
- **延迟物化**: 会话文件在第一条 user/assistant 消息到来时才创建（`materializeSessionFile`），避免产生空文件
- **缓冲队列**: 消息先入队，100ms 后批量 drain 到磁盘，减少 I/O 次数
- **原子追加**: 使用 `fs.appendFile` 追加，即使崩溃也最多丢失最后一个批次
- **退出刷新**: 通过 `cleanupRegistry` 注册退出时的 flush + reAppendSessionMetadata

```typescript
// 清理注册 - 确保退出时刷新缓冲并重写元数据
registerCleanup(async () => {
  await project?.flush()         // 刷新写入队列
  project?.reAppendSessionMetadata()  // 重写元数据到文件末尾
})
```

#### 3.1.4 Lite 读取 — 渐进式会话列表

对于会话列表显示（`/resume` 命令），系统采用 Lite 读取模式：

```
readHeadAndTail(filePath, LITE_READ_BUF_SIZE=64KB):
  1. 读取文件头部 64KB → 提取首条用户消息、创建时间
  2. 读取文件尾部 64KB → 提取最新消息、元数据(title/tag/summary)
  3. 无需读取整个文件即可显示会话列表
```

这解释了为什么 `reAppendSessionMetadata` 要在退出时将元数据重新追加到文件末尾——确保 Lite 读取的尾部窗口总能看到这些关键信息。

#### 3.1.5 Full 读取 — 完整会话加载

当用户选择恢复某个会话时，系统执行 Full 读取：

```
loadTranscriptFile(filePath):
  1. 读取完整 JSONL 文件
  2. 解析每行 JSON，按类型分流到不同的 Map:
     - 转录消息 → messages Map<UUID, TranscriptMessage>
     - 文件历史快照 → fileHistorySnapshots Map
     - 内容替换记录 → contentReplacements Map
     - 上下文折叠提交 → contextCollapseCommits
     - 摘要/标题/标签 → summaries/customTitles/tags Map
     - Worktree 状态 → worktreeStates Map
  3. 处理 legacy progress 条目的桥接（progressBridge）
  4. 计算 leafUuids（无子消息指向的端点集合）
  5. 应用压缩段重链接 (applyPreservedSegmentRelinks)
  6. 应用 Snip 删除 (applySnipRemovals)
```

### 3.2 中断/信号层

**核心文件**: `src/utils/abortController.ts`, `src/hooks/useCancelRequest.ts`

#### 3.2.1 AbortController 层级结构

系统使用标准的 Web AbortController API 实现取消信号的层级传播：

```typescript
// abortController.ts
export function createAbortController(maxListeners = 50): AbortController {
  const controller = new AbortController()
  setMaxListeners(maxListeners, controller.signal)
  return controller
}

export function createChildAbortController(parent, maxListeners?): AbortController {
  const child = createAbortController(maxListeners)
  
  if (parent.signal.aborted) {
    child.abort(parent.signal.reason)
    return child
  }
  
  // WeakRef 防止父控制器持有已放弃的子控制器
  const weakChild = new WeakRef(child)
  const weakParent = new WeakRef(parent)
  parent.signal.addEventListener('abort', handler, { once: true })
  
  // 子控制器 abort 时自动移除父监听器
  child.signal.addEventListener('abort', removeHandler, { once: true })
  
  return child
}
```

**设计亮点**：
- **WeakRef 防泄漏**: 父 → 子使用 WeakRef，子被 GC 后不影响父
- **双向清理**: 子 abort 时自动从父移除监听器，防止死 handler 累积
- **层级传播**: 父 abort → 所有子自动 abort，子 abort 不影响父

#### 3.2.2 CancelRequestHandler 组件

```typescript
// useCancelRequest.ts - CancelRequestHandler 组件
export function CancelRequestHandler(props): null {
  const handleCancel = useCallback(() => {
    // 优先级 1: 有活跃任务在运行 → 取消它
    if (abortSignal !== undefined && !abortSignal.aborted) {
      setToolUseConfirmQueue(() => [])  // 清空权限确认队列
      onCancel()                         // 触发取消回调
      return
    }
    
    // 优先级 2: 有排队命令 → 弹出队列
    if (hasCommandsInQueue()) {
      popCommandFromQueue?.()
      return
    }
    
    // 兜底: 无可取消项
    onCancel()
  }, [abortSignal, ...])

  // Escape 键 (chat:cancel) - 有条件激活
  useKeybinding('chat:cancel', handleCancel, {
    context: 'Chat',
    isActive: isEscapeActive,
  })

  // Ctrl+C (app:interrupt) - 更广泛的取消，包括团队视图返回
  useKeybinding('app:interrupt', handleInterrupt, {
    context: 'Global',
    isActive: isCtrlCActive,
  })

  // Ctrl+X Ctrl+K (chat:killAgents) - 两次按键确认杀所有后台 Agent
  useKeybinding('chat:killAgents', handleKillAgents, {
    context: 'Chat',
  })
  
  return null  // 纯逻辑组件，不渲染 UI
}
```

**三级取消机制**:
| 按键 | 绑定 | 行为 |
|---|---|---|
| `Escape` | `chat:cancel` | 取消当前请求或弹出命令队列 |
| `Ctrl+C` | `app:interrupt` | 中断当前任务，从团队视图返回主线程 |
| `Ctrl+X Ctrl+K` | `chat:killAgents` | 两次按键确认后杀所有后台 Agent |

### 3.3 优雅关闭层

**核心文件**: `src/utils/gracefulShutdown.ts` (~530行)

这是系统应对各种退出场景的最后防线。

#### 3.3.1 信号处理注册

```typescript
export const setupGracefulShutdown = memoize(() => {
  // 固定 signal-exit v4, 防止 Bun bug 导致的 handler 丢失
  onExit(() => {})

  process.on('SIGINT', () => {
    // print 模式由 print.ts 自己处理
    if (process.argv.includes('-p') || process.argv.includes('--print')) return
    void gracefulShutdown(0)
  })
  
  process.on('SIGTERM', () => {
    void gracefulShutdown(143)  // 128 + 15
  })
  
  process.on('SIGHUP', () => {
    void gracefulShutdown(129)  // 128 + 1
  })

  // 孤儿进程检测: macOS 关闭终端时不发 SIGHUP, 而是撤销 TTY 文件描述符
  if (process.stdin.isTTY) {
    setInterval(() => {
      if (!process.stdout.writable || !process.stdin.readable) {
        void gracefulShutdown(129)  // 模拟 SIGHUP
      }
    }, 30_000).unref()
  }
  
  // 全局异常捕获（用于可观测性，不中断进程）
  process.on('uncaughtException', error => { logEvent(...) })
  process.on('unhandledRejection', reason => { logEvent(...) })
})
```

**关键设计细节**:
- **Bun signal-exit 兼容**: 通过注册一个永不取消的 `onExit` 回调来 pin 住 signal-exit v4，防止 Bun 的 `removeListener` bug 导致信号处理器被静默删除
- **孤儿进程检测**: macOS 独有问题——关闭终端窗口时 macOS 撤销 TTY FD 而不发 SIGHUP，进程变成"孤儿"但仍在运行。每30秒检查 stdout 可写性来检测此情况
- **退出码语义**: SIGTERM → 143 (128+15), SIGHUP → 129 (128+1), SIGINT → 0

#### 3.3.2 关闭流程时序

```typescript
async function gracefulShutdown(exitCode, reason, options) {
  if (shutdownInProgress) return  // 防止重入
  shutdownInProgress = true

  // 1. 解析 SessionEnd Hook 超时预算
  const sessionEndTimeoutMs = getSessionEndHookTimeoutMs()

  // 2. 设置 failsafe 定时器 (max(5s, hook超时+3.5s))
  failsafeTimer = setTimeout(() => {
    cleanupTerminalModes()
    printResumeHint()
    forceExit(exitCode)
  }, Math.max(5000, sessionEndTimeoutMs + 3500))

  // 3. 【最先执行】恢复终端模式 + 打印恢复提示
  cleanupTerminalModes()   // 退出 alt screen, 禁用鼠标追踪, 显示光标等
  printResumeHint()        // "Resume this session with: claude --resume ..."

  // 4. 【最关键】运行清理函数（刷新会话数据到磁盘）
  //    有2秒超时保护，防止 MCP 连接等卡住
  await Promise.race([
    runCleanupFunctions(),
    timeout(2000)
  ])

  // 5. 执行 SessionEnd hooks（受用户配置超时限制）
  await executeSessionEndHooks(reason, { signal: AbortSignal.timeout(...) })

  // 6. 刷新分析数据（500ms 超时）
  await Promise.race([
    Promise.all([shutdown1PEventLogging(), shutdownDatadog()]),
    sleep(500)
  ])

  // 7. 强制退出
  forceExit(exitCode)
}
```

**优先级排序设计**:
1. **终端模式恢复** (同步) — 确保终端不被留在脏状态
2. **恢复提示** (同步) — 即使后续步骤超时，用户也能看到如何恢复
3. **会话数据刷盘** (2s超时) — 最关键的数据持久化
4. **SessionEnd hooks** (可配置超时) — 用户自定义清理
5. **分析数据** (500ms) — 可接受丢失
6. **failsafe 定时器** — 绝对保底，确保进程一定退出

#### 3.3.3 终端模式清理

```typescript
function cleanupTerminalModes() {
  if (!process.stdout.isTTY) return
  
  writeSync(1, DISABLE_MOUSE_TRACKING)     // 最先：停止鼠标事件
  inst?.unmount()                           // 卸载 Ink 实例，退出 alt screen
  inst?.drainStdin()                        // 排空 stdin 缓冲
  writeSync(1, DISABLE_MODIFY_OTHER_KEYS)  // 禁用扩展键报告
  writeSync(1, DISABLE_KITTY_KEYBOARD)     // 禁用 Kitty 键盘协议
  writeSync(1, DFE)                        // 禁用焦点事件
  writeSync(1, DBP)                        // 禁用括号粘贴模式
  writeSync(1, SHOW_CURSOR)               // 显示光标
  writeSync(1, CLEAR_ITERM2_PROGRESS)      // 清除 iTerm2 进度条
  writeSync(1, CLEAR_TAB_STATUS)           // 清除标签状态
  writeSync(1, CLEAR_TERMINAL_TITLE)       // 清除终端标题
}
```

#### 3.3.4 恢复提示

```typescript
function printResumeHint() {
  if (resumeHintPrinted) return
  if (!process.stdout.isTTY || !getIsInteractive() || isSessionPersistenceDisabled()) return
  
  const sessionId = getSessionId()
  if (!sessionIdExists(sessionId)) return  // 无会话文件（如子命令）
  
  const customTitle = getCurrentSessionTitle(sessionId)
  const resumeArg = customTitle ? `"${escaped}"` : sessionId
  
  writeSync(1, chalk.dim(`\nResume this session with:\nclaude --resume ${resumeArg}\n`))
  resumeHintPrinted = true
}
```

### 3.4 会话恢复层

**核心文件**: `src/utils/sessionRestore.ts` (~552行), `src/utils/conversationRecovery.ts` (~598行)

#### 3.4.1 loadConversationForResume — 恢复入口

```typescript
// conversationRecovery.ts
export async function loadConversationForResume(
  source: string | LogOption | undefined,
  sourceJsonlFile: string | undefined,
) {
  // 分支1: source === undefined → --continue, 加载最近的非后台会话
  if (source === undefined) {
    const logs = await loadMessageLogs()
    // 过滤掉活跃的后台/daemon会话
    const liveSessions = await listAllLiveSessions()
    log = logs.find(l => !liveSessions.has(getSessionIdFromLog(l)))
  }
  
  // 分支2: sourceJsonlFile → --resume <path.jsonl>, 从文件路径加载
  else if (sourceJsonlFile) {
    const loaded = await loadMessagesFromJsonlPath(sourceJsonlFile)
    messages = loaded.messages
    sessionId = loaded.sessionId
  }
  
  // 分支3: string → --resume <sessionId>, 按 ID 加载
  else if (typeof source === 'string') {
    log = await getLastSessionLog(source as UUID)
  }
  
  // 分支4: LogOption → 已加载的日志对象（交互式 /resume 选择后）
  else {
    log = source
  }

  // Lite → Full 升级
  if (isLiteLog(log)) {
    log = await loadFullLog(log)
  }

  // 复制计划文件和文件历史
  await copyPlanForResume(log, sessionId)
  void copyFileHistoryForResume(log)

  // 恢复技能状态
  restoreSkillStateFromMessages(messages)

  // 反序列化消息（关键步骤！）
  const deserialized = deserializeMessagesWithInterruptDetection(messages)
  
  // 执行 session start hooks
  const hookMessages = await processSessionStartHooks('resume', { sessionId })
  messages.push(...hookMessages)

  return {
    messages,
    turnInterruptionState,
    fileHistorySnapshots, attributionSnapshots, contentReplacements,
    sessionId, agentName, agentColor, agentSetting, customTitle, tag,
    mode, worktreeSession, ...
  }
}
```

#### 3.4.2 消息反序列化 — 中断检测

```typescript
// conversationRecovery.ts
export function deserializeMessagesWithInterruptDetection(serializedMessages) {
  // 1. 迁移 legacy 附件类型
  const migratedMessages = serializedMessages.map(migrateLegacyAttachmentTypes)
  
  // 2. 清理无效的 permissionMode
  for (const msg of migratedMessages) {
    if (msg.type === 'user' && !validModes.has(msg.permissionMode)) {
      msg.permissionMode = undefined
    }
  }
  
  // 3. 过滤未解决的工具使用（被中断的 tool_use 无 tool_result）
  const filteredToolUses = filterUnresolvedToolUses(migratedMessages)
  
  // 4. 过滤孤立的 thinking-only 助手消息
  const filteredThinking = filterOrphanedThinkingOnlyMessages(filteredToolUses)
  
  // 5. 过滤纯空白助手消息
  const filteredMessages = filterWhitespaceOnlyAssistantMessages(filteredThinking)
  
  // 6. 检测中断状态
  const internalState = detectTurnInterruption(filteredMessages)
  
  // 7. 如果是轮次中断，插入 "Continue from where you left off." 续行消息
  if (internalState.kind === 'interrupted_turn') {
    filteredMessages.push(createUserMessage({
      content: 'Continue from where you left off.',
      isMeta: true,
    }))
    turnInterruptionState = { kind: 'interrupted_prompt', message: continuationMessage }
  }
  
  // 8. 在最后一条用户消息后插入哨兵助手消息（确保 API 格式合法）
  if (lastRelevant.type === 'user') {
    filteredMessages.splice(lastRelevantIdx + 1, 0,
      createAssistantMessage({ content: NO_RESPONSE_REQUESTED })
    )
  }
  
  return { messages: filteredMessages, turnInterruptionState }
}
```

**中断检测逻辑** (`detectTurnInterruption`):

```
最后一条有效消息是...
├─ assistant → 轮次正常完成 → kind: 'none'
├─ user (isMeta 或 isCompactSummary) → 系统消息 → kind: 'none'
├─ user (tool_result) 
│  ├─ 是终端工具 (Brief/SendUserFile) → 轮次正常完成 → kind: 'none'
│  └─ 不是终端工具 → 轮次被中断 → kind: 'interrupted_turn'
├─ user (纯文本) → 提示已发但AI未开始回复 → kind: 'interrupted_prompt'
└─ attachment → 用户提供了上下文但AI未回复 → kind: 'interrupted_turn'
```

#### 3.4.3 processResumedConversation — 恢复状态重建

```typescript
// sessionRestore.ts
export async function processResumedConversation(result, opts, context) {
  // 1. 匹配协调器/普通模式
  const modeWarning = modeApi?.matchSessionMode(result.mode)
  
  // 2. 复用恢复会话的 ID（除非 --fork-session）
  if (!opts.forkSession) {
    switchSession(asSessionId(sid), transcriptDir)
    await renameRecordingForSession()      // 重命名 asciicast 录制
    await resetSessionFilePointer()         // 重置文件指针
    restoreCostStateForSession(sid)         // 恢复费用追踪
  }
  
  // 3. 恢复会话元数据
  restoreSessionMetadata(result)
  
  // 4. 恢复 worktree 工作目录
  restoreWorktreeForResume(result.worktreeSession)
  
  // 5. 接管已恢复的会话文件
  adoptResumedSessionFile()
  
  // 6. 恢复上下文折叠提交日志
  restoreFromEntries(result.contextCollapseCommits)
  
  // 7. 恢复 Agent 设置
  const { agentDefinition, agentType } = restoreAgentFromSession(
    result.agentSetting,
    context.mainThreadAgentDefinition,
    context.agentDefinitions,
  )
  
  // 8. 持久化当前模式
  saveMode(isCoordinatorMode() ? 'coordinator' : 'normal')
  
  // 9. 计算初始状态（归因、Agent 上下文、Agent 定义刷新）
  return {
    messages, fileHistorySnapshots, contentReplacements,
    agentName, agentColor, restoredAgentDef,
    initialState: { ...context.initialState, agent, attribution, ... }
  }
}
```

#### 3.4.4 restoreSessionStateFromLog — 状态恢复

```typescript
// sessionRestore.ts
export function restoreSessionStateFromLog(result, setAppState) {
  // 恢复文件修改历史
  if (result.fileHistorySnapshots?.length > 0) {
    fileHistoryRestoreStateFromLog(result.fileHistorySnapshots, newState => {
      setAppState(prev => ({ ...prev, fileHistory: newState }))
    })
  }

  // 恢复代码归因状态
  if (feature('COMMIT_ATTRIBUTION') && result.attributionSnapshots?.length > 0) {
    attributionRestoreStateFromLog(result.attributionSnapshots, newState => {
      setAppState(prev => ({ ...prev, attribution: newState }))
    })
  }

  // 恢复上下文折叠
  if (feature('CONTEXT_COLLAPSE')) {
    restoreFromEntries(result.contextCollapseCommits, result.contextCollapseSnapshot)
  }

  // 恢复 Todo 列表（从转录中提取最后一次 TodoWrite 调用）
  if (!isTodoV2Enabled() && result.messages?.length > 0) {
    const todos = extractTodosFromTranscript(result.messages)
    if (todos.length > 0) {
      setAppState(prev => ({
        ...prev,
        todos: { ...prev.todos, [agentId]: todos },
      }))
    }
  }
}
```

### 3.5 清理层

**核心文件**: `src/utils/cleanupRegistry.ts` (26行), `src/utils/cleanup.ts` (~603行)

#### 3.5.1 清理注册表 — 退出时清理

```typescript
// cleanupRegistry.ts — 极简但至关重要
const cleanupFunctions = new Set<() => Promise<void>>()

export function registerCleanup(cleanupFn: () => Promise<void>): () => void {
  cleanupFunctions.add(cleanupFn)
  return () => cleanupFunctions.delete(cleanupFn)  // 返回取消注册函数
}

export async function runCleanupFunctions(): Promise<void> {
  await Promise.all(Array.from(cleanupFunctions).map(fn => fn()))
}
```

**设计哲学**: 发布-订阅模式实现关注点分离。各子系统独立注册自己的清理函数，gracefulShutdown 统一调用。避免 gracefulShutdown 直接依赖各子系统造成循环依赖。

#### 3.5.2 定期清理 — 磁盘空间管理

`cleanup.ts` 负责清理过期数据（默认30天）：

```typescript
export async function cleanupOldMessageFilesInBackground() {
  await cleanupOldMessageFiles()           // MCP 日志、错误日志
  await cleanupOldSessionFiles()           // 会话 JSONL + 工具结果 + asciicast
  await cleanupOldPlanFiles()              // 计划文件
  await cleanupOldFileHistoryBackups()     // 文件历史备份
  await cleanupOldSessionEnvDirs()         // 会话环境变量目录
  await cleanupOldDebugLogs()              // 调试日志
  await cleanupOldImageCaches()            // 图片缓存
  await cleanupOldPastes(cutoffDate)       // 粘贴缓存
  await cleanupStaleAgentWorktrees(cutoffDate)  // 过期 Agent worktree
  // Ant 用户: 清理 npm 缓存中的旧版本包
  await cleanupNpmCacheForAnthropicPackages()
}
```

### 3.6 文件历史层

**核心文件**: `src/utils/fileHistory.ts` (~900行)

#### 3.6.1 文件修改追踪

每当 AI 要编辑文件时，系统先备份原始内容：

```typescript
export async function fileHistoryTrackEdit(
  updateFileHistoryState,
  filePath: string,
  messageId: UUID,
) {
  // 阶段1: 检查是否需要备份
  const mostRecent = state.snapshots.at(-1)
  if (mostRecent.trackedFileBackups[trackingPath]) return  // 已追踪

  // 阶段2: 异步创建备份
  const backup = await createBackup(filePath, 1)  // v1 = 修改前版本

  // 阶段3: 提交到状态
  updateFileHistoryState(state => ({
    ...state,
    trackedFiles: new Set(state.trackedFiles).add(trackingPath),
    // 更新最近快照的追踪记录
  }))
}
```

**备份存储**: `~/.claude/file-history/<session-id>/<hash>@v<version>`

每个快照（FileHistorySnapshot）记录：
- `messageId`: 关联的消息 ID
- `trackedFileBackups`: 文件路径 → 备份信息的映射
- `timestamp`: 快照时间

这使得 `/undo` 命令可以回滚到任意消息点的文件状态。

### 3.7 工具结果持久化

**核心文件**: `src/utils/toolResultStorage.ts` (~1000行)

#### 3.7.1 大型工具结果的磁盘卸载

当工具（如 Bash、Read 等）产生大量输出时：

```typescript
// 阈值计算
function getPersistenceThreshold(toolName, declaredMax) {
  // GrowthBook 可覆盖每个工具的阈值
  // 默认: min(工具声明的最大值, 50000字符)
}

// 持久化目录: <project-dir>/<session-id>/tool-results/
function getToolResultPath(id, isJson) {
  return join(getToolResultsDir(), `${id}.${isJson ? 'json' : 'txt'}`)
}
```

工具结果超过阈值时：
1. 完整内容写入磁盘文件
2. 对话中替换为预览 + 引用标记 `<persisted-output>`
3. 恢复时通过 `contentReplacements` 记录重建引用关系

---

## 4. 中断触发机制

### 4.1 用户主动中断

#### 4.1.1 Escape 键中断

**触发路径**: 用户按 Escape → `useKeybinding('chat:cancel')` → `handleCancel()`

```
handleCancel():
  ├─ 有运行中任务 (abortSignal未aborted)
  │   ├─ 清空工具使用确认队列
  │   ├─ 调用 onCancel() → abort AbortController
  │   └─ UI 显示 <InterruptedByUser /> 组件: "Interrupted · What should Claude do instead?"
  │
  ├─ 有排队命令 (命令队列非空)
  │   └─ 弹出队列顶部命令
  │
  └─ 其他情况
      └─ 清空确认队列 + onCancel()
```

**不激活场景**: 在以下情况下 Escape 的取消行为被抑制：
- 在 transcript 视图中
- 搜索历史时
- 消息选择器可见时
- 本地 JSX 命令执行中
- 帮助页面打开时
- overlay（模型选择器等）激活时
- Vim INSERT 模式
- 特殊输入模式（bash/background）下输入为空时
- 查看团队成员视图时

#### 4.1.2 Ctrl+C 中断

**触发路径**: 用户按 Ctrl+C → `useKeybinding('app:interrupt')` → `handleInterrupt()`

```
handleInterrupt():
  ├─ 在团队成员视图中
  │   ├─ 杀死所有后台 Agent
  │   └─ 退回主线程视图
  │
  └─ 有运行中任务或排队命令
      └─ 调用 handleCancel() (同 Escape)
```

#### 4.1.3 双击 Ctrl+C / Ctrl+D 退出

**触发路径**: `useExitOnCtrlCD` hook

```typescript
// 基于时间的双击机制 (不使用 chord 系统)
const handleCtrlCDoublePress = useDoublePress(
  pending => setExitState({ pending, keyName: 'Ctrl-C' }),
  exitFn,  // 第二次按下 → 退出应用
)
```

```
第一次按 Ctrl+C:
  ├─ 如果有运行中任务 → 先取消任务 (handleCancel)
  └─ 如果空闲 → 显示 "Press Ctrl-C again to exit"

第二次按 Ctrl+C (超时窗口内):
  └─ 调用 exit() → gracefulShutdown
```

#### 4.1.4 Ctrl+X Ctrl+K 杀后台 Agent

```
第一次按: 显示 "Press Ctrl+X Ctrl+K again to stop background agents" (3秒窗口)
第二次按 (3秒内):
  ├─ 杀死所有运行中的后台 Agent
  ├─ 为每个被杀 Agent 发送 SDK 终止事件
  └─ 聚合通知: "N background agents were stopped by the user: ..."
```

### 4.2 系统内部中断

#### 4.2.1 上下文窗口压缩

当对话上下文接近模型窗口限制时，系统自动触发 compaction：
- 保留最近的消息
- 将旧消息摘要化
- 在 JSONL 中插入 `compact_boundary` 标记
- 恢复时通过 `applyPreservedSegmentRelinks` 重建链接

#### 4.2.2 API 错误重试

当 API 调用失败时，系统通过 AbortController 的层级结构取消相关子任务，但保留会话状态以便重试。

### 4.3 外部信号中断

#### 4.3.1 SIGINT (Ctrl+C from terminal)

在非交互模式（print mode）下，SIGINT 直接触发 gracefulShutdown(0)。交互模式下由 TUI 层处理（见4.1.2）。

#### 4.3.2 SIGTERM (进程管理器)

```
SIGTERM → gracefulShutdown(143)
  → cleanupTerminalModes()
  → printResumeHint()
  → runCleanupFunctions()  // 刷新会话数据
  → executeSessionEndHooks()
  → forceExit(143)
```

#### 4.3.3 SIGHUP (终端关闭)

```
SIGHUP → gracefulShutdown(129)
  (同 SIGTERM 流程，但终端可能已不可写)
```

#### 4.3.4 孤儿进程检测 (macOS 特有)

```
每30秒检查: process.stdout.writable && process.stdin.readable
如果不可写/不可读 → gracefulShutdown(129)  // 模拟 SIGHUP
```

---

## 5. 状态持久化机制

### 5.1 JSONL 追加式日志

**设计选择理由**:
- **崩溃安全**: 追加操作是原子的（内核保证小于 PIPE_BUF 的写入不交错）
- **高性能**: 无需读取-修改-写回整个文件
- **版本兼容**: 新字段自动忽略，旧条目自动跳过
- **可调试**: 纯文本格式，可直接 grep/cat 查看

**文件布局**:
```
~/.claude/
├── projects/
│   └── <sanitized-project-path>/
│       ├── <session-id>.jsonl          # 主对话日志
│       ├── <session-id>.cast           # asciicast 终端录制
│       └── <session-id>/
│           ├── subagents/              # 子 Agent 转录
│           │   └── agent-<agent-id>.jsonl
│           ├── remote-agents/          # 远程 Agent 元数据
│           │   └── remote-agent-<task-id>.meta.json
│           └── tool-results/           # 大型工具结果文件
│               └── <tool-use-id>.txt|json
├── file-history/                       # 文件修改备份
│   └── <session-id>/
│       └── <hash>@v<version>
├── plans/                              # 计划文件
│   └── <slug>.md
└── debug/                              # 调试日志
    └── <session-id>.txt
```

### 5.2 实时流式写入

```
消息生成 → Project.insertMessageChain()
  │
  ├─ sessionFile === null? 
  │   ├─ 是且消息含 user/assistant → materializeSessionFile()
  │   └─ 否 → 缓冲到 pendingEntries
  │
  ├─ 为每条消息:
  │   ├─ 构建 TranscriptMessage (添加 parentUuid, sessionId, cwd, gitBranch, version)
  │   ├─ 调用 appendEntry(transcriptMessage)
  │   │   ├─ 本地模式: enqueueWrite(sessionFile, entry)
  │   │   │   └─ 加入 writeQueues → scheduleDrain() → 100ms 后 drainWriteQueue()
  │   │   └─ 远程模式: Session Ingress API / CCR v2 Internal Events
  │   └─ 更新 parentUuid 链 (跳过 progress 消息)
  │
  └─ 缓存 lastPrompt (用于恢复时显示)
```

### 5.3 元数据管理

会话元数据包括：
- `custom-title`: 用户自定义标题
- `tag`: 标签
- `agent-name` / `agent-color` / `agent-setting`: Agent 配置
- `mode`: 协调器/普通模式
- `worktree-state`: Git worktree 状态
- `last-prompt`: 最后一条用户提示
- `pr-link`: 关联的 PR 信息

**尾部追加策略** (`reAppendSessionMetadata`):

```typescript
reAppendSessionMetadata():
  // 1. 同步读取文件尾部 64KB
  const tail = readFileTailSync(this.sessionFile)
  
  // 2. 从尾部刷新 SDK 可变字段 (custom-title, tag)
  //    防止 CLI 的缓存覆盖 SDK 的最新写入
  const tailTitle = extractLastJsonStringField(tailLine, 'customTitle')
  if (tailTitle !== undefined) this.currentSessionTitle = tailTitle || undefined
  
  // 3. 按序重新追加所有元数据
  appendEntryToFile(this.sessionFile, { type: 'last-prompt', ... })
  appendEntryToFile(this.sessionFile, { type: 'custom-title', ... })
  appendEntryToFile(this.sessionFile, { type: 'tag', ... })
  appendEntryToFile(this.sessionFile, { type: 'agent-name', ... })
  appendEntryToFile(this.sessionFile, { type: 'agent-color', ... })
  appendEntryToFile(this.sessionFile, { type: 'agent-setting', ... })
  appendEntryToFile(this.sessionFile, { type: 'mode', ... })
  appendEntryToFile(this.sessionFile, { type: 'worktree-state', ... })
  appendEntryToFile(this.sessionFile, { type: 'pr-link', ... })
```

### 5.4 文件历史备份

```
文件编辑前:
  fileHistoryTrackEdit(filePath, messageId)
    → createBackup(filePath, version=1)
      → 读取原文件内容
      → 计算 SHA-256 哈希
      → 硬链接/复制到 ~/.claude/file-history/<session>/<hash>@v1
    → 更新 FileHistoryState.snapshots

每轮对话后:
  makeSnapshot(messageId)
    → 对所有已追踪文件创建新版本备份
    → 记录快照到 sessionStorage (file-history-snapshot entry)
    → 保留最多 MAX_SNAPSHOTS=100 个快照
```

---

## 6. 会话恢复流程

### 6.1 CLI 恢复路径

#### 6.1.1 `claude --continue` (继续最近会话)

```
CLI 入口 → loadConversationForResume(undefined, undefined)
  → loadMessageLogs()  // 按时间倒序列出所有会话
  → 过滤活跃后台/daemon 会话
  → 选择第一个符合条件的会话
  → loadFullLog() (Lite → Full 升级)
  → deserializeMessagesWithInterruptDetection()
  → processResumedConversation()
  → 启动 REPL 渲染
```

#### 6.1.2 `claude --resume <id|title|path>` (恢复指定会话)

```
CLI 入口 → 解析参数类型
  ├─ .jsonl 后缀 → loadConversationForResume(id, jsonlPath)
  │   → loadMessagesFromJsonlPath(path) (跨目录恢复)
  │
  ├─ UUID 格式 → loadConversationForResume(sessionId, undefined)  
  │   → getLastSessionLog(sessionId)
  │
  └─ 标题字符串 → 搜索匹配的会话
      → loadConversationForResume(matchedSessionId, undefined)
```

### 6.2 交互式恢复路径

#### 6.2.1 `/resume` 命令

```
用户输入 /resume → resume.tsx 处理
  → fetchLogs(limit)  // 获取会话列表 (Lite 模式)
  → 渲染 SessionPreview 列表 (显示标题/首条提示/时间/消息数)
  → 用户选择会话
  → loadConversationForResume(selectedLog, undefined)
  → 执行恢复流程
```

#### 6.2.2 `/session` 命令

提供更丰富的会话管理：
- 列出所有会话（支持 `--all` 显示所有项目的会话）
- 按 ID 或标题搜索
- 显示会话元数据（创建时间、消息数、标签、Git 分支等）

### 6.3 子代理恢复

**核心文件**: `src/tools/AgentTool/resumeAgent.ts`

```typescript
export async function resumeAgentBackground({ agentId, prompt, ... }) {
  // 1. 加载子代理的转录和元数据
  const [transcript, meta] = await Promise.all([
    getAgentTranscript(asAgentId(agentId)),  // 从 subagents/ 目录加载
    readAgentMetadata(asAgentId(agentId)),    // 从 .meta.json 加载
  ])

  // 2. 过滤和清理恢复的消息
  const resumedMessages = filterWhitespaceOnlyAssistantMessages(
    filterOrphanedThinkingOnlyMessages(
      filterUnresolvedToolUses(transcript.messages)
    )
  )

  // 3. 重建内容替换状态
  const resumedReplacementState = reconstructForSubagentResume(
    contentReplacementState, resumedMessages, transcript.contentReplacements
  )

  // 4. 恢复 worktree（如果存在）
  if (meta?.worktreePath) {
    const exists = await fsp.stat(meta.worktreePath).then(...)
    if (exists) await fsp.utimes(meta.worktreePath, now, now) // 刷新 mtime 防清理
  }

  // 5. 选择 Agent 定义
  if (meta?.agentType === FORK_AGENT.agentType) {
    selectedAgent = FORK_AGENT
  } else if (meta?.agentType) {
    selectedAgent = agentDefinitions.find(a => a.agentType === meta.agentType)
  } else {
    selectedAgent = GENERAL_PURPOSE_AGENT
  }

  // 6. 构建恢复参数并启动
  const runAgentParams = {
    agentDefinition: selectedAgent,
    promptMessages: [...resumedMessages, createUserMessage({ content: prompt })],
    contentReplacementState: resumedReplacementState,
    worktreePath: resumedWorktreePath,
    description: meta?.description,
    ...
  }

  // 7. 注册异步 Agent 任务并运行
  registerAsyncAgent({ agentId, description, ... })
  void runWithAgentContext(agentContext, () =>
    wrapWithCwd(worktreePath, () => runAsyncAgentLifecycle(...))
  )
}
```

**子代理元数据持久化**:
```
写入时: writeAgentMetadata(agentId, { agentType, worktreePath, description })
  → <project-dir>/<session-id>/subagents/agent-<id>.meta.json

恢复时: readAgentMetadata(agentId)
  → 从 .meta.json 恢复 agentType 用于正确路由
  → 恢复 worktreePath 用于正确的 cwd
  → 恢复 description 用于 UI 显示
```

### 6.4 跨项目恢复

**核心文件**: `src/utils/crossProjectResume.ts`

```typescript
export function checkCrossProjectResume(log, showAllProjects, worktreePaths) {
  const currentCwd = getOriginalCwd()
  
  if (!showAllProjects || !log.projectPath || log.projectPath === currentCwd) {
    return { isCrossProject: false }
  }
  
  // 检查是否是同一仓库的 worktree
  const isSameRepo = worktreePaths.some(
    wt => log.projectPath === wt || log.projectPath.startsWith(wt + sep)
  )
  
  if (isSameRepo) {
    // 同仓库 worktree → 可直接恢复
    return { isCrossProject: true, isSameRepoWorktree: true, projectPath }
  } else {
    // 不同仓库 → 生成 cd 命令
    const command = `cd ${quote([log.projectPath])} && claude --resume ${sessionId}`
    return { isCrossProject: true, isSameRepoWorktree: false, command, projectPath }
  }
}
```

### 6.5 远程环境恢复 (Teleport)

系统支持在远程环境（如 Teleport SSH 会话）之间恢复会话：

- `useTeleportResume.tsx`: Hook 处理远程恢复逻辑
- `TeleportResumeWrapper.tsx`: 包装组件处理远程会话的消息同步
- `hydrateSessionFromRemote()`: 从远程 Session Ingress API 拉取会话数据到本地
- `hydrateFromCCRv2InternalEvents()`: 从 CCR v2 内部事件重建会话状态

```
远程恢复流程:
  1. 检测当前是远程环境
  2. 通过 Session Ingress API 获取远程会话数据
  3. 将远程日志写入本地 JSONL 文件
  4. 设置远程 ingress URL 用于后续同步
  5. 正常恢复流程继续
```

---

## 7. 工作流集成

### 7.1 正常对话工作流

```
用户输入 → REPL 处理
  → 创建 AbortController
  → insertMessageChain([userMessage])     ← 实时持久化
  → 发送 API 请求 (携带 abortSignal)
  → 接收流式响应
  → 每个 content_block_stop:
  │   insertMessageChain([assistantBlock]) ← 实时持久化
  └─ 消息完成 → makeSnapshot()            ← 文件历史快照
```

### 7.2 工具使用工作流

```
AI 请求使用工具 → 权限检查
  ├─ 需要确认 → ToolUseConfirm 队列等待
  └─ 已授权 → 执行工具
      → 文件编辑工具: fileHistoryTrackEdit() ← 备份原文件
      → 执行工具逻辑
      → 检查输出大小
      │   ├─ < 阈值: 直接作为消息内容
      │   └─ > 阈值: persistToolResult()      ← 磁盘卸载
      └─ insertMessageChain([toolResult])      ← 实时持久化
```

### 7.3 后台 Agent 工作流

```
AgentTool 启动 → registerAsyncAgent()
  → createChildAbortController(parent)    ← 层级取消
  → writeAgentMetadata()                  ← 持久化 Agent 元数据
  → runAgent() (在独立上下文中)
  │   → 独立的 JSONL: subagents/agent-<id>.jsonl
  │   → 独立的工具结果目录
  └─ Agent 完成/被杀
      → 清理 AbortController
      → 更新任务状态

恢复时:
  → listRemoteAgentMetadata()  / readAgentMetadata()
  → resumeAgentBackground()
  → 重建消息历史 + 内容替换状态
  → 继续执行
```

### 7.4 Session Backgrounding 工作流

```
Ctrl+B → useSessionBackgrounding.handleBackgroundSession()
  ├─ 有前台化的任务 → 重新后台化
  │   ├─ 设置 isBackgrounded = true
  │   ├─ 清除 messages / loading 状态
  │   └─ 释放 AbortController
  └─ 主线程 → 将当前查询转为后台任务
      → onBackgroundQuery()
```

### 7.5 Worktree 恢复工作流

```
恢复包含 worktree 的会话:
  restoreWorktreeForResume(worktreeSession)
    ├─ 已有活跃 worktree → 保留（CLI --worktree 优先）
    ├─ worktreeSession 为空 → 无操作
    └─ 有 worktreeSession:
        ├─ process.chdir(worktreePath)  ← 切换工作目录
        ├─ setCwd(worktreePath)
        ├─ setOriginalCwd()
        ├─ restoreWorktreeSession()
        └─ 清除缓存 (CLAUDE.md, 系统提示, 计划目录)

/resume 切换到其他会话时:
  exitRestoredWorktree()
    ├─ restoreWorktreeSession(null)
    ├─ process.chdir(originalCwd)  ← 恢复原始目录
    └─ 清除缓存
```

---

## 8. 关键技术亮点

### 8.1 WeakRef AbortController 层级

传统的 AbortController 子控制器模式容易造成内存泄漏——父控制器通过事件监听器持有所有子控制器的强引用。Claude Code 使用 `WeakRef` 解决了这个问题：

```typescript
const weakChild = new WeakRef(child)
const handler = propagateAbort.bind(weakParent, weakChild)
parent.signal.addEventListener('abort', handler, { once: true })
```

即使子 AbortController 被丢弃而未 abort，也可以被 GC 回收。

### 8.2 尾部窗口元数据策略

JSONL 文件可能增长到几百 MB。为了在不读取整个文件的情况下获取会话信息：

1. 关键元数据在退出时追加到文件末尾
2. Lite 读取仅读头尾各 64KB
3. 压缩操作后元数据也会重新追加
4. SDK 外部写入通过尾部扫描合并（`reAppendSessionMetadata` 的 refresh 逻辑）

### 8.3 中断检测与自动续行

反序列化消息时，系统能检测到三种中断状态：

| 状态 | 条件 | 恢复行为 |
|---|---|---|
| `none` | 最后是完整的 assistant 回复 | 等待用户输入 |
| `interrupted_prompt` | 最后是用户消息，AI 未开始回复 | 标记为待处理提示 |
| `interrupted_turn` | AI 回复被中途打断（tool_result 未完成） | 自动插入 "Continue from where you left off." |

### 8.4 并行工具结果恢复

流式 API 每个 content_block_stop 产生独立的 AssistantMessage，并行 tool_use 产生多个分支。`recoverOrphanedParallelToolResults` 在恢复时重建这些分支：

1. 按 `message.id` 分组（同一逻辑消息的多个物理消息）
2. 找到被 parentUuid 链遗漏的"孤儿"siblings 和 tool_results
3. 在锚点消息后插入恢复的消息

### 8.5 Bun signal-exit 兼容性修复

```typescript
// signal-exit v4 的 Bun bug 防护
// Bun 的 removeListener 会重置内核 sigaction，导致后续信号直接杀进程
// 修复: 注册一个永不取消的 onExit 回调来 pin 住 v4 内部状态
onExit(() => {})
```

### 8.6 macOS 孤儿进程检测

```typescript
// macOS 特有: 关闭终端不发 SIGHUP，而是撤销 TTY 文件描述符
// 进程变成"孤儿"但仍在运行（消耗资源）
// 解决: 定期检查 stdout 可写性
setInterval(() => {
  if (!process.stdout.writable || !process.stdin.readable) {
    void gracefulShutdown(129)
  }
}, 30_000).unref()
```

### 8.7 Tombstone 快速路径

删除消息时（`removeMessageByUuid`），优化为尾部查找 + 截断：

```
目标消息在最后 64KB 内 (常见):
  → 找到行边界 → ftruncate + 追写剩余 → O(1)

目标消息不在尾部 (罕见):
  → 读取整个文件 → 过滤 → 重写 → O(n)
  → 文件 > 50MB 时跳过（防 OOM）
```

---

## 9. 数据流图

### 9.1 写入流 (实时持久化)

```
用户/AI 消息 ──→ insertMessageChain()
                    │
                    ├─ 构建 TranscriptMessage
                    │   (uuid, parentUuid, sessionId, cwd, gitBranch, timestamp, version)
                    │
                    ├─ appendEntry()
                    │   │
                    │   ├─ 本地: enqueueWrite() ──→ writeQueues
                    │   │                              │
                    │   │                              └─ 100ms ──→ drainWriteQueue()
                    │   │                                              │
                    │   │                                              └─ appendToFile()
                    │   │                                                  → session.jsonl
                    │   │
                    │   └─ 远程: Session Ingress API / CCR v2
                    │
                    └─ 更新 lastPrompt 缓存


文件编辑 ──→ fileHistoryTrackEdit()
               │
               ├─ createBackup() → ~/.claude/file-history/
               │
               └─ insertFileHistorySnapshot() → session.jsonl


大型工具输出 ──→ persistToolResult()
                    │
                    ├─ writeFile() → tool-results/<id>.txt|json
                    │
                    └─ recordContentReplacement() → session.jsonl
```

### 9.2 关闭流 (退出持久化)

```
退出信号 ──→ gracefulShutdown()
               │
               ├─ cleanupTerminalModes() [同步]
               │   (退出 alt screen, 禁用鼠标/键盘特殊模式, 显示光标)
               │
               ├─ printResumeHint() [同步]
               │   "Resume this session with: claude --resume ..."
               │
               ├─ runCleanupFunctions() [2s超时]
               │   │
               │   ├─ project.flush()  ← 刷新写入队列
               │   │
               │   └─ project.reAppendSessionMetadata()  ← 重写元数据到文件末尾
               │       (last-prompt, custom-title, tag, agent-*, mode, worktree, pr-link)
               │
               ├─ executeSessionEndHooks() [可配置超时]
               │
               ├─ 分析数据刷新 [500ms]
               │
               └─ forceExit()
```

### 9.3 恢复流 (会话重建)

```
恢复请求 ──→ loadConversationForResume()
               │
               ├─ 确定来源 (最近/指定ID/JSONL路径/已加载)
               │
               ├─ loadTranscriptFile()
               │   │
               │   ├─ 读取 JSONL, 按类型分流
               │   ├─ 处理 progress bridge
               │   ├─ 计算 leafUuids
               │   ├─ applyPreservedSegmentRelinks()
               │   └─ applySnipRemovals()
               │
               ├─ buildConversationChain()
               │   │
               │   ├─ leaf → root 遍历 (parentUuid 链)
               │   └─ recoverOrphanedParallelToolResults()
               │
               ├─ restoreSkillStateFromMessages()
               │
               ├─ deserializeMessagesWithInterruptDetection()
               │   │
               │   ├─ 迁移 legacy 附件类型
               │   ├─ 过滤未解决工具使用
               │   ├─ 过滤孤立 thinking 消息
               │   ├─ 检测中断状态
               │   └─ 插入续行消息 (如需)
               │
               └─ processResumedConversation()
                   │
                   ├─ switchSession() / restoreCostState()
                   ├─ restoreSessionMetadata()
                   ├─ restoreWorktreeForResume()
                   ├─ adoptResumedSessionFile()
                   ├─ restoreAgentFromSession()
                   ├─ restoreSessionStateFromLog()
                   │   ├─ fileHistoryRestoreStateFromLog()
                   │   ├─ attributionRestoreStateFromLog()
                   │   ├─ restoreFromEntries() [context collapse]
                   │   └─ extractTodosFromTranscript()
                   │
                   └─ 返回 ProcessedResume → 渲染 REPL
```

---

## 附录 A: 关键文件清单

| 文件 | 大小 | 职责 |
|---|---|---|
| `src/utils/sessionStorage.ts` | ~180KB | 会话数据持久化/加载核心 |
| `src/utils/sessionRestore.ts` | ~20KB | 会话恢复状态重建 |
| `src/utils/conversationRecovery.ts` | ~21KB | 对话恢复入口、消息反序列化、中断检测 |
| `src/utils/gracefulShutdown.ts` | ~20KB | 信号处理、优雅关闭、终端清理 |
| `src/utils/cleanup.ts` | ~17KB | 定期清理过期数据 |
| `src/utils/cleanupRegistry.ts` | ~0.5KB | 清理函数注册表 |
| `src/utils/abortController.ts` | ~3KB | WeakRef AbortController 层级 |
| `src/utils/fileHistory.ts` | ~34KB | 文件修改历史追踪和备份 |
| `src/utils/toolResultStorage.ts` | ~37KB | 大型工具结果磁盘卸载 |
| `src/utils/crossProjectResume.ts` | ~2KB | 跨项目恢复检测 |
| `src/hooks/useCancelRequest.ts` | ~10KB | 用户取消请求处理 |
| `src/hooks/useExitOnCtrlCD.ts` | ~3KB | 双击退出处理 |
| `src/hooks/useSessionBackgrounding.ts` | ~5KB | 会话后台/前台切换 |
| `src/tools/AgentTool/resumeAgent.ts` | ~9KB | 子代理恢复 |
| `src/commands/resume/resume.tsx` | ~36KB | /resume 命令 UI |
| `src/screens/ResumeConversation.tsx` | ~58KB | 恢复对话屏幕 |
| `src/components/ResumeTask.tsx` | ~38KB | 任务级恢复组件 |
| `src/components/InterruptedByUser.tsx` | ~2KB | 中断提示 UI |
| `src/bootstrap/state.ts` | ~55KB | 应用状态引导（含恢复路径） |

## 附录 B: 设计权衡

| 决策 | 选择 | 替代方案 | 理由 |
|---|---|---|---|
| 存储格式 | JSONL 追加 | SQLite / JSON | 崩溃安全、高吞吐、可调试 |
| 消息链接 | parentUuid DAG | 数组索引 | 支持分支/压缩/snip |
| 取消机制 | AbortController + WeakRef | 共享状态标志 | 标准API、GC友好、层级传播 |
| 清理注册 | 全局 Set + Promise.all | 直接依赖 | 避免循环依赖、关注点分离 |
| 文件历史 | 磁盘备份 + 快照链 | Git stash | 不依赖 Git、支持非 Git 项目 |
| 元数据持久化 | 尾部追加策略 | 独立元数据文件 | 原子性、单文件简单性 |
| Lite 加载 | 头尾64KB | 独立索引文件 | 零额外维护成本 |

---

*本文档基于 `claude-code-main` 源码分析生成，涵盖了终端恢复系统的完整设计。系统总代码量约 50万+ 行 TypeScript，其中与恢复相关的核心代码约 2万行，分布在 25+ 个关键文件中。*
