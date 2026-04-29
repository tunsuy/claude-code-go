# Claude Code 状态系统设计深度分析

> 本文档基于对 Claude Code 源码的全面分析，覆盖状态管理架构、设计理念、数据流、工作流集成和 UI 呈现等方面。

---

## 目录

1. [项目概述](#1-项目概述)
2. [状态系统宏观架构](#2-状态系统宏观架构)
3. [核心设计理念](#3-核心设计理念)
4. [第一层：AppState 全局状态中心](#4-第一层appstate-全局状态中心)
5. [第二层：Task 运行时状态](#5-第二层task-运行时状态)
6. [第三层：React Context 特性状态](#6-第三层react-context-特性状态)
7. [第四层：持久化任务系统 (TodoV2)](#7-第四层持久化任务系统-todov2)
8. [信号系统 (Signal)](#8-信号系统-signal)
9. [权限模式状态机](#9-权限模式状态机)
10. [推测执行状态 (Speculation)](#10-推测执行状态-speculation)
11. [状态流转：完整生命周期](#11-状态流转完整生命周期)
12. [UI 呈现与状态消费](#12-ui-呈现与状态消费)
13. [跨进程与分布式状态](#13-跨进程与分布式状态)
14. [Feature Flag 与死代码消除](#14-feature-flag-与死代码消除)
15. [关键技术点总结](#15-关键技术点总结)
16. [附录：核心文件清单](#16-附录核心文件清单)

---

## 1. 项目概述

Claude Code 是 Anthropic 推出的终端 AI 编程助手，技术栈为：

| 维度 | 技术选型 |
|------|---------|
| 运行时 | Bun (TypeScript) |
| UI 框架 | React + Ink (终端渲染) |
| 状态管理 | 自研轻量级 Store + React Context |
| 构建系统 | Bun bundler，支持编译时 feature flag |
| 代码规模 | ~1,900 TypeScript 文件，512,000+ LOC |

它是一个拥有**多 Agent 协作**、**远程会话**、**IDE 桥接**、**插件系统**、**推测执行**等复杂功能的终端应用，其状态系统的设计需要同时应对终端 UI 的特殊性和分布式多进程的挑战。

---

## 2. 状态系统宏观架构

Claude Code 的状态管理采用**四层分离架构**，每层有明确的职责边界：

```
┌─────────────────────────────────────────────────────────────────┐
│                    Layer 4: 持久化任务系统                         │
│            文件系统存储 + 分布式锁 (utils/tasks.ts)                 │
│         Task {id, subject, status, blocks, blockedBy}            │
└──────────────────────────┬──────────────────────────────────────┘
                           │ Signal: tasksUpdated
┌──────────────────────────┴──────────────────────────────────────┐
│                    Layer 3: React Context 层                      │
│    notifications │ overlays │ voice │ mailbox │ stats │ modal    │
│              各自独立，按特性域隔离                                   │
└──────────────────────────┬──────────────────────────────────────┘
                           │ useContext / useAppState
┌──────────────────────────┴──────────────────────────────────────┐
│                    Layer 2: Task 运行时状态                        │
│   LocalShell │ LocalAgent │ RemoteAgent │ Teammate │ Dream ...   │
│        TaskState ⊂ AppState.tasks (可变，包含函数类型)              │
└──────────────────────────┬──────────────────────────────────────┘
                           │ setState
┌──────────────────────────┴──────────────────────────────────────┐
│                    Layer 1: AppState 全局状态                      │
│     createStore<AppState> → DeepImmutable 类型约束                 │
│   toolPermissionContext │ mcp │ plugins │ speculation │ ...       │
│              单一 Store + 选择器订阅                                │
└─────────────────────────────────────────────────────────────────┘
```

**核心设计原则**：每一层解决一个明确的问题域，层与层之间通过明确的接口（setter / signal / file I/O）通信，避免紧耦合。

---

## 3. 核心设计理念

### 3.1 "类 Zustand" 的轻量级 Store

项目没有引入 Redux、Zustand、Jotai 等第三方状态库，而是在 35 行代码内自研了一个极简 Store：

```typescript
// src/state/store.ts — 完整实现仅 35 行
export type Store<T> = {
  getState: () => T
  setState: (updater: (prev: T) => T) => void
  subscribe: (listener: Listener) => () => void
}

export function createStore<T>(initialState: T, onChange?: OnChange<T>): Store<T> {
  let state = initialState
  const listeners = new Set<Listener>()
  return {
    getState: () => state,
    setState: (updater) => {
      const prev = state
      const next = updater(prev)
      if (Object.is(next, prev)) return  // 引用相等即跳过
      state = next
      onChange?.({ newState: next, oldState: prev })  // 侧效应通知
      for (const listener of listeners) listener()    // 订阅者通知
    },
    subscribe: (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
  }
}
```

**设计理念**：

- **极简核心**：不引入中间件、DevTools 等额外抽象，对终端 UI 场景（不需要时间旅行调试）来说是正确取舍
- **函数式更新**：`setState(prev => next)` 模式确保并发安全
- **引用相等短路**：`Object.is(next, prev)` 避免无意义通知
- **onChange 钩子**：在 Store 层面而非 UI 层面拦截状态变化，用于驱动侧效应（如持久化、CCR 同步等）

### 3.2 不可变性策略的精细控制

```typescript
export type AppState = DeepImmutable<{
  // UI、权限、模型、推测等领域 — 全部 readonly
  settings: SettingsJson
  toolPermissionContext: ToolPermissionContext
  speculation: SpeculationState
  // ...
}> & {
  // Tasks 故意排除在 DeepImmutable 之外
  // 因为 TaskState 包含函数类型 (abort controller, cleanup callbacks)
  tasks: { [taskId: string]: TaskState }
  agentNameRegistry: Map<string, AgentId>
  // ...MCP、plugins、fileHistory 等也在此区域
}
```

**设计理念**：
- 使用 `DeepImmutable<T>` 类型工具为状态核心提供**编译时**的不可变性保证
- **有选择地排除**包含函数类型的字段（如 `AbortController`、`cleanup` 回调），因为 TypeScript 的 `readonly` 对函数类型无法有效约束
- 这种"精细不可变"策略在类型安全和实际可用性之间取得了很好的平衡

### 3.3 选择器驱动的精细订阅

```typescript
// src/state/AppState.tsx
export function useAppState<T>(selector: (state: AppState) => T): T {
  const store = useAppStore()
  const get = () => selector(store.getState())
  return useSyncExternalStore(store.subscribe, get, get)
}
```

**设计理念**：
- 基于 React 18 的 `useSyncExternalStore` 实现外部 Store 与 React 渲染的正确同步
- **选择器模式**：组件只订阅关心的状态切片，未选择的字段变化不会触发重渲染
- 显式**禁止返回整个 state**（开发模式下有运行时检查），从架构层面杜绝性能问题
- 提供 `useSetAppState()` 只获取 setter 不订阅的 hook，纯写入组件零重渲染

### 3.4 状态变更的副作用集中化

```typescript
// src/state/onChangeAppState.ts
export function onChangeAppState({ newState, oldState }) {
  // 权限模式变化 → 通知 CCR + SDK
  if (prevMode !== newMode) {
    notifySessionMetadataChanged({ permission_mode: newExternal })
    notifyPermissionModeChanged(newMode)
  }
  // 模型变化 → 持久化到 settings
  if (newState.mainLoopModel !== oldState.mainLoopModel) { ... }
  // 展开视图变化 → 持久化到 globalConfig
  if (newState.expandedView !== oldState.expandedView) { ... }
  // settings 变化 → 清除认证缓存
  if (newState.settings !== oldState.settings) { ... }
}
```

**设计理念**：
- 所有 `AppState` 变更的副作用**集中在一个函数**中处理
- 源码注释特别说明了这一设计的动机：之前权限模式变化的 CCR 通知分散在 8+ 个调用点，常常遗漏，导致远程状态不同步
- 通过 Store 的 `onChange` 钩子统一拦截 diff，**任何 setState 都会自动触发所有需要的副作用**
- 这本质上是一个**状态变更事件总线**，把散落的 "同步到 X" 逻辑收归一处

---

## 4. 第一层：AppState 全局状态中心

### 4.1 状态域划分

AppState 是一个约 **450 行**的类型定义（`src/state/AppStateStore.ts`），按功能域划分为以下子系统：

| 域 | 关键字段 | 说明 |
|----|---------|------|
| **UI 控制** | `verbose`, `expandedView`, `isBriefOnly`, `footerSelection`, `spinnerTip` | 控制终端渲染行为 |
| **模型配置** | `mainLoopModel`, `mainLoopModelForSession`, `thinkingEnabled`, `effortValue` | 当前使用的 AI 模型及参数 |
| **权限系统** | `toolPermissionContext`, `denialTracking`, `workerSandboxPermissions` | 工具调用的权限状态机 |
| **任务管理** | `tasks`, `foregroundedTaskId`, `viewingAgentTaskId` | 后台任务、Agent 子任务 |
| **远程会话** | `remoteSessionUrl`, `remoteConnectionStatus`, `remoteBackgroundTaskCount` | CCR 远程 Agent |
| **Bridge 桥接** | `replBridgeEnabled/Connected/SessionActive/Reconnecting/...` | IDE 双向通信状态 |
| **MCP 系统** | `mcp.clients/tools/commands/resources` | Model Context Protocol 扩展 |
| **插件系统** | `plugins.enabled/disabled/errors/installationStatus` | 插件生命周期 |
| **推测执行** | `speculation`, `speculationSessionTimeSavedMs`, `promptSuggestion` | 预测用户意图并提前执行 |
| **团队协作** | `teamContext`, `standaloneAgentContext`, `inbox` | Swarm 多 Agent 协调 |
| **计划模式** | `initialMessage`, `pendingPlanVerification` | Plan mode 工作流 |
| **Ultraplan** | `ultraplanLaunching/SessionUrl/PendingChoice/LaunchPending` | 服务端规划 |
| **认证** | `authVersion` | 登录/登出触发依赖数据刷新 |

### 4.2 初始状态工厂

```typescript
export function getDefaultAppState(): AppState {
  const initialMode: PermissionMode =
    teammateUtils.isTeammate() && teammateUtils.isPlanModeRequired()
      ? 'plan' : 'default'

  return {
    settings: getInitialSettings(),
    tasks: {},
    agentNameRegistry: new Map(),
    toolPermissionContext: {
      ...getEmptyToolPermissionContext(),
      mode: initialMode,  // 根据运行环境动态决定初始权限模式
    },
    speculation: IDLE_SPECULATION_STATE,
    // ... 约 100 个字段的默认值
  }
}
```

**技术要点**：
- 初始状态工厂是一个**函数**而非常量，支持动态初始化（如根据是否为团队成员决定权限模式）
- 使用 lazy require 解决循环依赖（`teammate.ts` ↔ `AppStateStore.ts`）
- 所有字段都有明确的默认值，确保应用在任何时刻都处于有效状态

### 4.3 Provider 与嵌套保护

```typescript
// src/state/AppState.tsx
export function AppStateProvider({ children, initialState, onChangeAppState }) {
  const hasAppStateContext = useContext(HasAppStateContext)
  if (hasAppStateContext) {
    throw new Error("AppStateProvider can not be nested within another AppStateProvider")
  }

  const [store] = useState(() =>
    createStore(initialState ?? getDefaultAppState(), onChangeAppState)
  )

  // 挂载时检查 bypass 权限模式是否需要禁用
  useEffect(() => { ... }, [])

  // 监听外部 settings 文件变化并同步到 AppState
  const onSettingsChange = useEffectEvent(source =>
    applySettingsChange(source, store.setState)
  )
  useSettingsChange(onSettingsChange)

  return (
    <HasAppStateContext.Provider value={true}>
      <AppStoreContext.Provider value={store}>
        <MailboxProvider>
          <VoiceProvider>{children}</VoiceProvider>
        </MailboxProvider>
      </AppStoreContext.Provider>
    </HasAppStateContext.Provider>
  )
}
```

**关键设计**：
- `HasAppStateContext` 双重上下文防嵌套，确保全应用**唯一 Store 实例**
- Store 创建在 `useState` 中完成，引用永远不变 → Provider 永远不触发 consumer 重渲染
- 集成了 `MailboxProvider`（Agent 间消息）和 `VoiceProvider`（语音输入）作为内层 Provider
- `useSettingsChange` 监听文件系统上 settings.json 的变化，实现**配置热更新**

### 4.4 选择器与 Hook 体系

```typescript
// 基本选择器 — 只在选中值变化时重渲染
const verbose = useAppState(s => s.verbose)
const model = useAppState(s => s.mainLoopModel)

// 子对象选择器 — 返回已存在的引用，无额外分配
const { text, promptId } = useAppState(s => s.promptSuggestion)

// 纯写入 — 组件不订阅任何状态
const setAppState = useSetAppState()

// 获取 Store 实例 — 传给非 React 代码
const store = useAppStateStore()

// 安全版本 — 在 Provider 外返回 undefined
const maybeVerbose = useAppStateMaybeOutsideOfProvider(s => s.verbose)
```

---

## 5. 第二层：Task 运行时状态

### 5.1 任务类型体系

```typescript
// src/Task.ts
export type TaskType =
  | 'local_bash'            // Shell 命令执行
  | 'local_agent'           // 进程内子 Agent
  | 'remote_agent'          // 远程 Agent (CCR)
  | 'in_process_teammate'   // Swarm 团队成员
  | 'local_workflow'        // 工作流执行
  | 'monitor_mcp'           // MCP 服务器健康监控
  | 'dream'                 // 后台/定时任务

export type TaskStatus =
  | 'pending'    // 已创建，未启动
  | 'running'    // 正在执行
  | 'completed'  // 成功完成 (终态)
  | 'failed'     // 失败 (终态)
  | 'killed'     // 手动终止 (终态)
```

### 5.2 任务 ID 设计

```typescript
// 每种任务类型有唯一前缀
const TASK_ID_PREFIXES = {
  local_bash: 'b',           // b + 8位随机字符
  local_agent: 'a',
  remote_agent: 'r',
  in_process_teammate: 't',
  local_workflow: 'w',
  monitor_mcp: 'm',
  dream: 'd',
}

// 使用 36 进制字母表 + crypto.randomBytes 生成
// 36^8 ≈ 2.8 万亿种组合，足以抵抗 symlink 攻击
const TASK_ID_ALPHABET = '0123456789abcdefghijklmnopqrstuvwxyz'
```

**设计理念**：
- 前缀使得从 ID 即可判断任务类型，便于日志分析和调试
- 大小写不敏感的字母表避免在不同文件系统上产生冲突
- 密码学安全的随机数防止 ID 预测攻击

### 5.3 任务状态流转

```
            ┌─────────┐
创建 ────────▶│ pending │
            └────┬────┘
                 │ start()
            ┌────▼────┐
            │ running │──────────┐
            └────┬────┘          │
                 │               │
         ┌───────┼───────┐      │
         │       │       │      │
    ┌────▼──┐ ┌──▼───┐ ┌─▼────┐│
    │completed│ │failed│ │killed││
    └────────┘ └──────┘ └──────┘│
         ▲       ▲       ▲      │
         └───────┴───────┴──────┘
              终态（不可逆）
```

```typescript
// 终态守卫函数 — 全局使用
export function isTerminalTaskStatus(status: TaskStatus): boolean {
  return status === 'completed' || status === 'failed' || status === 'killed'
}
```

### 5.4 TaskState 联合类型

```typescript
// src/tasks/types.ts
export type TaskState =
  | LocalShellTaskState       // 包含 pid, exitCode 等
  | LocalAgentTaskState       // 包含 agentId, messages 等
  | RemoteAgentTaskState      // 包含 sessionUrl, pollState 等
  | InProcessTeammateTaskState// 包含 agentName, transcript 等
  | LocalWorkflowTaskState    // 包含 workflowSteps 等
  | MonitorMcpTaskState       // 包含 serverName, healthStatus 等
  | DreamTaskState            // 包含 schedule, lastRun 等
```

每种具体 TaskState 扩展公共基类 `TaskStateBase`：

```typescript
export type TaskStateBase = {
  id: string           // 类型前缀 + 8位随机
  type: TaskType       // 判别联合标签
  status: TaskStatus   // 当前生命周期阶段
  description: string  // 人类可读描述
  toolUseId?: string   // 关联的 tool_use ID
  startTime: number    // 创建时间戳
  endTime?: number     // 终止时间戳
  totalPausedMs?: number // 暂停总时长
  outputFile: string   // 磁盘输出文件路径
  outputOffset: number // 当前读取偏移量
  notified: boolean    // 是否已通知用户
}
```

### 5.5 后台任务判断

```typescript
export function isBackgroundTask(task: TaskState): task is BackgroundTaskState {
  // 必须是运行中或等待中
  if (task.status !== 'running' && task.status !== 'pending') return false
  // 前台任务不算后台任务
  if ('isBackgrounded' in task && task.isBackgrounded === false) return false
  return true
}
```

### 5.6 任务与 AppState 的集成

任务存储在 `AppState.tasks` 中（有意排除在 `DeepImmutable` 之外），通过 `setAppState` 进行原子更新：

```typescript
// 创建任务
setAppState(prev => ({
  ...prev,
  tasks: {
    ...prev.tasks,
    [taskId]: newTaskState,
  },
}))

// 更新任务状态
setAppState(prev => ({
  ...prev,
  tasks: {
    ...prev.tasks,
    [taskId]: {
      ...prev.tasks[taskId],
      status: 'completed',
      endTime: Date.now(),
    },
  },
}))
```

---

## 6. 第三层：React Context 特性状态

### 6.1 Context Provider 矩阵

| Context | 文件 | 大小 | 职责 |
|---------|------|------|------|
| **Notifications** | `context/notifications.tsx` | 33KB | 优先级队列、超时自动消失、key 去重合并 |
| **Overlays** | `context/overlayContext.tsx` | 14KB | 模态层栈、Escape 键协调 |
| **PromptOverlay** | `context/promptOverlayContext.tsx` | 12KB | 提示输入框内的覆盖层 |
| **Mailbox** | `context/mailbox.tsx` | 3.4KB | Agent 间异步消息传递 |
| **Voice** | `context/voice.tsx` | 8.8KB | 语音输入状态（feature-gated） |
| **Stats** | `context/stats.tsx` | 22KB | 性能和使用量追踪 |
| **FPS Metrics** | `context/fpsMetrics.tsx` | 3.2KB | 帧率监控 |
| **QueuedMessage** | `context/QueuedMessageContext.tsx` | 5.6KB | 排队消息调度 |
| **Modal** | `context/modalContext.tsx` | 58行 | 模态框尺寸和滚动 |

### 6.2 通知系统设计

通知系统是 Context 层最复杂的一个，采用了**优先级队列 + 折叠合并**的设计：

```typescript
type Priority = 'low' | 'medium' | 'high' | 'immediate'

type Notification = {
  key: string           // 唯一键，用于去重和合并
  invalidates?: string[]// 使哪些通知失效
  priority: Priority    // 显示优先级
  timeoutMs?: number    // 自动消失时间（默认 8秒）
  fold?: (acc, incoming) => Notification  // 同 key 合并策略
  text?: string         // 文本通知
  jsx?: ReactNode       // 或 JSX 通知
}
```

**状态流转**：
```
入队 → queue[] 中按优先级排序
  ↓ current === null 时
出队 → current = 最高优先级项
  ↓ timeout 到期
清除 → current = null → 触发下一轮出队
```

特殊处理：
- `immediate` 优先级的通知会**中断**当前显示的通知
- `invalidates` 机制允许新通知**撤销**旧通知（如"操作成功"撤销"正在操作"）
- `fold` 机制允许**合并**相同 key 的通知（如多个文件修改合并为"修改了 N 个文件"）

### 6.3 覆盖层系统

```typescript
// 解决核心问题：Escape 键到底是关闭 Select 弹窗，还是取消当前请求？

export function useRegisterOverlay(id: string, enabled = true) {
  useEffect(() => {
    if (!enabled) return
    setAppState(prev => {
      const next = new Set(prev.activeOverlays)
      next.add(id)
      return { ...prev, activeOverlays: next }
    })
    return () => {
      setAppState(prev => {
        const next = new Set(prev.activeOverlays)
        next.delete(id)
        return { ...prev, activeOverlays: next }
      })
    }
  }, [id, enabled])
}

// CancelRequestHandler 检查：
// if (activeOverlays.size > 0) → 只关闭覆盖层
// if (activeOverlays.size === 0) → 取消当前请求
```

**设计理念**：将覆盖层的存在性作为**全局共享状态**存入 AppState（而非各组件自行管理），使得任何需要感知覆盖层的代码都能可靠地查询。

---

## 7. 第四层：持久化任务系统 (TodoV2)

### 7.1 文件系统存储结构

```
~/.claude/tasks/<task-list-id>/
├── 1.json              # Task 1 的 JSON 数据
├── 2.json              # Task 2 的 JSON 数据
├── .highwatermark      # 已分配的最大 ID，防止重用
└── .lock               # 文件锁（flock）
```

### 7.2 Task 数据模型

```typescript
export type Task = {
  id: string
  subject: string        // 简短标题（祈使句）
  description: string    // 详细描述
  activeForm?: string    // 进行时描述（如 "Running tests"）
  owner?: string         // 认领的 Agent ID
  status: 'pending' | 'in_progress' | 'completed'
  blocks: string[]       // 此任务阻塞的任务 ID 列表
  blockedBy: string[]    // 阻塞此任务的任务 ID 列表
  metadata?: Record<string, unknown>
}
```

注意：这里的 `TaskStatus` (`pending | in_progress | completed`) 与 Layer 2 的运行时 `TaskStatus` (`pending | running | completed | failed | killed`) 是**不同的类型**，服务于不同的抽象层。

### 7.3 分布式锁与并发安全

```typescript
// 为 ~10+ 个并发 Swarm Agent 设计的锁策略
const LOCK_OPTIONS = {
  retries: {
    retries: 30,      // 最多重试 30 次
    minTimeout: 5,    // 最短等待 5ms
    maxTimeout: 100,  // 最长等待 100ms
  },
  // 总等待预算 ~2.6 秒，覆盖 10 路竞争场景
}
```

**设计理念**：
- 使用文件锁（`lockfile` 模块）而非数据库实现分布式互斥
- High water mark 文件防止 `resetTaskList()` 后重用旧 ID
- 每次写操作（create/update/delete）都在锁内执行
- 锁的退避参数针对 Swarm（多 Agent 并发）场景精心调优

### 7.4 Signal 驱动的 UI 刷新

```typescript
// 任务变更 → 发射信号
const tasksUpdated = createSignal()
export const onTasksUpdated = tasksUpdated.subscribe

// UI 侧订阅
useEffect(() => {
  return onTasksUpdated(() => {
    // 重新读取文件系统，刷新 UI
    refreshTasks()
  })
}, [])
```

---

## 8. 信号系统 (Signal)

### 8.1 实现

```typescript
// src/utils/signal.ts — 44 行，极简发布-订阅
export function createSignal<Args extends unknown[] = []>(): Signal<Args> {
  const listeners = new Set<(...args: Args) => void>()
  return {
    subscribe(listener) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    emit(...args) {
      for (const listener of listeners) listener(...args)
    },
    clear() {
      listeners.clear()
    },
  }
}
```

### 8.2 与 Store 的区别

| 维度 | Store | Signal |
|------|-------|--------|
| 是否有快照 | 有 (`getState()`) | 无 |
| 用途 | "当前值是什么" | "发生了什么事" |
| 通知内容 | 无参数 | 可携带事件参数 |
| 典型场景 | UI 状态 | 跨模块事件通知 |

### 8.3 应用场景

```typescript
// 任务列表变更
const tasksUpdated = createSignal()

// Settings 文件变更
const settingsChanged = createSignal<[SettingSource]>()

// 会话切换
const sessionSwitched = createSignal()

// 快速模式冷却
const cooldownTriggered = createSignal()

// 使用量超限
const overageRejection = createSignal()
```

---

## 9. 权限模式状态机

### 9.1 权限模式层次

```typescript
// 外部（用户可见）模式
export const EXTERNAL_PERMISSION_MODES = [
  'acceptEdits',       // 自动接受编辑
  'bypassPermissions', // 跳过所有权限检查
  'default',           // 默认：每次询问
  'dontAsk',           // 不询问
  'plan',              // 计划模式：审批计划后执行
] as const

// 内部（包含引擎专用）模式
export type InternalPermissionMode =
  | ExternalPermissionMode
  | 'auto'    // 分类器自动审批（ant-only）
  | 'bubble'  // 中间态（内部使用）
```

### 9.2 模式配置表

```typescript
const PERMISSION_MODE_CONFIG = {
  default:           { title: 'Default',            symbol: '',   color: 'text' },
  plan:              { title: 'Plan Mode',          symbol: '⏸',  color: 'planMode' },
  acceptEdits:       { title: 'Accept edits',       symbol: '⏵⏵', color: 'autoAccept' },
  bypassPermissions: { title: 'Bypass Permissions', symbol: '⏵⏵', color: 'error' },
  dontAsk:           { title: "Don't Ask",          symbol: '⏵⏵', color: 'error' },
  auto:              { title: 'Auto mode',          symbol: '⏵⏵', color: 'warning' },
}
```

### 9.3 权限决策流水线

每次工具调用触发以下 4 阶段决策：

```
工具调用请求
    │
    ▼
┌────────────────┐
│ Stage 1: 规则  │  alwaysAllow / alwaysDeny / alwaysAsk 规则匹配
│   (静态匹配)   │  来源：settings.json, CLI 参数, 会话规则
└───────┬────────┘
        │ 无匹配
        ▼
┌────────────────┐
│ Stage 2: Hooks │  PreToolUse hooks (settings.json 中配置)
│   (用户脚本)   │  可返回 approve / deny / passthrough
└───────┬────────┘
        │ passthrough
        ▼
┌────────────────┐
│ Stage 3: 分类器│  安全分类器 (auto 模式启用时)
│  (AI 判断)     │  分析命令安全性，自动批准或拒绝
└───────┬────────┘
        │ 未决定
        ▼
┌────────────────┐
│ Stage 4: 用户  │  终端交互提示
│  (人工决策)    │  用户选择 Allow / Deny / Always Allow
└────────────────┘
```

### 9.4 ToolPermissionContext 详细结构

```typescript
export type ToolPermissionContext = DeepImmutable<{
  mode: PermissionMode                                    // 当前权限模式
  additionalWorkingDirectories: Map<string, AdditionalWorkingDirectory> // 额外允许的工作目录
  alwaysAllowRules: ToolPermissionRulesBySource           // 始终允许的规则（按来源分组）
  alwaysDenyRules: ToolPermissionRulesBySource             // 始终拒绝的规则
  alwaysAskRules: ToolPermissionRulesBySource               // 始终询问的规则
  isBypassPermissionsModeAvailable: boolean                // bypass 模式是否可用
  isAutoModeAvailable?: boolean                            // auto 模式是否可用
  shouldAvoidPermissionPrompts?: boolean                   // 后台 Agent 不显示权限弹窗
  awaitAutomatedChecksBeforeDialog?: boolean                // 先等自动检查再弹窗
  prePlanMode?: PermissionMode                             // 进入 Plan mode 前的模式（用于退出时恢复）
}>
```

### 9.5 权限决策结果

```typescript
type PermissionResult =
  | { behavior: 'allow';  updatedInput?; decisionReason? }
  | { behavior: 'deny';   message; decisionReason }
  | { behavior: 'ask';    message; suggestions?; pendingClassifierCheck? }
  | { behavior: 'passthrough'; message; pendingClassifierCheck? }  // 继续下一阶段
```

`decisionReason` 记录了决策来源，用于审计和调试：

```typescript
type PermissionDecisionReason =
  | { type: 'rule'; rule: PermissionRule }           // 静态规则命中
  | { type: 'mode'; mode: PermissionMode }           // 权限模式决定
  | { type: 'hook'; hookName; hookSource?; reason? } // Hook 脚本决定
  | { type: 'classifier'; classifier; reason }       // AI 分类器决定
  | { type: 'sandboxOverride'; reason }              // 沙箱覆盖
  | { type: 'safetyCheck'; reason; classifierApprovable } // 安全检查
  | { type: 'other'; reason }                        // 其他
```

---

## 10. 推测执行状态 (Speculation)

### 10.1 状态定义

```typescript
export type SpeculationState =
  | { status: 'idle' }  // 空闲
  | {
      status: 'active'
      id: string
      abort: () => void                    // 取消推测
      startTime: number
      messagesRef: { current: Message[] }  // 可变引用 — 避免数组扩展
      writtenPathsRef: { current: Set<string> } // 推测写入的文件路径
      boundary: CompletionBoundary | null  // 完成边界标记
      suggestionLength: number
      toolUseCount: number
      isPipelined: boolean                 // 是否管线化执行
      contextRef: { current: REPLHookContext }
      pipelinedSuggestion?: {
        text: string
        promptId: 'user_intent' | 'stated_intent'
        generationRequestId: string | null
      } | null
    }
```

### 10.2 完成边界

```typescript
export type CompletionBoundary =
  | { type: 'complete'; completedAt: number; outputTokens: number }
  | { type: 'bash'; command: string; completedAt: number }
  | { type: 'edit'; toolName: string; filePath: string; completedAt: number }
  | { type: 'denied_tool'; toolName; detail; completedAt: number }
```

### 10.3 推测执行流转

```
用户开始输入
    │
    ▼
┌────────┐
│  idle  │ ← speculation.status
└───┬────┘
    │ 触发推测
    ▼
┌────────┐    用户继续输入 / 推测不匹配
│ active │ ─────────────────────────────────▶ abort() ──▶ idle
└───┬────┘
    │ 推测完成
    │
    ├─▶ 用户接受 → 注入主消息流 → speculationSessionTimeSavedMs +=
    │
    └─▶ 用户忽略 → 丢弃 → idle
```

**设计理念**：
- 使用 `Ref`（可变引用）而非不可变数组存储推测消息，避免频繁的数组创建开销
- `CompletionBoundary` 精确标记推测到达的"边界"，使主流程知道从何处接续
- `speculationSessionTimeSavedMs` 累计本次会话节省的时间，展示推测执行的价值

---

## 11. 状态流转：完整生命周期

### 11.1 用户输入到 AI 响应的完整数据流

```
用户按回车提交消息
    │
    ▼
┌─────────────────────────────────────────┐
│ REPL 组件捕获输入                        │
│  → setAppState: initialMessage = {...}  │
│  → 如有推测：检查是否可复用              │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│ QueryEngine 接收消息                     │
│  → 构造 API 请求 (messages + system)    │
│  → setStreamMode('thinking')             │
│  → 开始流式请求                          │
└──────────────┬──────────────────────────┘
               │ 流式响应开始
               ▼
┌─────────────────────────────────────────┐
│ 流式处理循环                             │
│  → content_block_start → 创建新块       │
│  → content_block_delta → 累积内容       │
│  → content_block_stop  → 块完成         │
│  → 遇到 tool_use 块 → 进入工具执行     │
└──────────────┬──────────────────────────┘
               │ 遇到 tool_use
               ▼
┌─────────────────────────────────────────┐
│ 权限决策流水线                           │
│  Rules → Hooks → Classifier → User      │
│  → behavior = 'allow' / 'deny' / 'ask' │
└──────────────┬──────────────────────────┘
               │ allow
               ▼
┌─────────────────────────────────────────┐
│ StreamingToolExecutor                    │
│  → 并发安全检查 isConcurrencySafe()     │
│  → 执行工具 tool.call(args, context)    │
│  → 收集进度 onProgress()                │
│  → 得到 ToolResult                      │
└──────────────┬──────────────────────────┘
               │ 工具结果
               ▼
┌─────────────────────────────────────────┐
│ 结果注入消息流                           │
│  → tool_result 添加到 messages          │
│  → 如有 newMessages → 追加              │
│  → 如有 contextModifier → 应用          │
│  → 继续下一轮 API 调用（Agent loop）    │
└──────────────┬──────────────────────────┘
               │ stop_reason = 'end_turn'
               ▼
┌─────────────────────────────────────────┐
│ 响应完成                                 │
│  → setStreamMode(null)                  │
│  → 持久化到 transcript                  │
│  → 触发 post-sampling hooks             │
│  → 如有推测：开始下一轮推测             │
│  → UI 回到等待输入状态                  │
└─────────────────────────────────────────┘
```

### 11.2 后台任务的生命周期

```
Agent 调用 BashTool (background: true)
    │
    ▼
┌─────────────────────────────────────────┐
│ 1. 生成 Task ID (b + 8 随机字符)         │
│ 2. 创建 TaskStateBase (status: pending) │
│ 3. setAppState: tasks[id] = newState    │
│ 4. 写 outputFile 路径                   │
└──────────────┬──────────────────────────┘
               │ spawn
               ▼
┌─────────────────────────────────────────┐
│ 5. 子进程启动                            │
│ 6. status → running                     │
│ 7. stdout/stderr → 写入 outputFile      │
│ 8. 进度 → onProgress → UI 更新          │
└──────────────┬──────────────────────────┘
               │ 进程退出
               ▼
┌─────────────────────────────────────────┐
│ 9. exitCode → status (completed/failed) │
│ 10. endTime = Date.now()                │
│ 11. notified = false → 通知逻辑触发     │
│ 12. UI 显示任务完成通知                  │
└─────────────────────────────────────────┘
```

### 11.3 Plan Mode 状态流转

```
用户输入 / 模型自主进入 Plan Mode
    │
    ▼
┌─────────────────────────────────────┐
│ toolPermissionContext.mode → 'plan'  │
│ prePlanMode = 之前的模式（用于恢复） │
│ UI: 状态栏显示 ⏸ Plan Mode          │
└──────────────┬──────────────────────┘
               │
               ▼
┌─────────────────────────────────────┐
│ 模型生成计划                         │
│ → 写入 plan 文件                    │
│ → pendingPlanVerification.plan = .. │
└──────────────┬──────────────────────┘
               │ ExitPlanMode
               ▼
┌─────────────────────────────────────┐
│ 用户审批计划                         │
│ → 可能附带 allowedPrompts           │
│ → initialMessage = { message, mode, │
│     allowedPrompts }                 │
└──────────────┬──────────────────────┘
               │ 批准
               ▼
┌─────────────────────────────────────┐
│ 切换回之前的权限模式                  │
│ mode → prePlanMode                  │
│ 执行计划中的操作                     │
│ 后台验证: pendingPlanVerification   │
└─────────────────────────────────────┘
```

---

## 12. UI 呈现与状态消费

### 12.1 组件-状态映射关系

| 组件 | 消费的状态 | 呈现行为 |
|------|-----------|---------|
| **REPL** | 几乎全部 | 顶层编排：消息列表、输入框、Spinner、状态栏 |
| **Spinner** | `isBriefOnly`, `viewingAgentTaskId`, `expandedView`, `tasks` | 加载动画 + 当前活动描述 + 任务树 |
| **StatusLine** | `toolPermissionContext.mode`, `mainLoopModel`, settings | 底部信息栏：模型、模式、上下文用量 |
| **Messages** | `tasks`, `foregroundedTaskId` | 消息列表渲染 |
| **TaskListV2** | `tasks` (via TodoV2) | 持久化任务的 UI 面板 |
| **BridgeDialog** | `replBridge*` 全族字段 | IDE 桥接状态弹窗 |
| **ModelPicker** | `mainLoopModel` | 模型选择器 |
| **ThemePicker** | settings | 主题选择器 |
| **CompanionSprite** | `companionReaction`, `companionPetAt`, `footerSelection` | 伙伴精灵动画 |

### 12.2 Spinner 的多模式渲染

Spinner 是 UI 中最复杂的状态消费者之一，它根据 `SpinnerMode` 呈现不同的视觉效果：

```typescript
export type SpinnerMode =
  | 'thinking'      // 模型思考中 — 显示思考动画
  | 'streaming'     // 流式输出中 — 显示输出进度
  | 'tool_use'      // 工具执行中 — 显示工具名和进度
  | 'paused'        // 暂停 — 等待用户权限决策
  | null            // 空闲 — 不显示
```

Spinner 还集成了：
- **活动描述**（来自 `activityManager` 和 `tool.getActivityDescription()`）
- **任务树**（展开模式下显示所有 Agent 的状态）
- **Brief 模式**（极简显示，仅关键信息）
- **队友 Spinner 树**（Swarm 模式下显示每个队友的状态）

### 12.3 StatusLine 的自定义渲染

StatusLine 支持通过 settings.json 中的 `statusLine` 命令自定义输出：

```typescript
function buildStatusLineCommandInput(...): StatusLineCommandInput {
  return {
    model: { id: runtimeModel, display_name: renderModelName(runtimeModel) },
    workspace: { current_dir, project_dir, added_dirs },
    cost: { total_cost_usd, total_duration_ms, total_lines_added, ... },
    context_window: { total_input_tokens, used_percentage, remaining_percentage },
    rate_limits: { five_hour: { used_percentage, resets_at } },
    // ...
  }
}
```

StatusLine 命令（通常是 shell 脚本）接收结构化输入，返回 ANSI 格式化的文本，实现了**状态到视觉的可编程映射**。

### 12.4 权限提示的 UI 呈现

当权限决策进入 Stage 4（用户决策）时，UI 流程：

```
权限请求入队
    │
    ▼
┌────────────────────────────────┐
│ SpinnerMode → 'paused'         │
│ 暂停动画，显示暂停标志          │
└──────────┬─────────────────────┘
           │
           ▼
┌────────────────────────────────┐
│ PermissionPrompt 组件渲染      │
│ • 工具名称和输入摘要            │
│ • 风险等级标记                  │
│ • Allow / Deny / Always Allow  │
│ • 覆盖层注册 (Escape 协调)     │
└──────────┬─────────────────────┘
           │ 用户选择
           ▼
┌────────────────────────────────┐
│ • Allow → behavior: 'allow'   │
│ • Deny → behavior: 'deny'     │
│ • Always → 写入 settings.json │
│ SpinnerMode → 恢复            │
└────────────────────────────────┘
```

### 12.5 工具结果的差异化渲染

每个 Tool 定义了一套完整的渲染接口：

```typescript
interface Tool {
  // 工具调用时的展示
  renderToolUseMessage(input, options): ReactNode

  // 工具进度（执行中）
  renderToolUseProgressMessage?(progress, options): ReactNode

  // 工具结果
  renderToolResultMessage?(content, progress, options): ReactNode

  // 工具排队中
  renderToolUseQueuedMessage?(): ReactNode

  // 工具被拒绝
  renderToolUseRejectedMessage?(input, options): ReactNode

  // 工具出错
  renderToolUseErrorMessage?(result, options): ReactNode

  // 分组渲染（多个并行工具）
  renderGroupedToolUse?(toolUses, options): ReactNode | null

  // 是否截断（控制点击展开行为）
  isResultTruncated?(output): boolean
}
```

**设计理念**：每种工具完全控制自己在 UI 上的呈现，从调用、进度、成功、失败到拒绝，覆盖了工具生命周期的每个阶段。这种**策略模式**使得添加新工具时无需修改 UI 框架代码。

---

## 13. 跨进程与分布式状态

### 13.1 远程会话 (CCR) 状态同步

```
本地 REPL                         CCR 远程 Agent
┌──────────┐                      ┌──────────┐
│ AppState │ ◄──── WebSocket ───▶ │ Daemon   │
│          │   (事件流)            │ Process  │
│ remote-  │                      │          │
│ Connection│                      │ tasks,   │
│ Status:  │                      │ messages │
│ connected│                      └──────────┘
└──────────┘
```

状态同步机制：
- `remoteConnectionStatus`: `'connecting' → 'connected' → 'reconnecting' → 'disconnected'`
- 本地 `AppState.tasks` 在 viewer 模式下**始终为空** — 任务在远程进程中
- `remoteBackgroundTaskCount` 通过 WS 事件 `system/task_started` 和 `system/task_notification` 更新

### 13.2 Bridge (IDE 桥接) 状态机

```
replBridgeEnabled = true (用户启用)
    │
    ▼
环境注册 + 会话创建
    │
    ▼
replBridgeConnected = true (Ready)
    │
    ▼
WebSocket 连接
    │
    ├─▶ replBridgeSessionActive = true (Connected - 用户在 claude.ai 上)
    │
    ├─▶ replBridgeReconnecting = true (重连中)
    │
    └─▶ replBridgeError = "..." (错误)

权限桥接：
    replBridgePermissionCallbacks → 双向权限检查
    channelPermissionCallbacks → Telegram/iMessage 等渠道权限
```

### 13.3 Swarm 团队协作状态

```typescript
teamContext: {
  teamName: string          // 团队名称
  teamFilePath: string      // 团队配置文件路径
  leadAgentId: string       // 领导 Agent ID
  selfAgentId?: string      // 自身 Agent ID
  selfAgentName?: string    // 自身名称 ('team-lead' for 领导)
  isLeader?: boolean        // 是否为领导
  teammates: {
    [teammateId: string]: {
      name: string
      agentType?: string
      color?: string           // UI 颜色标记
      tmuxSessionName: string  // tmux 会话名
      tmuxPaneId: string       // tmux 窗格 ID
      cwd: string              // 工作目录
      worktreePath?: string    // Git worktree 路径
      spawnedAt: number        // 启动时间
    }
  }
}
```

### 13.4 收件箱 (Inbox) 状态机

```typescript
inbox: {
  messages: Array<{
    id: string
    from: string
    text: string
    timestamp: string
    status: 'pending' | 'processing' | 'processed'
    color?: string
    summary?: string
  }>
}
```

状态流转：`pending → processing → processed`

Worker 侧的权限请求也通过消息传递：
```typescript
workerSandboxPermissions: {
  queue: Array<{
    requestId: string
    workerId: string
    workerName: string
    host: string
  }>
  selectedIndex: number
}
```

---

## 14. Feature Flag 与死代码消除

### 14.1 编译时 Feature Flag

```typescript
import { feature } from 'bun:bundle'

// 编译时判断 — 未启用的分支会被 bundler 完全删除 (DCE)
const VoiceProvider = feature('VOICE_MODE')
  ? require('../context/voice.js').VoiceProvider
  : ({ children }) => children

// 条件性扩展 AppState 字段
showTeammateMessagePreview?: boolean  // 仅 ENABLE_AGENT_SWARMS 时存在
```

### 14.2 已知 Feature Flag

| Flag | 功能 |
|------|------|
| `VOICE_MODE` | 语音输入 |
| `BRIDGE_MODE` | IDE 桥接 |
| `KAIROS` / `KAIROS_BRIEF` | 主动代码建议 |
| `TRANSCRIPT_CLASSIFIER` | auto mode 分类器 |
| `AGENT_TRIGGERS` | Agent 自动触发 |
| `MONITOR_TOOL` | 进程监控工具 |
| `CHICAGO_MCP` | 计算机视觉 MCP |
| `ENABLE_AGENT_SWARMS` | 多 Agent 协作 |

### 14.3 Feature Flag 对状态的影响

Feature flag 深入影响状态系统的方方面面：
- **类型层面**：某些 AppState 字段仅在特定 flag 下存在（用 `?` 标记）
- **Provider 层面**：如 `VoiceProvider` 在外部构建中退化为透传
- **权限层面**：`auto` 模式仅在 `TRANSCRIPT_CLASSIFIER` 启用时可用
- **UI 层面**：Brief 模式仅在 `KAIROS` 相关 flag 启用时生效

---

## 15. 关键技术点总结

### 15.1 架构层面

| 技术点 | 说明 |
|--------|------|
| **四层状态分离** | 全局 Store / 运行时 Task / Context 特性域 / 文件持久化，各层职责清晰 |
| **自研轻量 Store** | 35 行实现，避免了重量级状态库在终端 UI 场景下的不必要开销 |
| **精细不可变性** | `DeepImmutable` + 函数类型豁免，在类型安全和实用性间取得平衡 |
| **副作用集中化** | `onChangeAppState` 统一处理所有状态变更的副作用，消除遗漏 |
| **编译时死代码消除** | `bun:bundle` 的 `feature()` 在编译时裁剪无关代码路径 |

### 15.2 性能层面

| 技术点 | 说明 |
|--------|------|
| **选择器订阅** | `useAppState(selector)` 只在选中值变化时触发重渲染 |
| **useSyncExternalStore** | React 18 推荐的外部 Store 集成方式，保证并发模式下的一致性 |
| **可变 Ref** | 推测执行中使用 `{ current: ... }` 避免频繁数组创建 |
| **Object.is 短路** | Store 级别的引用相等检查，避免无意义的通知链 |
| **嵌套保护** | `HasAppStateContext` 防止重复 Provider，确保单一 Store 实例 |

### 15.3 可靠性层面

| 技术点 | 说明 |
|--------|------|
| **分布式文件锁** | 支持 10+ Agent 并发的 lockfile + 退避策略 |
| **High water mark** | 防止 Task ID 重用，即使在 reset 后 |
| **终态守卫** | `isTerminalTaskStatus()` 全局使用，防止向已终止的任务注入消息 |
| **信号容错** | `notifyTasksUpdated()` 内部 try/catch，listener 失败不影响写操作 |
| **权限审计链** | `PermissionDecisionReason` 完整记录每个决策的来源和原因 |

### 15.4 可扩展性层面

| 技术点 | 说明 |
|--------|------|
| **Tool 策略模式** | 每个工具完全控制自己的权限检查、UI 渲染、进度报告 |
| **buildTool 工厂** | 统一的工具构建入口，提供安全默认值 |
| **Signal 解耦** | 跨模块事件通知无需直接依赖 |
| **Plugin 系统** | 插件注入 MCP tools、commands、errors，通过 AppState.plugins 统一管理 |
| **TaskType 联合扩展** | 添加新任务类型只需新增 TaskState 变体 + 实现类 |

---

## 16. 附录：核心文件清单

### 状态核心

| 文件 | 行数 | 职责 |
|------|------|------|
| `src/state/store.ts` | 35 | 通用 Store 实现 |
| `src/state/AppStateStore.ts` | 570 | AppState 类型定义和默认值工厂 |
| `src/state/AppState.tsx` | 200 | React Provider 和 Hook |
| `src/state/onChangeAppState.ts` | 172 | 状态变更副作用 |
| `src/state/selectors.ts` | 77 | 派生状态选择器 |

### Task 系统

| 文件 | 行数 | 职责 |
|------|------|------|
| `src/Task.ts` | 126 | Task 类型、ID 生成、基类 |
| `src/tasks/types.ts` | 47 | TaskState 联合类型 |
| `src/tasks/LocalShellTask/` | ~522 | Shell 任务实现 |
| `src/tasks/LocalAgentTask/` | ~682 | 进程内 Agent 任务 |
| `src/tasks/RemoteAgentTask/` | ~855 | 远程 Agent 任务 |
| `src/tasks/InProcessTeammateTask/` | ~125 | Swarm 队友任务 |
| `src/tasks/DreamTask/` | ~157 | 后台定时任务 |
| `src/utils/tasks.ts` | 863 | 文件持久化任务系统 |

### Context 层

| 文件 | 职责 |
|------|------|
| `src/context/notifications.tsx` | 优先级通知队列 |
| `src/context/overlayContext.tsx` | 覆盖层栈管理 |
| `src/context/mailbox.tsx` | Agent 间消息 |
| `src/context/voice.tsx` | 语音输入 |
| `src/context/stats.tsx` | 性能指标 |

### 权限系统

| 文件 | 职责 |
|------|------|
| `src/types/permissions.ts` | 纯类型定义（无运行时依赖） |
| `src/utils/permissions/PermissionMode.ts` | 模式配置和转换 |
| `src/utils/permissions/permissionSetup.ts` | 权限初始化 |
| `src/utils/permissions/denialTracking.ts` | 拒绝计数追踪 |
| `src/Tool.ts` | 工具接口（含权限相关方法） |

### 信号与工具

| 文件 | 职责 |
|------|------|
| `src/utils/signal.ts` | 发布-订阅原语 |
| `src/services/tools/StreamingToolExecutor.ts` | 流式工具执行器 |
| `src/services/tools/toolHooks.ts` | 工具 Hook 系统 |

---

> **文档生成时间**：2026-04-29
> **基于版本**：Claude Code 源码主干分支
