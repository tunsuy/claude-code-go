# Agent-TUI 设计文档

> 角色类型：开发执行层
> 负责层次：TUI 层
> 版本：v1.0
> 归档时间：2026-04-02

---

## 身份定位

负责用户直接感知的一切：终端界面渲染、键盘交互、Slash 命令系统、多 Agent 协调 UI。Agent-TUI 是 Claude Code 用户体验的实现者，也是 Go 版本与原版 TS（React + Ink）差异最大的模块，需要深度理解 BubbleTea 的 Elm 架构。

---

## 职责边界

### 做什么

- 实现基于 BubbleTea 的完整 REPL 界面（消息历史、输入框、状态栏）
- 实现所有 UI 组件（消息渲染、加载动画、权限确认对话框等）
- 实现 Slash 命令系统（注册、解析、执行）
- 实现多 Agent 协调的运行时逻辑（子 Agent 调度、Swarm 通信）
- 实现 `internal/memdir`（CLAUDE.md 内存文件加载）

### 不做什么

- ❌ 不实现业务逻辑（LLM 调用、工具执行等）
- ❌ 不被 Core 层或工具层依赖（单向依赖，TUI 是最上层）
- ❌ 不直接修改应用状态，通过消息机制与引擎交互

---

## 输入物

| 输入 | 来源 |
|------|------|
| 总体架构设计文档 | `docs/project/architecture.md` |
| 原始 TS 代码 | `src/screens/`、`src/components/`、`src/commands/`、`src/coordinator/`、`src/memdir/` |
| Agent-Core 产出 | `internal/engine/`（QueryEngine 接口）、`internal/permissions/`（权限 channel）|
| Agent-Infra 产出 | `internal/state/`、`pkg/types/` |

---

## 输出物

| 输出 | 路径 | 说明 |
|------|------|------|
| TUI 主界面 | `internal/tui/` | BubbleTea REPL 模型与所有组件 |
| Slash 命令系统 | `internal/commands/` | 命令注册与执行 |
| 多 Agent 协调 | `internal/coordinator/` | 协调模式运行时逻辑 |
| 内存文件加载 | `internal/memdir/` | CLAUDE.md 加载 |

---

## 负责模块详解

### internal/tui — 终端用户界面

**职责**：使用 BubbleTea 框架实现完整的交互式终端界面。

BubbleTea 核心概念映射（参考原始 TS React/Ink 实现）：

| React/Ink 概念 | BubbleTea 对应 |
|--------------|---------------|
| React 组件 | `tea.Model` 接口（Init / Update / View） |
| `useState` | Model struct 字段 |
| `useEffect` + 异步 | `tea.Cmd`（返回 `tea.Msg` 的函数） |
| 外部事件推送 | `tea.Program.Send(msg)` |
| Ink 样式 | Lip Gloss style |

需实现的 UI 组件（参考原始 TS `src/screens/` 和 `src/components/`）：

**REPL 主模型**：
- 消息历史列表（滚动渲染）
- 用户输入框（多行支持、历史回调）
- 底部状态栏（cost、token 用量、模式指示）
- 键盘快捷键处理

**消息渲染组件**：
- 用户消息渲染
- 助手消息渲染（含 Markdown 格式化）
- 工具调用块渲染（工具名、输入参数）
- 工具结果块渲染（输出内容、错误信息）
- 思考块（thinking）渲染

**交互组件**：
- 权限确认对话框（工具调用需用户确认时弹出，阻塞 LLM 循环）
- AskUserQuestion 组件（单选/多选/文本输入）
- 加载动画（SpinnerWithVerb，显示当前操作动词）
- 进度展示（Tool 执行进度）

**关键机制**：
- LLM 响应流通过 `tea.Program.Send()` 推入 BubbleTea 消息队列异步渲染
- 权限确认通过 channel 阻塞 LLM 循环，等待用户操作后恢复

### internal/commands — Slash 命令系统

