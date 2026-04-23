# Claude Code 记忆系统设计深度分析

> 基于 claude-code-main 源码的全面分析，涵盖架构设计、存储机制、工作流集成、安全防护等方面。

---

## 目录

- [一、设计理念与技术哲学](#一设计理念与技术哲学)
- [二、系统架构总览](#二系统架构总览)
- [三、多层记忆体系详解](#三多层记忆体系详解)
  - [3.1 Auto Memory（自动记忆）](#31-auto-memory自动记忆)
  - [3.2 Team Memory（团队记忆）](#32-team-memory团队记忆)
  - [3.3 Session Memory（会话记忆）](#33-session-memory会话记忆)
  - [3.4 Agent Memory（代理记忆）](#34-agent-memory代理记忆)
  - [3.5 CLAUDE.md 指令层](#35-claudemd-指令层)
- [四、记忆的创建流程](#四记忆的创建流程)
  - [4.1 显式创建](#41-显式创建)
  - [4.2 后台自动提取（Extract Memories）](#42-后台自动提取extract-memories)
  - [4.3 快速记忆输入（# 快捷方式）](#43-快速记忆输入-快捷方式)
- [五、记忆的存储格式与组织](#五记忆的存储格式与组织)
  - [5.1 文件格式](#51-文件格式)
  - [5.2 索引机制（MEMORY.md）](#52-索引机制memorymd)
  - [5.3 目录结构](#53-目录结构)
- [六、记忆的检索与使用](#六记忆的检索与使用)
  - [6.1 系统提示注入](#61-系统提示注入)
  - [6.2 相关性检索（Relevant Memory Surfacing）](#62-相关性检索relevant-memory-surfacing)
  - [6.3 记忆时效性管理](#63-记忆时效性管理)
  - [6.4 附件注入机制](#64-附件注入机制)
- [七、记忆整合与维护（Auto Dream）](#七记忆整合与维护auto-dream)
  - [7.1 触发条件与门控](#71-触发条件与门控)
  - [7.2 整合流程的四个阶段](#72-整合流程的四个阶段)
  - [7.3 锁机制](#73-锁机制)
- [八、团队记忆同步机制](#八团队记忆同步机制)
  - [8.1 同步架构](#81-同步架构)
  - [8.2 文件监视器](#82-文件监视器)
  - [8.3 安全防护](#83-安全防护)
- [九、Forked Agent 基础设施](#九forked-agent-基础设施)
  - [9.1 核心概念](#91-核心概念)
  - [9.2 Prompt Cache 共享](#92-prompt-cache-共享)
  - [9.3 隔离与安全](#93-隔离与安全)
- [十、记忆管理（用户界面）](#十记忆管理用户界面)
  - [10.1 /memory 命令](#101-memory-命令)
  - [10.2 /remember 技能](#102-remember-技能)
  - [10.3 /dream 命令](#103-dream-命令)
- [十一、与其他工作流的集成](#十一与其他工作流的集成)
  - [11.1 Compact（上下文压缩）集成](#111-compact上下文压缩集成)
  - [11.2 Query Loop 集成](#112-query-loop-集成)
  - [11.3 Stop Hooks 集成](#113-stop-hooks-集成)
  - [11.4 工具权限集成](#114-工具权限集成)
- [十二、关键设计模式与技术亮点](#十二关键设计模式与技术亮点)
- [十三、类型系统](#十三类型系统)
- [十四、总结](#十四总结)

---

## 一、设计理念与技术哲学

Claude Code 的记忆系统体现了以下核心设计理念：

### 1.1 「渐进式学习」而非「一次性配置」

传统 AI 助手的个性化依赖用户手动配置规则。Claude Code 采用**渐进式学习**策略——在每次对话中自动观察、提取、积累知识，使 AI 对用户的理解随时间自然增长。这就像人类的记忆一样：你不需要一次性告诉助手所有偏好，它会在协作过程中逐渐「记住」你。

### 1.2 「文件即记忆」的朴素存储哲学

整个记忆系统**完全基于文件系统**存储，没有使用任何数据库。这个看似简单的设计选择背后有深刻的考量：

- **透明可审计**：用户可以直接浏览、编辑、删除记忆文件
- **版本控制友好**：团队记忆可以通过 Git 进行版本管理
- **零依赖**：不需要任何外部数据库或服务
- **跨会话持久**：文件系统天然提供持久化
- **用户主权**：数据始终在用户本地，用户拥有完全控制权

### 1.3 「分层作用域」设计

记忆按作用域分为多个层次，从个人到团队，从项目到全局，每层有不同的可见性和同步策略：

```
全局管理层 (Managed)     → /etc/claude-code/CLAUDE.md    → 管理员控制
用户全局层 (User)        → ~/.claude/CLAUDE.md            → 用户私有全局
项目层 (Project)         → {project}/CLAUDE.md            → 团队共享（Git）
本地层 (Local)           → {project}/CLAUDE.local.md      → 个人项目偏好
自动记忆层 (AutoMem)     → ~/.claude/projects/*/memory/   → AI 自动学习
团队记忆层 (TeamMem)     → ~/.claude/projects/*/team/     → API 同步共享
```

### 1.4 「后台智能体」模式

记忆的提取、整理、检索都由**后台 Forked Agent** 完成——它们是主对话的「影子分身」，共享相同的上下文但独立运行，不打断用户的交互流程。这是一种非常优雅的异步智能处理模式。

### 1.5 「最终一致性」而非「强一致性」

记忆系统的各个子系统（提取、整合、同步）都采用**最终一致性**策略——它们是「尽力而为」的后台任务，失败不会影响主流程，且有重试和自愈机制。

---

## 二、系统架构总览

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          主对话循环 (Query Loop)                         │
│                                                                         │
│  ┌─────────────┐    ┌──────────────┐    ┌───────────────────────────┐   │
│  │ System      │    │ User Context │    │   Attachment System       │   │
│  │ Prompt      │◄───┤ (CLAUDE.md)  │    │                           │   │
│  │             │    └──────────────┘    │  ┌─────────────────────┐  │   │
│  │ ┌─────────┐ │                        │  │ nested_memory       │  │   │
│  │ │ Memory  │ │    ┌──────────────┐    │  │ (CLAUDE.md 层级注入)│  │   │
│  │ │ Prompt  │◄┼────┤ MEMORY.md    │    │  └─────────────────────┘  │   │
│  │ │ Section │ │    │ (索引)       │    │  ┌─────────────────────┐  │   │
│  │ └─────────┘ │    └──────────────┘    │  │ relevant_memories   │  │   │
│  └─────────────┘                        │  │ (相关记忆预取)      │  │   │
│                                         │  └─────────────────────┘  │   │
│                                         │  ┌─────────────────────┐  │   │
│                                         │  │ session_memory      │  │   │
│                                         │  │ (会话记忆)          │  │   │
│                                         │  └─────────────────────┘  │   │
│                                         └───────────────────────────┘   │
└────────────┬────────────────────────────────────┬───────────────────────┘
             │ Turn 结束触发                       │
             ▼                                     ▼
┌────────────────────┐   ┌───────────────────┐   ┌───────────────────────┐
│ Extract Memories   │   │  Auto Dream       │   │  Team Memory Sync     │
│ (后台 Forked Agent)│   │  (后台 Forked Agent)│   │  (文件监视器 + API)   │
│                    │   │                   │   │                       │
│ 分析对话 → 提取    │   │ 扫描 → 整合 →    │   │  本地 ←→ 服务器       │
│ 有价值记忆         │   │ 清理过时记忆      │   │  Pull / Push          │
└────────────────────┘   └───────────────────┘   └───────────────────────┘
             │                    │                        │
             ▼                    ▼                        ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                       文件系统 (Memory Directory)                        │
│                                                                         │
│  ~/.claude/projects/<project-slug>/memory/                              │
│  ├── MEMORY.md              (索引文件)                                  │
│  ├── user_preferences.md    (用户偏好)                                  │
│  ├── project_architecture.md(项目架构)                                  │
│  ├── feedback_testing.md    (反馈: 测试偏好)                            │
│  ├── reference_dashboards.md(外部引用)                                  │
│  ├── .consolidate-lock      (Dream 锁文件)                             │
│  └── team/                  (团队记忆子目录)                            │
│      ├── MEMORY.md          (团队索引)                                  │
│      └── team_conventions.md(团队约定)                                  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 核心子系统一览

| 子系统 | 文件位置 | 功能 | 触发方式 |
|--------|---------|------|---------|
| **memdir** | `src/memdir/` | 记忆路径、提示词构建、扫描 | 系统启动时 |
| **Extract Memories** | `src/services/extractMemories/` | 从对话中自动提取记忆 | 每个 Turn 结束时 |
| **Auto Dream** | `src/services/autoDream/` | 记忆整合、清理过时记忆 | 累积 5 个会话后 |
| **Session Memory** | `src/services/SessionMemory/` | 当前会话的上下文记忆 | Compact 时 |
| **Team Memory Sync** | `src/services/teamMemorySync/` | 团队记忆云端同步 | 文件变更时 |
| **Relevant Memory** | `src/utils/attachments.ts` | 相关记忆检索注入 | 每个 Turn 开始时 |

---

## 三、多层记忆体系详解

### 3.1 Auto Memory（自动记忆）

**核心模块**：`src/memdir/`

Auto Memory 是整个记忆系统的核心，它是 AI 自动学习和积累的知识库。

#### 路径计算 (`paths.ts`)

```typescript
// 路径解析优先级链
function getAutoMemPath(): string {
  // 1. 环境变量覆盖（cowork 模式）
  if (CLAUDE_COWORK_MEMORY_PATH_OVERRIDE) return override

  // 2. 设置中的自定义目录
  if (settings.autoMemoryDirectory) return resolve(cwd, settings.autoMemoryDirectory)

  // 3. 默认路径: ~/.claude/projects/<sanitized-git-root>/memory/
  return join(memoryBase, 'projects', sanitizeProjectSlug(gitRoot), 'memory')
}
```

项目标识通过 Git 根目录 slug 化实现，确保**同一个 Git 仓库**的所有目录共享同一个记忆空间。

#### 启用/禁用门控

```typescript
function isAutoMemoryEnabled(): boolean {
  // 优先级从高到低：
  // 1. CLAUDE_CODE_DISABLE_AUTO_MEMORY 环境变量 → 禁用
  // 2. --bare / SIMPLE 模式 → 禁用
  // 3. CCR 无持久存储 → 禁用
  // 4. settings.json 中的 autoMemoryEnabled → 遵循设置
  // 5. 默认 → 启用
}
```

#### 四种记忆类型分类 (`memoryTypes.ts`)

系统定义了一个**封闭的四类型分类体系**：

| 类型 | 说明 | 示例 |
|------|------|------|
| **user** | 用户角色、目标、偏好 | "用户是前端工程师，偏好 TypeScript" |
| **feedback** | 对 AI 工作方式的反馈指导 | "不要自动提交代码"、"测试前先问我" |
| **project** | 项目相关的非代码可推导信息 | "v2 API 已废弃，迁移到 v3" |
| **reference** | 外部系统的指针和链接 | "Grafana 面板: https://..." |

同时定义了**不应保存的内容**：

```typescript
const WHAT_NOT_TO_SAVE_SECTION = [
  '## What NOT to save',
  '- Things easily found in the codebase',
  '- Transient debugging context',
  '- Complete code snippets (link to files instead)',
  '- Exact terminal output or error messages',
  '- Information that changes every session',
]
```

### 3.2 Team Memory（团队记忆）

**核心模块**：`src/memdir/teamMemPaths.ts`、`src/services/teamMemorySync/`

团队记忆是 Auto Memory 的共享扩展，通过 API 在团队成员间同步。

#### 双目录架构

当 Team Memory 启用时，系统维护两个目录：

```
~/.claude/projects/<slug>/memory/        ← 私有记忆（仅当前用户）
~/.claude/projects/<slug>/memory/team/   ← 团队记忆（所有协作者共享）
```

#### 作用域路由

每种记忆类型都有推荐的 scope（私有 vs 团队）：

- **user 类型** → 通常是 private（个人偏好）
- **feedback 类型** → 通常是 private（个人工作方式）
- **project 类型** → 通常是 team（项目信息大家共享）
- **reference 类型** → 取决于是否团队共用

### 3.3 Session Memory（会话记忆）

**核心模块**：`src/services/SessionMemory/`

Session Memory 是**当前会话的笔记本**，用于在上下文压缩（Compact）时保留关键信息。

#### 与 Auto Memory 的区别

| 维度 | Session Memory | Auto Memory |
|------|---------------|-------------|
| 生命周期 | 当前会话 | 跨会话持久 |
| 用途 | Compact 后的上下文恢复 | 长期知识积累 |
| 写入方式 | Compact 时自动生成 | 后台 Agent 自动提取 |
| 存储位置 | `~/.claude/session-memory/` | `~/.claude/projects/*/memory/` |

#### 关键实现

Session Memory 在 Compact（上下文压缩）时通过 Forked Agent 生成：

```typescript
// sessionMemory.ts 中的核心逻辑
await runForkedAgent({
  promptMessages: [createUserMessage({ content: userPrompt })],
  cacheSafeParams: createCacheSafeParams(context),
  querySource: 'session_memory',
  // ... 工具限制和配置
})
```

### 3.4 Agent Memory（代理记忆）

**核心模块**：`src/tools/AgentTool/agentMemory.ts`

Agent Memory 为每种 Agent 类型（如 code_reviewer、test_writer 等）维护独立的记忆快照。

```typescript
// 每个 agent 类型有独立的记忆目录
function getAgentMemoryDir(agentType: string): string {
  return join(agentMemoryBase, agentType)
}
```

通过 `agentMemorySnapshot.ts` 实现记忆快照的保存和恢复。

### 3.5 CLAUDE.md 指令层

**核心模块**：`src/utils/claudemd.ts`

CLAUDE.md 不是传统意义上的「记忆」，但它是记忆系统的**指令层**——用户和团队预定义的规则和偏好。

#### 加载顺序与优先级

```
1. Managed (最低优先级)  → /etc/claude-code/CLAUDE.md    → 管理员全局指令
2. User                 → ~/.claude/CLAUDE.md            → 用户全局指令
3. Project              → {project}/CLAUDE.md            → 项目级指令 (Git 追踪)
                          {project}/.claude/CLAUDE.md
                          {project}/.claude/rules/*.md
4. Local (最高优先级)   → {project}/CLAUDE.local.md      → 个人项目指令 (不入 Git)
```

**关键设计**：后加载的文件优先级更高，模型会更关注后出现的指令。

#### @include 指令

CLAUDE.md 支持文件包含指令，最大递归深度 5 层：

```markdown
@./path/to/shared-rules.md     # 相对路径
@~/global-rules.md             # Home 路径
@/etc/claude/org-rules.md      # 绝对路径
```

---

## 四、记忆的创建流程

### 4.1 显式创建

用户可以直接要求 AI 记住信息，AI 通过文件工具写入记忆目录：

```
用户: "记住我更喜欢用 pnpm 而不是 npm"

AI 执行两步操作:
Step 1: 写入记忆文件
  → FileWrite: ~/.claude/projects/<slug>/memory/feedback_package_manager.md
  内容:
  ---
  name: Package Manager Preference
  description: User prefers pnpm over npm
  type: feedback
  ---
  The user prefers pnpm for package management. Always use pnpm commands
  instead of npm when running install, add, or build operations.

Step 2: 更新索引
  → FileEdit: ~/.claude/projects/<slug>/memory/MEMORY.md
  添加行:
  - [Package Manager](feedback_package_manager.md) — Use pnpm, not npm
```

### 4.2 后台自动提取（Extract Memories）

**核心模块**：`src/services/extractMemories/extractMemories.ts`

这是记忆系统最精妙的部分——AI 在对话结束后自动分析是否有值得记住的信息。

#### 触发条件

```typescript
// stopHooks.ts 中的触发逻辑
if (
  feature('EXTRACT_MEMORIES') &&     // Feature Flag 开启
  !toolUseContext.agentId &&          // 非子代理（仅主线程）
  isExtractModeActive()               // 提取模式激活
) {
  void extractMemoriesModule!.executeExtractMemories(...)
}
```

#### 运行模式

Extract Memories 运行为**对主对话的完美 Fork**——它看到完整的对话历史，但在隔离的沙箱中运行：

```typescript
// extractMemories.ts 核心逻辑
const result = await runForkedAgent({
  promptMessages: [createUserMessage({ content: userPrompt })],
  cacheSafeParams,                    // 共享父对话的 Prompt Cache
  querySource: 'extract_memories',
  maxTurns: 3,                        // 最多 3 轮（读 → 写 → 验证）
  onMessage: messageCallback,
  // 工具限制：只允许 FileRead/Write/Edit, Grep, Glob, 只读 Bash
})
```

#### 提取 Prompt 策略

```typescript
function buildExtractAutoOnlyPrompt(newMessageCount, existingMemories) {
  return [
    // 1. 角色声明
    "You are now acting as the memory extraction subagent.",
    "Analyze the most recent ~N messages above...",
    
    // 2. 工具限制
    "Available tools: FileRead, Grep, Glob, read-only Bash, FileEdit/Write for memory dir only",
    
    // 3. 效率要求
    "Turn 1 — issue all FileRead calls in parallel",
    "Turn 2 — issue all FileWrite/FileEdit calls in parallel",
    
    // 4. 内容边界
    "MUST only use content from the last ~N messages. Do not investigate further.",
    
    // 5. 已有记忆清单（防重复）
    existingMemories,
    
    // 6. 类型分类和格式要求
    ...TYPES_SECTION,
    ...WHAT_NOT_TO_SAVE_SECTION,
    ...HOW_TO_SAVE_SECTION,
  ]
}
```

#### 跳过条件

系统会跳过提取当：
- 主 Agent 在本轮已经**主动写过记忆**（`hasMemoryWritesSince` 检查）
- 处于 `--bare` / SIMPLE 模式
- 正在执行子代理任务

### 4.3 快速记忆输入（# 快捷方式）

用户可以在对话中用 `#` 前缀快速保存记忆：

```
# 我更喜欢简短的代码注释
```

这会被渲染为特殊的 `UserMemoryInputMessage` 组件，显示随机确认消息（"Got it." / "Good to know." / "Noted."），并触发记忆保存流程。

---

## 五、记忆的存储格式与组织

### 5.1 文件格式

每个记忆文件使用 **YAML Frontmatter + Markdown 正文** 格式：

```markdown
---
name: API Architecture Notes
description: Key decisions about the v3 REST API design
type: project
---

## REST API v3 Design

The API follows resource-oriented design:
- Base path: /api/v3/
- Auth: Bearer token (JWT)
- Rate limit: 1000 req/min per org

### Migration from v2
- v2 endpoints deprecated since 2024-03
- Automatic redirect layer at gateway
```

### 5.2 索引机制（MEMORY.md）

`MEMORY.md` 是记忆目录的**索引入口**，它被直接注入到系统提示中：

```markdown
- [API Architecture](project_api_v3.md) — REST v3 design decisions and migration notes
- [User Preferences](user_preferences.md) — Code style, testing, and workflow preferences
- [Feedback: Testing](feedback_testing.md) — Run tests before committing, ask before adding deps
- [Grafana Dashboards](reference_dashboards.md) — Links to monitoring and alerting dashboards
```

#### 截断保护

```typescript
const MAX_ENTRYPOINT_LINES = 200     // 最多 200 行
const MAX_ENTRYPOINT_BYTES = 25_000  // 最多 25KB

function truncateEntrypointContent(content: string) {
  // 按行截断 + 按字节截断
  // 超出时添加 "[Truncated — X lines omitted]" 提示
}
```

### 5.3 目录结构

```
~/.claude/
├── CLAUDE.md                          # 用户全局指令
├── projects/
│   └── <project-slug>/
│       └── memory/                    # Auto Memory 根目录
│           ├── MEMORY.md              # 索引入口
│           ├── user_preferences.md    # 用户偏好
│           ├── feedback_testing.md    # 反馈记忆
│           ├── project_api_v3.md      # 项目知识
│           ├── reference_dashboards.md# 外部引用
│           ├── .consolidate-lock      # Dream 锁文件
│           ├── logs/                  # 助手模式日志
│           │   └── 2025/04/
│           │       └── 2025-04-23.md
│           └── team/                  # 团队记忆
│               ├── MEMORY.md          # 团队索引
│               └── team_conventions.md
├── session-memory/                    # 会话记忆
└── agent-memory/                      # Agent 记忆
    ├── code_reviewer/
    └── test_writer/
```

---

## 六、记忆的检索与使用

### 6.1 系统提示注入

记忆通过 `loadMemoryPrompt()` 注入到系统提示中，它是每个会话启动时的关键步骤：

```typescript
// memdir.ts
async function loadMemoryPrompt(): Promise<string | null> {
  const autoEnabled = isAutoMemoryEnabled()

  // 根据不同模式选择不同的提示构建器
  if (isKairosAssistantMode() && autoEnabled) {
    return buildAssistantDailyLogPrompt()  // 助手模式：每日日志
  }
  if (isTeamMemEnabled() && autoEnabled) {
    return buildCombinedMemoryPrompt()     // 团队+个人：联合提示
  }
  if (autoEnabled) {
    return buildSingleDirMemoryPrompt()    // 仅个人：单目录提示
  }
  return null                              // 全部禁用
}
```

在系统提示中的位置：

```typescript
// prompts.ts - 系统提示构建
const dynamicSections = [
  systemPromptSection('session_guidance', () => getSessionGuidanceSection()),
  systemPromptSection('memory', () => loadMemoryPrompt()),  // ← 记忆注入
  systemPromptSection('env_info', () => computeEnvInfo()),
  systemPromptSection('language', () => getLanguageSection()),
  // ...
]
```

`systemPromptSection` 使用**缓存机制**——同一个 session 内只计算一次，避免重复的文件 I/O。

### 6.2 相关性检索（Relevant Memory Surfacing）

这是记忆系统最具技术含量的部分——在每个 Turn 开始时，异步预取与当前问题最相关的记忆文件。

#### 预取机制 (`attachments.ts`)

```typescript
// query.ts 中的预取启动
using pendingMemoryPrefetch = startRelevantMemoryPrefetch(
  state.messages,
  state.toolUseContext,
)
```

使用 `using` 关键字绑定 Disposable，确保在任何退出路径（return、throw、.return()）上都能正确清理。

#### 检索流程

```
1. 提取最后一条真实用户消息（跳过 isMeta 系统注入）
   ↓
2. 扫描记忆目录获取所有 .md 文件的 Header（frontmatter）
   ↓
3. 过滤已展示的记忆（防止重复）
   ↓
4. 调用 Sonnet 模型（通过 sideQuery）评估相关性
   ↓
5. 返回最多 5 个最相关的记忆文件路径
   ↓
6. 读取文件内容（限制 200 行 / 4KB）
   ↓
7. 作为 <system-reminder> 附件注入到下一轮对话
```

#### 相关性选择器（AI 驱动）

```typescript
// findRelevantMemories.ts
const response = await sideQuery({
  systemPrompt: "You are a memory relevance selector...",
  userMessage: `
    User query: ${userInput}
    
    Available memories:
    ${memoryHeaders.map(h => `- ${h.name}: ${h.description}`)}
    
    Select up to 5 most relevant memories.
  `,
  outputSchema: {
    type: 'object',
    properties: {
      selected: {
        type: 'array',
        items: { type: 'string' },  // 文件名列表
        maxItems: 5
      }
    }
  }
})
```

#### 流量控制

```typescript
const RELEVANT_MEMORIES_CONFIG = {
  // 每轮注入上限：5 个文件 × 4KB = 20KB
  MAX_MEMORIES_PER_TURN: 5,
  MAX_MEMORY_BYTES: 4096,
  
  // 会话级累计上限：约 3 次完整注入后停止预取
  // 原因：最重要的记忆已经在上下文中了
  MAX_SESSION_BYTES: 60_000,
  
  // 每 N 个附件发送一次完整提醒
  FULL_REMINDER_EVERY_N_ATTACHMENTS: 5,
}
```

### 6.3 记忆时效性管理

**核心模块**：`src/memdir/memoryAge.ts`

每个记忆文件都带有时效性信息，帮助 AI 判断记忆的可靠性：

```typescript
function memoryAge(mtimeMs: number): string {
  const days = memoryAgeDays(mtimeMs)
  if (days === 0) return 'today'
  if (days === 1) return 'yesterday'
  return `${days} days ago`
}

function memoryFreshnessText(mtimeMs: number): string {
  if (memoryAgeDays(mtimeMs) <= 1) return ''
  return `⚠️ This memory was last updated ${memoryAge(mtimeMs)}. It may be outdated.`
}
```

时效性提醒通过 `<system-reminder>` 标签注入：

```
<system-reminder>
Memory "API Architecture" was saved 12 days ago — verify before relying on it.
</system-reminder>
```

### 6.4 附件注入机制

记忆通过**附件系统**（Attachment System）注入到对话中，而非直接拼接到消息文本。

附件类型定义：

```typescript
type Attachment =
  | { type: 'nested_memory'; path: string; content: MemoryFileInfo }   // CLAUDE.md 层级
  | { type: 'relevant_memories'; memories: RelevantMemory[] }          // 相关记忆
  | { type: 'current_session_memory'; content: string; path: string }  // 会话记忆
```

去重机制（防止重复注入）：

```typescript
function filterDuplicateMemoryAttachments(
  attachments: Attachment[],
  readFileState: FileStateCache,
): Attachment[] {
  return attachments.map(att => {
    if (att.type !== 'relevant_memories') return att
    // 过滤掉模型已经通过 FileRead/Edit/Write 接触过的文件
    const filtered = att.memories.filter(m => !readFileState.has(m.path))
    return { ...att, memories: filtered }
  })
}
```

---

## 七、记忆整合与维护（Auto Dream）

**核心模块**：`src/services/autoDream/`

Auto Dream 是记忆系统的「整理员」——它定期回顾和整合记忆，移除过时信息，合并重复条目。

### 7.1 触发条件与门控

Auto Dream 采用**多级门控**策略，从最便宜的检查开始（减少不必要的开销）：

```typescript
async function executeAutoDream(context, appendSystemMessage) {
  // Gate 1: 时间门 — 距上次整合 >= 24小时
  const lastConsolidatedAt = await readLastConsolidatedAt()
  if (Date.now() - lastConsolidatedAt < minHours * 3600 * 1000) return

  // Gate 2: 会话门 — 自上次整合以来 >= 5 个会话
  const sessions = await listSessionsTouchedSince(lastConsolidatedAt)
  if (sessions.length < minSessions) return

  // Gate 3: 扫描节流 — 10分钟内不重复扫描
  if (Date.now() - lastScanTime < SCAN_THROTTLE_MS) return

  // Gate 4: 锁门 — 无其他进程在整合中
  const priorMtime = await tryAcquireConsolidationLock()
  if (priorMtime === null) return

  // 所有门通过 → 执行整合
  await runForkedAgent({...})
}
```

### 7.2 整合流程的四个阶段

整合通过一个精心设计的 Prompt 指导 Forked Agent 执行四个阶段：

#### Phase 1 — Orient（定向）

```markdown
- `ls` the memory directory to see what already exists
- Read `MEMORY.md` to understand the current index
- Skim existing topic files so you improve them rather than creating duplicates
```

#### Phase 2 — Gather（收集信号）

```markdown
Look for new information worth persisting. Sources in rough priority order:
1. Daily logs (logs/YYYY/MM/YYYY-MM-DD.md) if present
2. Existing memories that drifted — facts that contradict the codebase
3. Transcript search — grep JSONL transcripts for narrow terms
```

#### Phase 3 — Consolidate（整合）

```markdown
Focus on:
- Merging new signal into existing topic files (not creating near-duplicates)
- Converting relative dates to absolute dates
- Deleting contradicted facts
```

#### Phase 4 — Prune and Index（修剪索引）

```markdown
Update MEMORY.md so it stays under 200 lines AND under ~25KB:
- Remove pointers to stale/wrong/superseded memories
- Demote verbose entries
- Add pointers to newly important memories
- Resolve contradictions
```

### 7.3 锁机制

**核心模块**：`src/services/autoDream/consolidationLock.ts`

使用**文件系统锁**协调多个 Claude Code 进程的 Dream 操作：

```typescript
// 锁文件: memory/.consolidate-lock
// 文件内容: 持有者的 PID
// 文件 mtime: lastConsolidatedAt 时间戳（一石二鸟）

async function tryAcquireConsolidationLock(): Promise<number | null> {
  // 1. 读取现有锁
  const [stat, pid] = await Promise.all([stat(lockPath), readFile(lockPath)])
  
  // 2. 检查是否过期（1小时后过期，防PID重用）
  if (Date.now() - stat.mtimeMs < HOLDER_STALE_MS) {
    if (isProcessRunning(pid)) return null  // 被活跃进程持有
  }
  
  // 3. 写入当前 PID
  await writeFile(lockPath, String(process.pid))
  
  // 4. 验证写入（两个竞争者都写入时，后写者胜出）
  const verify = await readFile(lockPath)
  if (parseInt(verify) !== process.pid) return null  // 竞争失败
  
  return priorMtime  // 返回之前的 mtime（用于失败回滚）
}
```

#### 失败回滚

```typescript
async function rollbackConsolidationLock(priorMtime: number) {
  if (priorMtime === 0) {
    await unlink(lockPath)       // 恢复到无锁状态
  } else {
    await writeFile(lockPath, '')
    await utimes(lockPath, priorMtime/1000, priorMtime/1000)  // 回退 mtime
  }
}
```

### Dream 任务 UI

Dream 运行时会在 TUI 底部显示状态：

```typescript
// DreamTask.ts
type DreamTaskState = {
  type: 'dream'
  phase: 'starting' | 'updating'     // 首次 Edit/Write 时切换
  sessionsReviewing: number           // 正在回顾的会话数
  filesTouched: string[]              // 已修改的文件列表
  turns: DreamTurn[]                  // Agent 的执行步骤
  abortController?: AbortController   // 支持用户中断
  priorMtime: number                  // 回滚用的 mtime
}
```

用户可以通过 Shift+Down 查看 Dream 详情对话框，也可以 Kill 正在运行的 Dream。

---

## 八、团队记忆同步机制

### 8.1 同步架构

```
┌──────────────┐                ┌──────────────────┐
│  本地文件系统  │  ──Push──>    │  团队记忆 API     │
│  team/       │  <──Pull──    │  (云端同步)       │
│              │                │                  │
│  ┌─────────┐ │                │  GitHub repo     │
│  │ watcher │ │  ──Notify──>  │  识别为项目标识   │
│  └─────────┘ │                └──────────────────┘
└──────────────┘
```

### 8.2 文件监视器

**核心模块**：`src/services/teamMemorySync/watcher.ts`

```typescript
// 启动条件
if (
  feature('TEAMMEM') &&           // Feature Flag
  isTeamMemoryEnabled() &&         // 用户设置
  isTeamMemorySyncAvailable() &&   // API 可用
  hasGitHubRemote()                // 必须有 GitHub 远程仓库
) {
  startTeamMemoryWatcher()
}
```

监视器的关键设计：

| 特性 | 实现 |
|------|------|
| **初始同步** | 启动时先 Pull 最新团队记忆 |
| **实时监听** | `fs.watch({recursive: true})` |
| **防抖** | 2 秒防抖后才推送（避免写入中间态） |
| **错误抑制** | 永久性失败（403/413/无OAuth）后抑制重试 |
| **恢复机制** | 文件删除（unlink）自动清除抑制标记 |
| **关闭刷新** | 进程退出时的 2 秒优雅窗口内尽力推送 |

### 8.3 安全防护

#### 客户端密钥扫描 (`secretScanner.ts`)

在上传前拦截敏感信息，**密钥永远不离开用户机器**：

```typescript
// 覆盖 34 种密钥模式
const SECRET_RULES = [
  { id: 'aws-access-token', source: '\\b(AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16}\\b' },
  { id: 'anthropic-api-key', source: `\\b(${ANT_KEY_PFX}03-...)` },
  { id: 'github-pat', source: 'ghp_[0-9a-zA-Z]{36}' },
  { id: 'private-key', source: '-----BEGIN.*PRIVATE KEY-----' },
  // ... 30+ 种更多模式
]
```

#### 写入守卫 (`teamMemSecretGuard.ts`)

在 FileWriteTool 和 FileEditTool 的 `validateInput` 中调用：

```typescript
function checkTeamMemSecrets(filePath: string, content: string): string | null {
  if (!isTeamMemPath(filePath)) return null
  
  const matches = scanForSecrets(content)
  if (matches.length === 0) return null
  
  return `Content contains potential secrets (${labels}) and cannot be written to team memory.`
}
```

关键安全特性：
- **规则精选**：只使用有明确前缀的高置信度规则（来自 gitleaks 的子集）
- **不返回匹配文本**：扫描结果只包含规则 ID 和标签，不包含实际密钥内容
- **延迟编译**：正则表达式在首次扫描时才编译，减少启动开销
- **脱敏函数**：`redactSecrets()` 可将匹配内容替换为 `[REDACTED]`

---

## 九、Forked Agent 基础设施

**核心模块**：`src/utils/forkedAgent.ts`

Forked Agent 是记忆系统（以及其他后台任务）的运行基础设施。

### 9.1 核心概念

Forked Agent 是主对话的「影子分身」：

```
┌─────────────────────────────┐
│      主对话 (Main Thread)    │
│                             │
│  System Prompt ─────────┐   │
│  Message History ───────┤   │
│  Tool Config ───────────┤   │
│  Thinking Config ───────┤   │  CacheSafeParams
│                         │   │  (共享 Prompt Cache)
│                         ▼   │
│  ┌────────────────────────┐ │
│  │ saveCacheSafeParams()  │ │
│  └──────────┬─────────────┘ │
└─────────────┼───────────────┘
              │
    ┌─────────┼─────────────────────────────────────┐
    │         ▼                                     │
    │  ┌──────────────┐  ┌──────────────┐           │
    │  │Extract Memory│  │ Auto Dream   │  ...      │
    │  │Forked Agent  │  │Forked Agent  │           │
    │  │              │  │              │           │
    │  │Same prompt   │  │Same prompt   │           │
    │  │Same cache    │  │Same cache    │           │
    │  │Isolated ctx  │  │Isolated ctx  │           │
    │  └──────────────┘  └──────────────┘           │
    │           后台 Fork 集群                       │
    └───────────────────────────────────────────────┘
```

### 9.2 Prompt Cache 共享

这是 Forked Agent 最重要的优化——**所有 Fork 共享父对话的 Prompt Cache**：

```typescript
type CacheSafeParams = {
  systemPrompt: SystemPrompt      // 系统提示（不变）
  userContext: Record<string, string>  // 用户上下文（不变）
  systemContext: Record<string, string> // 系统上下文（不变）
  toolUseContext: ToolUseContext   // 工具配置（不变）
  forkContextMessages: Message[]  // 对话历史前缀（不变）
}
```

这些参数构成了 API 请求的 **Cache Key**——只要它们不变，所有 Fork 都能命中同一个缓存条目，**极大降低 API 成本**。

```typescript
// 每次 Turn 结束时保存 CacheSafeParams
// stopHooks.ts
if (querySource === 'repl_main_thread' || querySource === 'sdk') {
  saveCacheSafeParams(createCacheSafeParams(stopHookContext))
}
```

### 9.3 隔离与安全

`createSubagentContext()` 创建隔离环境，防止 Fork 修改父状态：

```typescript
function createSubagentContext(parent: ToolUseContext): ToolUseContext {
  return {
    ...parent,
    readFileState: new Map(parent.readFileState),  // 克隆（不污染父状态）
    abortController: new AbortController(),         // 独立的取消控制
    getAppState: () => ({
      ...parent.getAppState(),
      shouldAvoidPermissionPrompts: true,           // 静默执行
    }),
    // 所有变异回调默认 no-op（不影响主对话 UI）
    appendSystemMessage: () => {},
    addNotification: () => {},
  }
}
```

### 使用 Forked Agent 的系统

| 系统 | querySource | maxTurns | 用途 |
|------|-------------|----------|------|
| Extract Memories | `extract_memories` | 3 | 自动提取记忆 |
| Auto Dream | `auto_dream` | 10+ | 记忆整合 |
| Session Memory | `session_memory` | 2 | 会话记忆生成 |
| Compact | `compact` | 1 | 上下文压缩 |
| Agent Summary | `agent_summary` | 1 | 子代理进度摘要 |
| Prompt Suggestion | `prompt_suggestion` | 1 | 下一步建议 |

---

## 十、记忆管理（用户界面）

### 10.1 /memory 命令

**核心模块**：`src/commands/memory/memory.tsx`

`/memory` 命令提供交互式记忆文件管理：

```
> /memory

┌── Memory ──────────────────────────────┐
│                                         │
│  📁 CLAUDE.md files                     │
│    ~/.claude/CLAUDE.md                  │
│    ./CLAUDE.md                          │
│    ./CLAUDE.local.md                    │
│                                         │
│  📁 Auto-memory files                   │
│    ~/.claude/projects/.../memory/       │
│      MEMORY.md                          │
│      user_preferences.md                │
│      feedback_testing.md                │
│                                         │
│  Select a file to edit...               │
└─────────────────────────────────────────┘
```

选择文件后，使用 `$EDITOR` 或 `$VISUAL` 打开编辑器。

### 10.2 /remember 技能

**核心模块**：`src/skills/bundled/remember.ts`

`/remember` 是一个高级记忆审计技能（仅限内部用户），它会：

1. **收集所有记忆层** — 读取 CLAUDE.md、CLAUDE.local.md、Auto Memory、Team Memory
2. **分类每个条目** — 判断最佳归属位置
3. **识别清理机会** — 找出重复、过时、冲突的条目
4. **生成报告** — 分为 Promotions / Cleanup / Ambiguous / No Action

分类规则：

| 目标位置 | 适合内容 | 示例 |
|---------|---------|------|
| CLAUDE.md | 所有贡献者应遵循的项目规范 | "用 bun 不用 npm" |
| CLAUDE.local.md | 个人指令，不适用他人 | "我偏好简洁回复" |
| Team Memory | 跨仓库的组织知识 | "staging 在 staging.internal" |
| 留在 Auto Memory | 临时笔记或不确定归属 | 会话观察 |

### 10.3 /dream 命令

手动触发记忆整合（无需等待自动触发条件）：

```
> /dream
```

执行时调用 `recordConsolidation()` 写入乐观时间戳，然后运行与 Auto Dream 相同的整合流程。

---

## 十一、与其他工作流的集成

### 11.1 Compact（上下文压缩）集成

当对话上下文过长需要压缩时：

```
Compact 触发
    ↓
生成 Session Memory（保存关键上下文到文件）
    ↓
清除 getUserContext 缓存（CLAUDE.md 层）
    ↓
重置 getMemoryFiles 缓存
    ↓
触发 InstructionsLoaded Hook（允许外部审计）
    ↓
重新加载记忆提示（下次 Turn 生效）
```

关键代码 (`postCompactCleanup.ts`)：

```typescript
// 仅主线程 Compact 才重置（子代理共享模块级状态）
if (!isSubagent) {
  getUserContext.cache = undefined     // 清除外层缓存
  resetGetMemoryFilesCache('compact')  // 清除内层缓存
}
```

### 11.2 Query Loop 集成

在主查询循环中，记忆参与了多个关键节点：

```typescript
// query.ts 简化流程

async function* queryLoop() {
  // 1. Turn 开始: 预取相关记忆
  using pendingMemoryPrefetch = startRelevantMemoryPrefetch(messages, ctx)
  
  while (hasMoreTurns) {
    // 2. 工具执行
    const toolResults = await executeTools(...)
    
    // 3. 检查预取是否完成
    if (pendingMemoryPrefetch?.settledAt !== null) {
      const memories = filterDuplicateMemoryAttachments(
        await pendingMemoryPrefetch.promise,
        readFileState,
      )
      // 注入为附件
      for (const mem of memories) yield createAttachmentMessage(mem)
    }
    
    // 4. 流式响应
    yield* streamResponse(...)
  }
  
  // 5. Turn 结束: 触发 StopHooks → Extract Memories + Auto Dream
}
```

### 11.3 Stop Hooks 集成

每次 Turn 结束后的 `handleStopHooks` 中：

```typescript
// stopHooks.ts
if (!isBareMode()) {
  // 1. 保存 CacheSafeParams（供后续 Fork 使用）
  saveCacheSafeParams(createCacheSafeParams(context))
  
  // 2. 触发记忆提取（仅主线程 + ExtractMemories 特性开启）
  if (feature('EXTRACT_MEMORIES') && !agentId && isExtractModeActive()) {
    void extractMemoriesModule.executeExtractMemories(context, appendSystemMessage)
  }
  
  // 3. 触发记忆整合（仅主线程）
  if (!agentId) {
    void executeAutoDream(context, appendSystemMessage)
  }
}
```

### 11.4 工具权限集成

记忆文件的读写受工具权限系统保护：

- **FileReadTool**：读取记忆文件需要路径在允许范围内
- **FileWriteTool/FileEditTool**：写入团队记忆文件时触发 `checkTeamMemSecrets` 密钥检查
- **BashTool**：Extract Memories Fork 中 Bash 限制为只读（禁止 `rm`）
- **路径验证**：`isAutoMemPath()` 安全地检查路径是否在记忆目录内（防止路径遍历攻击）

```typescript
// pathValidation.ts 中的记忆路径安全检查
function isAutoMemPath(filePath: string): boolean {
  const memPath = getAutoMemPath()
  const resolved = resolve(filePath)
  // 确保路径确实在记忆目录内（防止 ../../../etc/passwd 式攻击）
  return resolved.startsWith(memPath + sep) || resolved === memPath
}
```

---

## 十二、关键设计模式与技术亮点

### 12.1 「门控链」模式 (Gate Chain)

系统大量使用门控链——从最便宜的检查开始，逐步深入：

```
环境变量检查 (0 cost)
  → Feature Flag 检查 (0 cost)
    → 文件 stat 检查 (1 syscall)
      → 文件内容读取 (N syscalls)
        → AI 模型调用 ($$$)
```

### 12.2 「Disposable Prefetch」模式

使用 TypeScript 的 `using` + `Symbol.dispose` 实现零泄漏的异步预取：

```typescript
// 自动在任何退出路径上清理
using prefetch = startPrefetch()
// ... 正常逻辑
// 即使抛出异常，prefetch 也会被正确 abort
```

### 12.3 「最终一致性」模式

所有后台任务都是 fire-and-forget：

```typescript
// 不等待，不阻塞主流程
void executeExtractMemories(...)  // 提取
void executeAutoDream(...)        // 整合
```

失败后有自动退避机制（扫描节流、锁文件 mtime 回滚）。

### 12.4 「Prompt Cache 共享」模式

所有 Forked Agent 共享父对话的 Prompt Cache Key，这是一个**关键的成本优化**——让后台任务的 API 调用成本几乎为零（命中缓存的部分不计费）。

### 12.5 「文件系统锁」模式

使用文件的 mtime 作为时间戳 + 文件内容作为 PID 持有者标识，实现了一个轻量级的跨进程锁，无需额外依赖。

### 12.6 「层级覆盖」模式

记忆/指令的加载遵循明确的层级优先关系：
- 后加载的优先级高
- 本地优先于全局
- 用户优先于管理员
- 但管理员可以通过特定机制强制覆盖

### 12.7 「记忆矫正提示」模式

当用户取消或拒绝 AI 的操作时，系统自动注入提示让 AI 关注用户可能表达的偏好：

```typescript
const MEMORY_CORRECTION_HINT = 
  "Note: The user's next message may contain a correction or preference. " +
  "Pay close attention — if they explain what went wrong or how they'd prefer " +
  "you to work, consider saving that to memory for future sessions."
```

---

## 十三、类型系统

### 记忆类型定义

```typescript
// utils/memory/types.ts
type MemoryType = 
  | 'User'       // ~/.claude/CLAUDE.md
  | 'Project'    // {project}/CLAUDE.md, .claude/CLAUDE.md, .claude/rules/*.md
  | 'Local'      // {project}/CLAUDE.local.md
  | 'Managed'    // /etc/claude-code/CLAUDE.md
  | 'AutoMem'    // ~/.claude/projects/*/memory/
  | 'TeamMem'    // (feature flag 控制)

// memdir/memoryTypes.ts - 记忆内容类型
type MemoryContentType = 'user' | 'feedback' | 'project' | 'reference'
```

### 记忆文件信息

```typescript
// utils/claudemd.ts
type MemoryFileInfo = {
  path: string
  type: MemoryType
  content: string
  parent?: string              // @include 的父文件
  globs?: string[]             // 适用的文件路径 glob
  contentDiffersFromDisk?: boolean  // 内容是否被处理过（截断/strip）
  rawContent?: string          // 原始磁盘内容
}
```

### 相关记忆头信息

```typescript
// memdir/memoryScan.ts
type MemoryHeader = {
  name: string
  description: string
  type: string
  path: string
  mtimeMs: number
}
```

### 附件类型

```typescript
// utils/attachments.ts
type RelevantMemoryAttachment = {
  type: 'relevant_memories'
  memories: {
    path: string
    content: string
    mtimeMs: number
    header?: string  // 预计算的头部（保持缓存稳定）
  }[]
}
```

---

## 十四、总结

Claude Code 的记忆系统是一个**精心设计的多层次、多维度持久化学习系统**，其核心技术特点可以总结为：

### 架构优势

| 维度 | 设计选择 | 优势 |
|------|---------|------|
| **存储** | 文件系统 | 透明、可审计、零依赖 |
| **分层** | 6 层作用域 | 灵活的可见性控制 |
| **提取** | 后台 Forked Agent | 不打断用户交互 |
| **检索** | AI 驱动相关性评估 | 精准的上下文增强 |
| **整合** | Auto Dream 定期整理 | 自动维护记忆质量 |
| **同步** | 文件监视 + API | 实时团队协作 |
| **安全** | 多层密钥扫描 | 密钥永不泄露 |
| **成本** | Prompt Cache 共享 | 后台任务近零成本 |

### 设计哲学总结

1. **渐进式学习** — AI 在协作中自然积累知识，无需用户手动配置
2. **文件即记忆** — 朴素但强大，用户拥有完全控制权
3. **影子分身** — 后台 Fork 模式优雅地处理异步智能任务
4. **最终一致** — 所有后台任务都是尽力而为，失败不影响主流程
5. **安全第一** — 密钥扫描、路径验证、写入守卫多层防护
6. **成本敏感** — Prompt Cache 共享使后台任务几乎零成本

这个系统展示了如何在保持简单性的同时实现复杂的智能记忆管理——文件系统作为存储层提供了极佳的透明度和可维护性，而 Forked Agent 模式则为异步智能处理提供了优雅的基础设施。
