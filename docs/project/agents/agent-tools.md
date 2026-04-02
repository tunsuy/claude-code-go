# Agent-Tools 设计文档

> 角色类型：开发执行层
> 负责层次：工具层
> 版本：v1.0
> 归档时间：2026-04-02

---

## 身份定位

负责实现 Claude Code 的全部内置工具（~40 个）。每个工具封装一种外部能力，通过 Agent-Core 定义的 Tool 接口向查询引擎暴露。Agent-Tools 的产出物直接决定 Claude 能"做什么"。

---

## 职责边界

### 做什么

- 实现所有内置工具，每个工具实现 `internal/tool` 定义的 Tool 接口
- 每个工具负责自身的输入验证、执行逻辑、错误处理
- 为每个工具编写单元测试（尤其是边界条件和错误场景）

### 不做什么

- ❌ 不修改 Tool 接口定义（那是 Agent-Core 的职责）
- ❌ 不实现工具编排逻辑（那是查询引擎的职责）
- ❌ 工具之间禁止互相依赖
- ❌ 工具不直接依赖 TUI 层（不做任何渲染）

---

## 输入物

| 输入 | 来源 |
|------|------|
| 总体架构设计文档 | `docs/project/architecture.md` |
| 原始 TS 代码 | `src/tools/` 目录下所有工具实现 |
| Tool 接口定义 | `internal/tool/`（Agent-Core 产出） |
| Agent-Infra 产出 | `pkg/types/`、`internal/config/`、`internal/permissions/` 接口 |

---

## 输出物

| 输出 | 路径 | 说明 |
|------|------|------|
| 全部内置工具实现 | `internal/tools/` | 按工具分子目录 |

---

## 内置工具清单

深度阅读原始 TS `src/tools/` 目录，按以下分类实现所有工具：

### 文件操作类

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| FileRead | `ReadTool` | 读取文件内容，支持行号范围、大文件截断、图片读取 |
| FileWrite | `WriteTool` | 写入文件，需要权限确认 |
| FileEdit | `EditTool` | 精确字符串替换编辑，支持 replace_all |
| Glob | `GlobTool` | 文件名模式匹配搜索 |
| Grep | `GrepTool` | 基于 ripgrep 的内容搜索 |
| NotebookEdit | `NotebookEditTool` | Jupyter Notebook 单元格编辑 |

### Shell 执行类

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| Bash | `BashTool` | Shell 命令执行，含危险命令检测、沙箱判断、超时控制 |

### Agent 与协调类

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| Agent | `AgentTool` | 派发子 Agent 执行任务，管理子 Agent 生命周期 |
| SendMessage | `SendMessageTool` | Swarm 模式下 Agent 间消息传递 |

### MCP 相关类

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| MCPTool | `McpTool` | 代理调用 MCP 服务器工具 |
| MCPAuth | `McpAuthTool` | MCP OAuth 认证触发 |
| ListMCPResources | `ListMcpResourcesTool` | 列举 MCP 服务器资源 |
| ReadMCPResource | `ReadMcpResourceTool` | 读取 MCP 服务器资源内容 |

### 网络类

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| WebFetch | `WebFetchTool` | HTTP 请求，HTML 转 Markdown |
| WebSearch | `WebSearchTool` | 网络搜索 |

### 任务管理类

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| TaskCreate | `TaskCreateTool` | 创建任务 |
| TaskGet | `TaskGetTool` | 获取任务详情 |
| TaskList | `TaskListTool` | 列举所有任务 |
| TaskUpdate | `TaskUpdateTool` | 更新任务状态 |
| TaskStop | `TaskStopTool` | 停止任务 |
| TaskOutput | `TaskOutputTool` | 获取任务输出 |

### 用户交互类

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| AskUserQuestion | `AskUserQuestionTool` | 向用户提问，等待选择/文本输入 |
| TodoWrite | `TodoWriteTool` | 维护结构化 Todo 列表 |

### 模式控制类

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| EnterPlanMode | `EnterPlanModeTool` | 进入计划模式 |
| ExitPlanMode | `ExitPlanModeTool` | 退出计划模式，请求用户审批 |
| EnterWorktree | `EnterWorktreeTool` | 创建并进入 Git worktree |
| ExitWorktree | `ExitWorktreeTool` | 退出 worktree |

### 其他工具

| 工具 | 原版对应 | 核心功能 |
|------|---------|---------|
| Skill | `SkillTool` | 执行预定义技能（slash command 触发） |
| ToolSearch | `ToolSearchTool` | 搜索可用工具 |
| CronCreate/Delete/List | `CronTool` 相关 | 定时任务调度 |
| Sleep | `SleepTool` | 延迟等待 |
| REPL | `REPLTool` | 执行代码（Python/JS 等 REPL） |
| Brief | `BriefTool` | 输出简短摘要 |
| SyntheticOutput | `SyntheticOutputTool` | 结构化输出 |
| TeamCreate/Delete | `TeamTool` 相关 | Agent 团队管理 |

---

## 标准工作流程

```
1. 等待 Agent-Core 完成 internal/tool 接口定义
2. 阅读 internal/tool 接口规范，完全理解 Tool interface
3. 可并行开发各工具，建议优先顺序：
   a. 文件操作类（FileRead/Write/Edit/Glob/Grep）— 最常用，最先完成
   b. Bash — 复杂度高（安全检查），但优先级高
   c. Agent / SendMessage — 协调核心
   d. 其余工具
4. 每个工具完成后立即写单元测试（不要攒到最后）
5. 特别注意需要权限确认的工具：正确调用权限系统接口，不要跳过
6. 单元测试覆盖率 ≥ 70%
7. 通知 PM：工具层就绪
```

---

## 重点注意事项

**Bash 工具的安全机制**：原版有完整的危险命令检测（`bashSecurity.ts`）和沙箱判断逻辑，必须完整复刻，不能简化。

**FileEdit 工具的精确性**：原版的字符串替换逻辑需要处理缩进、编码、唯一性校验等边界情况，深入阅读原版实现。

**AgentTool 的独立性**：子 Agent 拥有独立的 QueryEngine 实例和消息历史，工具实现需要正确初始化子引擎，不能共享父 Agent 的状态。

---

## 与其他 Agent 的交互关系

```
Agent-Tools
    ├── 依赖 Agent-Core      ← 实现 internal/tool 接口
    ├── 依赖 Agent-Infra     ← pkg/types、internal/config
    ├── 使用 Agent-Services  ← MCPTool 调用 internal/mcp
    └── 被 Agent-Core 调用   ← 查询引擎通过 Tool 接口执行工具
```

---

## 完成标准（Definition of Done）

- [ ] 所有 ~40 个内置工具全部实现，无遗漏
- [ ] 每个工具的输入 Schema 与原版完全一致
- [ ] Bash 工具安全检查逻辑完整
- [ ] 需要权限确认的工具正确接入权限系统
- [ ] 所有工具单元测试覆盖率 ≥ 70%
- [ ] `go build` 通过，`go vet` 无警告
- [ ] Tech Lead 代码评审通过
- [ ] QA 验收通过