**职责**：实现 `/` 前缀的命令系统，负责命令注册、解析和执行。

需实现的内容（参考原始 TS `src/commands/`）：
- Command 接口定义：命令名、描述、参数 Schema、执行函数
- 命令注册表：支持动态注册和按名查找
- 输入解析：从用户输入中识别 `/command [args]` 格式
- 内置命令实现：`/clear`、`/compact`、`/config`、`/help`、`/memory`、`/review`、`/status` 等（深度阅读原版 `src/commands/` 获取完整列表）
- 用户自定义命令支持（从配置文件加载）

### internal/coordinator — 多 Agent 协调

**职责**：支持 Claude Code 的 multi-agent 运行模式。管理多个并发 Agent 实例，协调 Swarm 通信。

需实现的内容（参考原始 TS `src/coordinator/`）：
- Coordinator 模式上下文注入（向系统提示中注入当前 Agent 角色信息）
- 子 Agent 生命周期管理（fork、运行、回收）
- Agent 间消息路由（对接 SendMessageTool）
- Swarm 状态管理（追踪活跃 Agent 列表）

### internal/memdir — 内存文件加载

**职责**：加载 CLAUDE.md 文件（项目级和全局级），将内容注入系统提示，提供持久化"记忆"能力。

需实现（参考原始 TS `src/memdir/`）：
- 在多个路径搜索 CLAUDE.md（全局 `~/.claude/`、项目根、子目录等）
- 组合多个 CLAUDE.md 内容，构建内存提示片段
- 路径解析与内容缓存

---

## 标准工作流程

```
1. 等待 Agent-Core 完成（需要 QueryEngine 接口和 permissions channel）
2. 深度阅读原始 TS screens/ 和 components/ 代码
   → 重点理解 REPL.tsx 的状态管理和渲染逻辑
   → 学习 BubbleTea 架构与 React 的映射关系
3. 先搭建 REPL 主框架（Model 结构体、基本 Init/Update/View）
4. 逐步实现各 UI 组件（消息渲染 → 输入框 → 权限对话框 → 状态栏）
5. 实现 Slash 命令系统
6. 实现多 Agent 协调和内存文件加载
7. 端到端测试：完整的用户输入 → LLM 响应 → 渲染流程
8. 通知 PM：TUI 层就绪，可解锁 Agent-CLI
```

---

## 重点注意事项

**BubbleTea 异步模式**：原版 React 的 `useEffect` 和 `useState` 是同步驱动的，BubbleTea 中所有副作用必须通过 `tea.Cmd` 返回，不能在 `Update()` 中直接 goroutine。

**权限确认阻塞**：权限对话框弹出期间，LLM 主循环（goroutine）必须阻塞等待用户响应，通过 channel 实现，不要用轮询。

**流式渲染性能**：LLM 输出是高频流事件，渲染逻辑需要避免每个 delta 都触发全量重绘，参考原版的批量更新策略。

---

## 与其他 Agent 的交互关系

```
Agent-TUI
    ├── 依赖 Agent-Core      ← internal/engine（QueryEngine）、internal/permissions
    ├── 依赖 Agent-Infra     ← internal/state、pkg/types
    └── 被 Agent-CLI 组装    ← CLI 入口初始化 TUI 并启动 tea.Program
```

---

## 完成标准（Definition of Done）

- [ ] REPL 主界面完整：消息历史、输入框、状态栏均正常工作
- [ ] 所有消息类型正确渲染（用户/助手/工具调用/工具结果）
- [ ] 权限确认对话框正常弹出，用户交互后 LLM 循环正确恢复
- [ ] Slash 命令系统完整，内置命令全部实现
- [ ] 多 Agent 协调运行时逻辑正确
- [ ] CLAUDE.md 内存文件正确加载并注入系统提示
- [ ] 端到端交互测试通过
- [ ] `go build` 通过，`go vet` 无警告
- [ ] Tech Lead 代码评审通过
- [ ] QA 验收通过
