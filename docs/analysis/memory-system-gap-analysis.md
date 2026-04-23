# 记忆系统差异分析报告

> **文档状态**：功能规划  
> **创建日期**：2026-04-23  
> **跟踪 Issue**：#TBD（待创建）  
> **相关文档**：[`memory-system-design.md`](./origin/memory-system-design.md)

---

## 概述

本文档基于对 Claude Code TypeScript 原版记忆系统的深入分析（见 `origin/memory-system-design.md`），
与当前 Go 版本实现进行逐项对比，识别差距并规划改进路径。

原版记忆系统包含 **6 大核心子系统**（Auto Memory、Extract Memories、Auto Dream、Session Memory、
Team Memory、Relevant Memory Surfacing）和 **完整的 Forked Agent 基础设施**。
当前 Go 版本仅实现了基础存储层和 CLAUDE.md 加载，高级智能功能几乎全部缺失。

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

### 1.1 Auto Memory 基础存储层 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 记忆目录路径计算 (`autoMemoryPath`) | ✅ 完成 | `internal/memdir/store.go` |
| 项目 slug 算法 (`sanitizeProjectSlug`) | ✅ 完成 | `internal/memdir/store.go` |
| `MemoryStore` 读写接口 | ✅ 完成 | `internal/memdir/store.go` |
| MEMORY.md 索引构建 (`BuildIndex`) | ✅ 完成 | `internal/memdir/store.go` |
| 索引截断保护 | ✅ 完成 | `MaxMemoryIndexLines=200`, `MaxMemoryIndexBytes=25000` |

**关键常量与 TS 原版一致**：
```go
// internal/memdir/store.go
const DefaultMemoryBase = ".claude"
const MemoryFileName    = "MEMORY.md"
const MaxMemoryIndexLines = 200
const MaxMemoryIndexBytes = 25_000
```

**路径格式**：`~/.claude/projects/<basename>-<sha256_8hex>/memory/`

### 1.2 记忆类型系统 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 四种记忆内容类型 | ✅ 完成 | `internal/memdir/types.go` |
| YAML Frontmatter 解析 | ✅ 完成 | `internal/memdir/types.go` |
| Frontmatter 序列化 | ✅ 完成 | `internal/memdir/types.go` |
| `MemoryHeader` / `MemoryFile` 类型 | ✅ 完成 | `internal/memdir/types.go` |

**支持的记忆类型**：
- `user` — 用户偏好（编码风格、工具选择）
- `feedback` — 反馈修正（对 AI 工作方式的指导）
- `project` — 项目知识（架构、约定）
- `reference` — 外部引用（文档、URL）

### 1.3 CLAUDE.md 发现与加载 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 向上遍历发现 CLAUDE.md | ✅ 完成 | `internal/memdir/discover.go` |
| Home 目录去重 | ✅ 完成 | `internal/memdir/discover.go` |
| 文件内容拼接 (`LoadMemoryPrompt`) | ✅ 完成 | `internal/memdir/loader.go` |
| UTF-8 安全截断 (`LoadAndTruncate`) | ✅ 完成 | `internal/memdir/loader.go` |
| CLAUDE.md + MEMORY.md 联合加载 (`LoadAllMemory`) | ✅ 完成 | `internal/memdir/loader.go` |
| TUI 异步加载集成 | ✅ 完成 | `internal/tui/init.go`, `update.go`, `cmds.go` |

**TUI 集成流程**：
```
Init() → loadMemdirCmd() → MemdirLoadedMsg → update.go 存储到 m.memdirPrompt
                                              → cmds.go 注入 engine.SystemPrompt
```

### 1.4 LLM 记忆工具 ✅

| 工具 | 状态 | 文件位置 |
|------|------|----------|
| `MemoryRead` — 列出/读取记忆 | ✅ 完成 | `internal/tools/memory/memory.go` |
| `MemoryWrite` — 创建/更新记忆 | ✅ 完成 | `internal/tools/memory/memory.go` |
| `MemoryDelete` — 删除记忆 | ✅ 完成 | `internal/tools/memory/memory.go` |
| 工具注册 | ✅ 完成 | `internal/bootstrap/tools.go` |

### 1.5 /memory 斜杠命令 ⚠️（部分实现）

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| `/memory` 命令注册 | ✅ 完成 | `internal/commands/builtins.go` |
| 展示记忆系统状态 | ⚠️ 仅固定文本 | `internal/commands/builtins.go:269-299` |

---

## 二、未实现/不完整功能

### 2.1 ❌ CLAUDE.md 多层作用域（最大架构差距之一）

**原版实现**：
```
1. Managed (最低优先级)  → /etc/claude-code/CLAUDE.md    → 管理员全局指令
2. User                 → ~/.claude/CLAUDE.md            → 用户全局指令
3. Project              → {project}/CLAUDE.md            → 项目级指令 (Git 追踪)
                          {project}/.claude/CLAUDE.md
                          {project}/.claude/rules/*.md
4. Local (最高优先级)   → {project}/CLAUDE.local.md      → 个人项目指令 (不入 Git)
```

原版还支持 `@include` 指令（最大递归 5 层）：
```markdown
@./path/to/shared-rules.md     # 相对路径
@~/global-rules.md             # Home 路径
```

**当前 Go 实现**：
```go
// internal/memdir/discover.go — DiscoverClaudeMd()
// 仅遍历 startDir → parents → home，只查找 CLAUDE.md
p := filepath.Join(d, "CLAUDE.md")
if isReadableFile(p) { paths = append(paths, p) }
```

**差距分析**：
- ❌ 不支持 `/etc/claude-code/CLAUDE.md`（Managed 层）
- ❌ 不支持 `CLAUDE.local.md`（Local 层）
- ❌ 不支持 `.claude/CLAUDE.md` 子目录
- ❌ 不支持 `.claude/rules/*.md` 规则目录
- ❌ 不支持 `@include` 指令
- ❌ 没有层级优先级语义（后加载优先）
- ❌ 没有 `MemoryType`（User/Project/Local/Managed）分层标识

**建议优先级**：🔴 P1（核心功能差距）

**修复难度**：中（~200 行代码）

---

### 2.2 ❌ 记忆提取（Extract Memories）— 后台自动学习

**原版实现**：

```typescript
// src/services/extractMemories/extractMemories.ts
// 每个 Turn 结束时通过 stopHooks 触发 Forked Agent
const result = await runForkedAgent({
  promptMessages: [createUserMessage({ content: userPrompt })],
  cacheSafeParams,            // 共享父对话的 Prompt Cache
  querySource: 'extract_memories',
  maxTurns: 3,                // 最多 3 轮（读 → 写 → 验证）
})
```

核心特性：
- 在每个 Turn 结束后**自动分析对话**，提取值得记住的信息
- 通过 Forked Agent 在**后台异步运行**，不打断用户交互
- 自动跳过主 Agent 已经主动写过记忆的情况（`hasMemoryWritesSince` 检查）
- 精心设计的 Prompt 策略（分类规则 + 不应保存的内容 + 效率要求）

**当前 Go 实现**：
- **完全没有实现**
- 没有 `extractMemories` 相关代码
- 没有 `stopHooks` 触发机制
- 前置依赖：Forked Agent 基础设施

**建议优先级**：🔴 P1（记忆系统核心功能）

**修复难度**：高（需要先实现 Forked Agent 基础设施）

---

### 2.3 ❌ 记忆整合（Auto Dream）

**原版实现**：

```typescript
// src/services/autoDream/
// 多级门控 → 四阶段整合 → 文件系统锁

// Gate 1: 时间门 — 距上次整合 >= 24小时
// Gate 2: 会话门 — 自上次整合以来 >= 5 个会话
// Gate 3: 扫描节流 — 10分钟内不重复扫描
// Gate 4: 锁门 — 无其他进程在整合中

// Phase 1: Orient（定向）— ls + read MEMORY.md
// Phase 2: Gather（收集）— 从日志、drift、对话中收集新信息
// Phase 3: Consolidate（整合）— 合并新旧，删除矛盾
// Phase 4: Prune and Index（修剪索引）— 保持 MEMORY.md < 200 lines
```

核心特性：
- 定期回顾和整合记忆，移除过时信息，合并重复条目
- 使用文件系统锁（`.consolidate-lock`）协调多进程
- 锁文件 mtime 作为 `lastConsolidatedAt` 时间戳（一石二鸟）
- 失败回滚机制

**当前 Go 实现**：
- **完全没有实现**
- `MemoryHeader.Source` 类型定义中提到 `"dream"` 作为可能的来源值，但仅是占位
- 没有 `/dream` 斜杠命令
- 前置依赖：Forked Agent 基础设施

**建议优先级**：🟡 P2（记忆质量维护）

**修复难度**：高

---

### 2.4 ❌ 会话记忆（Session Memory）

**原版实现**：
```typescript
// src/services/SessionMemory/
// Compact 时通过 Forked Agent 生成
await runForkedAgent({
  promptMessages: [createUserMessage({ content: userPrompt })],
  cacheSafeParams: createCacheSafeParams(context),
  querySource: 'session_memory',
})
```

| 维度 | Session Memory | Auto Memory |
|------|---------------|-------------|
| 生命周期 | 当前会话 | 跨会话持久 |
| 用途 | Compact 后的上下文恢复 | 长期知识积累 |
| 存储位置 | `~/.claude/session-memory/` | `~/.claude/projects/*/memory/` |

**当前 Go 实现**：
- **完全没有实现**
- `internal/compact/compact.go` 中定义了 `CompactionResult.Attachments` 字段，但未使用
- Compact 后不生成会话记忆，上下文恢复能力受限

**建议优先级**：🟡 P2（Compact 体验）

**修复难度**：高（需要 Forked Agent）

---

### 2.5 ❌ 相关记忆检索（Relevant Memory Surfacing）

**原版实现**：

```typescript
// 每个 Turn 开始时异步预取相关记忆
using pendingMemoryPrefetch = startRelevantMemoryPrefetch(
  state.messages,
  state.toolUseContext,
)
```

检索流程：
```
1. 提取最后一条用户消息
2. 扫描记忆目录获取所有 .md 文件的 Header（frontmatter）
3. 过滤已展示的记忆（防重复）
4. 调用 Sonnet 模型评估相关性（sideQuery）
5. 返回最多 5 个最相关的记忆文件路径
6. 读取文件内容（限制 200 行 / 4KB）
7. 作为 <system-reminder> 附件注入到下一轮对话
```

流量控制：
```typescript
const RELEVANT_MEMORIES_CONFIG = {
  MAX_MEMORIES_PER_TURN: 5,
  MAX_MEMORY_BYTES: 4096,
  MAX_SESSION_BYTES: 60_000,
  FULL_REMINDER_EVERY_N_ATTACHMENTS: 5,
}
```

**当前 Go 实现**：
- **完全没有实现**
- 没有 `sideQuery` 机制
- 没有相关性评估 AI 调用
- 没有附件注入系统

**差距影响**：当前 Go 版本的记忆只能通过 MEMORY.md 索引注入系统提示，无法**按需精准检索**相关记忆。用户积累大量记忆后，仅靠索引无法有效利用。

**建议优先级**：🔴 P1（记忆利用率核心功能）

**修复难度**：高

---

### 2.6 ❌ 团队记忆（Team Memory）

**原版实现**：

```
本地文件系统 team/  ──Push──>  团队记忆 API（云端同步）
                    <──Pull──  GitHub repo 识别
```

核心特性：
- 双目录架构：`memory/`（私有）+ `memory/team/`（共享）
- 文件监视器（`fs.watch`）实时监听变更
- 2 秒防抖后推送、错误抑制、恢复机制
- 关闭时优雅刷新

**当前 Go 实现**：
- **完全没有实现**
- 没有 `team/` 子目录支持
- 没有文件监视器
- 没有同步 API 调用

**建议优先级**：🟢 P3（高级协作功能）

**修复难度**：高

---

### 2.7 ❌ Forked Agent 基础设施（关键前置依赖）

**原版实现**：

```typescript
// src/utils/forkedAgent.ts
type CacheSafeParams = {
  systemPrompt: SystemPrompt,
  userContext: Record<string, string>,
  systemContext: Record<string, string>,
  toolUseContext: ToolUseContext,
  forkContextMessages: Message[],
}
```

Forked Agent 是主对话的「影子分身」——共享 Prompt Cache，独立运行，不打断用户交互。

| 系统 | querySource | maxTurns | 用途 |
|------|-------------|----------|------|
| Extract Memories | `extract_memories` | 3 | 自动提取记忆 |
| Auto Dream | `auto_dream` | 10+ | 记忆整合 |
| Session Memory | `session_memory` | 2 | 会话记忆生成 |
| Compact | `compact` | 1 | 上下文压缩 |

**当前 Go 实现**：
```go
// internal/tools/agent/agent.go
func (t *agentTool) Call(...) (*tools.Result, error) {
    // TODO(dep): Implement via Agent-Core SubAgentManager.
    return &tools.Result{
        IsError: true,
        Content: "Agent tool not yet implemented: ...",
    }, nil
}
```

**差距分析**：
- AgentTool 只有接口定义和桩实现
- 没有 `runForkedAgent` 函数
- 没有 `CacheSafeParams` 缓存共享机制
- 没有 `createSubagentContext` 隔离环境
- 没有 `saveCacheSafeParams` / Prompt Cache 共享优化

这是记忆系统高级功能（Extract Memories、Auto Dream、Session Memory、Relevant Memory）的**根本阻塞依赖**。

**建议优先级**：🔴 P0（所有高级功能的前置条件）

**修复难度**：极高（~5-10 天，核心架构组件）

---

### 2.8 ❌ 附件注入系统

**原版实现**：
```typescript
type Attachment =
  | { type: 'nested_memory'; path: string; content: MemoryFileInfo }   // CLAUDE.md 层级
  | { type: 'relevant_memories'; memories: RelevantMemory[] }          // 相关记忆
  | { type: 'current_session_memory'; content: string; path: string }  // 会话记忆

// 去重机制
function filterDuplicateMemoryAttachments(attachments, readFileState)
```

**当前 Go 实现**：
- `CompactionResult.Attachments []types.Message` — 仅结构体定义，未使用
- 没有 `Attachment` 类型定义
- 没有 `<system-reminder>` 标签注入逻辑
- 没有去重机制

**建议优先级**：🟡 P2

**修复难度**：中（~150 行代码）

---

### 2.9 ❌ 记忆时效性管理

**原版实现**：
```typescript
// src/memdir/memoryAge.ts
function memoryFreshnessText(mtimeMs: number): string {
  if (memoryAgeDays(mtimeMs) <= 1) return ''
  return `⚠️ This memory was last updated ${memoryAge(mtimeMs)}. It may be outdated.`
}
```

通过 `<system-reminder>` 标签注入时效性提醒：
```
<system-reminder>
Memory "API Architecture" was saved 12 days ago — verify before relying on it.
</system-reminder>
```

**当前 Go 实现**：
- `MemoryHeader` 包含 `UpdatedAt` 字段，但没有基于它的时效性逻辑
- 没有 `memoryAge` / `memoryFreshnessText` 函数
- 没有时效性提醒注入

**建议优先级**：🟡 P2

**修复难度**：低（~50 行代码）

---

### 2.10 ❌ 快速记忆输入（# 快捷方式）

**原版实现**：
```
用户输入: # 我更喜欢简短的代码注释

→ 渲染为 UserMemoryInputMessage 组件
→ 显示随机确认消息（"Got it." / "Good to know." / "Noted."）
→ 触发记忆保存流程
```

**当前 Go 实现**：
- **完全没有实现**
- TUI 输入层没有 `#` 前缀检测
- 没有 `UserMemoryInputMessage` 组件

**建议优先级**：🟡 P2

**修复难度**：中（~100 行代码，涉及 TUI 层）

---

### 2.11 ❌ 密钥扫描（Secret Scanner）

**原版实现**：
```typescript
// src/services/teamMemorySync/secretScanner.ts
// 覆盖 34 种密钥模式
const SECRET_RULES = [
  { id: 'aws-access-token', source: '\\b(AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16}\\b' },
  { id: 'anthropic-api-key', source: `\\b(${ANT_KEY_PFX}03-...)` },
  { id: 'github-pat', source: 'ghp_[0-9a-zA-Z]{36}' },
  { id: 'private-key', source: '-----BEGIN.*PRIVATE KEY-----' },
  // ... 30+ 种更多模式
]

// 写入守卫 — 在 FileWriteTool/FileEditTool 的 validateInput 中调用
function checkTeamMemSecrets(filePath, content): string | null
```

**当前 Go 实现**：
- **完全没有实现**
- 没有密钥正则模式库
- 没有写入前拦截逻辑
- 没有脱敏函数（`redactSecrets`）

**建议优先级**：🟡 P2（安全功能，与团队记忆配套）

**修复难度**：中（~200 行代码）

---

### 2.12 ❌ StopHooks 触发机制

**原版实现**：
```typescript
// stopHooks.ts — 每次 Turn 结束后
if (!isBareMode()) {
  // 1. 保存 CacheSafeParams（供后续 Fork 使用）
  saveCacheSafeParams(createCacheSafeParams(context))
  // 2. 触发记忆提取（仅主线程）
  if (feature('EXTRACT_MEMORIES') && !agentId) {
    void extractMemoriesModule.executeExtractMemories(context, ...)
  }
  // 3. 触发记忆整合（仅主线程）
  if (!agentId) {
    void executeAutoDream(context, ...)
  }
}
```

**当前 Go 实现**：
- **完全没有实现**
- 没有 Turn 结束后的 Hook 机制
- 没有 `handleStopHooks` 函数

**建议优先级**：🔴 P1（多个后台任务的触发入口）

**修复难度**：中（~100 行代码，但需要 Forked Agent 作为前置）

---

### 2.13 ❌ 记忆启用/禁用门控

**原版实现**：
```typescript
function isAutoMemoryEnabled(): boolean {
  // 1. CLAUDE_CODE_DISABLE_AUTO_MEMORY 环境变量 → 禁用
  // 2. --bare / SIMPLE 模式 → 禁用
  // 3. CCR 无持久存储 → 禁用
  // 4. settings.json 中的 autoMemoryEnabled → 遵循设置
  // 5. 默认 → 启用
}
```

**当前 Go 实现**：
- 没有环境变量控制
- 没有 `--bare` 模式检查
- 没有 `settings.json` 配置读取
- 记忆系统始终隐式启用

**建议优先级**：🟡 P2

**修复难度**：低（~30 行代码）

---

### 2.14 ❌ /memory 交互式编辑器

**原版实现**：
```
> /memory
┌── Memory ──────────────────────────────┐
│  📁 CLAUDE.md files                     │
│    ~/.claude/CLAUDE.md                  │
│    ./CLAUDE.md                          │
│  📁 Auto-memory files                   │
│    ~/.claude/projects/.../memory/       │
│      MEMORY.md                          │
│      user_preferences.md                │
│  Select a file to edit...               │
└─────────────────────────────────────────┘
// 选择文件后，使用 $EDITOR 或 $VISUAL 打开编辑器
```

**当前 Go 实现**：
```go
// internal/commands/builtins.go:269-299
// 仅输出固定描述文本
sb.WriteString("📝 Memory System Status\n\n")
sb.WriteString("## CLAUDE.md Files\n")
sb.WriteString("CLAUDE.md files are auto-discovered from the working directory\n")
sb.WriteString("up to the filesystem root and the home directory.\n\n")
```

**差距分析**：
- 不展示实际发现的文件路径
- 不支持交互式文件选择
- 不支持 `$EDITOR` / `$VISUAL` 打开编辑器

**建议优先级**：🟡 P2

**修复难度**：中（~150 行代码，涉及 TUI 交互组件）

---

### 2.15 ❌ /dream 和 /remember 命令

**原版实现**：
- `/dream` — 手动触发记忆整合（无需等待自动触发条件）
- `/remember` — 高级记忆审计技能：收集所有记忆层 → 分类每个条目 → 识别清理机会 → 生成报告

**当前 Go 实现**：
- `/dream` — 未实现
- `/remember` — 未实现

**建议优先级**：🟢 P3

---

### 2.16 ❌ 记忆矫正提示（Memory Correction Hint）

**原版实现**：
```typescript
const MEMORY_CORRECTION_HINT = 
  "Note: The user's next message may contain a correction or preference. " +
  "Pay close attention — if they explain what went wrong or how they'd prefer " +
  "you to work, consider saving that to memory for future sessions."
```

当用户取消或拒绝 AI 操作时，系统自动注入此提示，引导 AI 关注用户偏好。

**当前 Go 实现**：
- **完全没有实现**

**建议优先级**：🟢 P3

**修复难度**：低（~20 行代码）

---

### 2.17 ❌ Agent Memory（代理记忆）

**原版实现**：
```typescript
// src/tools/AgentTool/agentMemory.ts
// 每种 Agent 类型有独立的记忆目录
function getAgentMemoryDir(agentType: string): string {
  return join(agentMemoryBase, agentType)
}
// 通过 agentMemorySnapshot.ts 实现记忆快照的保存和恢复
```

**当前 Go 实现**：
- **完全没有实现**
- AgentTool 本身还是桩实现

**建议优先级**：🟢 P3

---

### 2.18 ⚠️ MemoryStore 依赖注入不完整

**原版实现**：
- MemoryStore 通过上下文或依赖容器注入

**当前 Go 实现**：
```go
// internal/tools/memory/memory.go:373-386
func getStore(_ *tools.UseContext) *memdir.MemoryStore {
    // Get the working directory from os, then create a MemoryStore.
    // In a real implementation, the MemoryStore would be injected via
    // the UseContext or a dependency container.
    wd, err := os.Getwd()
    if err != nil { return nil }
    store, err := memdir.NewMemoryStore(wd)
    if err != nil { return nil }
    return store
}
```

**差距分析**：
- 每次调用都通过 `os.Getwd()` 重新创建 MemoryStore
- 没有通过 `UseContext` 注入（注释明确说明了 TODO）
- 不支持自定义记忆目录（`settings.autoMemoryDirectory`）
- 不支持 cowork 模式覆盖（`CLAUDE_COWORK_MEMORY_PATH_OVERRIDE`）

**建议优先级**：🟡 P2

**修复难度**：低（~50 行代码）

---

## 三、架构差异总结

| 维度 | 原版 TypeScript | 当前 Go | 完成度 |
|------|----------------|---------|--------|
| **记忆存储层** | 完整（路径 + 读写 + 索引） | 完整 | ✅ 100% |
| **记忆类型系统** | 四类型 + 六层作用域 | 四类型（缺作用域分层） | ⚠️ 70% |
| **CLAUDE.md 加载** | 6 层 + @include + rules/*.md | 仅基础遍历 | ⚠️ 40% |
| **记忆工具（LLM）** | FileWrite/Edit 直接操作 | MemoryRead/Write/Delete | ✅ 90% |
| **记忆提取（Extract）** | Forked Agent 后台自动提取 | 无 | ❌ 0% |
| **记忆整合（Dream）** | 多级门控 + 四阶段整合 | 无 | ❌ 0% |
| **会话记忆（Session）** | Compact 时自动生成 | 无 | ❌ 0% |
| **相关记忆检索** | AI 驱动 + 附件注入 | 无 | ❌ 0% |
| **团队记忆同步** | 文件监视 + API Push/Pull | 无 | ❌ 0% |
| **Forked Agent 基础设施** | 完整（Cache 共享 + 隔离） | 仅桩定义 | ❌ 5% |
| **附件系统** | 三种类型 + 去重 | 仅结构体定义 | ❌ 5% |
| **密钥扫描** | 34 种模式 + 写入守卫 | 无 | ❌ 0% |
| **时效性管理** | freshness 提醒 | 无 | ❌ 0% |
| **# 快速记忆** | TUI 组件 + 确认消息 | 无 | ❌ 0% |
| **StopHooks 触发** | Turn 结束后触发 | 无 | ❌ 0% |
| **/memory 交互式** | 文件选择 + $EDITOR | 固定文本 | ⚠️ 20% |
| **/dream 命令** | 手动触发整合 | 无 | ❌ 0% |
| **启用/禁用门控** | 多级门控 | 无 | ❌ 0% |
| **记忆矫正提示** | 用户拒绝后自动注入 | 无 | ❌ 0% |

**整体评估**：约 **20-25%** 功能覆盖率。基础存储层完整，但所有智能化功能（自动提取、整合、检索、同步）均未实现。

---

## 四、优先改进计划

### 🔴 P0 — 关键前置依赖

| # | 功能 | 工作量估算 | 影响范围 | 说明 |
|---|------|-----------|----------|------|
| 1 | **Forked Agent 基础设施** | 5-8 天 | 全局 | 所有高级功能的前置条件；包含 `runForkedAgent`、`CacheSafeParams`、`createSubagentContext` |

**P0 总工作量**：~5-8 天

### 🔴 P1 — 核心功能

| # | 功能 | 工作量估算 | 前置依赖 |
|---|------|-----------|----------|
| 2 | CLAUDE.md 多层作用域 | 1.5 天 | 无 |
| 3 | StopHooks 触发机制 | 1 天 | 无（但完整效果需 P0） |
| 4 | 记忆提取（Extract Memories） | 3 天 | P0 |
| 5 | 相关记忆检索（Relevant Memory） | 3 天 | P0 + 附件系统 |

**P1 总工作量**：~8.5 天

### 🟡 P2 — 体验完善

| # | 功能 | 工作量估算 |
|---|------|-----------|
| 6 | 附件注入系统 | 1.5 天 |
| 7 | 会话记忆（Session Memory） | 2 天 |
| 8 | 记忆整合（Auto Dream） | 3 天 |
| 9 | 记忆时效性管理 | 0.5 天 |
| 10 | # 快速记忆输入 | 1 天 |
| 11 | /memory 交互式编辑器 | 1 天 |
| 12 | MemoryStore 依赖注入 | 0.5 天 |
| 13 | 记忆启用/禁用门控 | 0.5 天 |
| 14 | 密钥扫描 | 1.5 天 |

**P2 总工作量**：~11.5 天

### 🟢 P3 — 高级功能

| # | 功能 | 工作量估算 |
|---|------|-----------|
| 15 | 团队记忆同步 | 5 天 |
| 16 | /dream 和 /remember 命令 | 2 天 |
| 17 | 记忆矫正提示 | 0.5 天 |
| 18 | Agent Memory（代理记忆） | 2 天 |

**P3 总工作量**：~9.5 天

---

## 五、实现建议

### 5.1 CLAUDE.md 多层作用域（P1）

```go
// 建议修改: internal/memdir/discover.go

// MemoryScope 表示 CLAUDE.md 文件的作用域层级
type MemoryScope int

const (
    ScopeManaged MemoryScope = iota // /etc/claude-code/CLAUDE.md
    ScopeUser                       // ~/.claude/CLAUDE.md
    ScopeProject                    // {project}/CLAUDE.md, .claude/CLAUDE.md, .claude/rules/*.md
    ScopeLocal                      // {project}/CLAUDE.local.md
)

// DiscoveredFile 表示一个发现的记忆文件及其作用域
type DiscoveredFile struct {
    Path  string
    Scope MemoryScope
}

// DiscoverClaudeMdScoped 发现所有 CLAUDE.md 文件并标注作用域
func DiscoverClaudeMdScoped(startDir string) []DiscoveredFile {
    var files []DiscoveredFile
    
    // 1. Managed 层
    if isReadableFile("/etc/claude-code/CLAUDE.md") {
        files = append(files, DiscoveredFile{"/etc/claude-code/CLAUDE.md", ScopeManaged})
    }
    
    // 2. User 层
    home, _ := os.UserHomeDir()
    userFile := filepath.Join(home, ".claude", "CLAUDE.md")
    if isReadableFile(userFile) {
        files = append(files, DiscoveredFile{userFile, ScopeUser})
    }
    
    // 3. Project 层 — 遍历到 git root
    projectDir := findGitRoot(startDir)
    for _, candidate := range []string{
        filepath.Join(projectDir, "CLAUDE.md"),
        filepath.Join(projectDir, ".claude", "CLAUDE.md"),
    } {
        if isReadableFile(candidate) {
            files = append(files, DiscoveredFile{candidate, ScopeProject})
        }
    }
    // .claude/rules/*.md
    rulesDir := filepath.Join(projectDir, ".claude", "rules")
    if entries, err := os.ReadDir(rulesDir); err == nil {
        for _, e := range entries {
            if strings.HasSuffix(e.Name(), ".md") {
                files = append(files, DiscoveredFile{
                    filepath.Join(rulesDir, e.Name()), ScopeProject,
                })
            }
        }
    }
    
    // 4. Local 层
    localFile := filepath.Join(projectDir, "CLAUDE.local.md")
    if isReadableFile(localFile) {
        files = append(files, DiscoveredFile{localFile, ScopeLocal})
    }
    
    return files
}
```

### 5.2 Forked Agent 基础设施（P0）

```go
// 建议位置: internal/engine/forked_agent.go

// CacheSafeParams 存储可供 Fork 共享的 Prompt Cache 参数
type CacheSafeParams struct {
    SystemPrompt       SystemPrompt
    ContextMessages    []types.Message  // 对话历史前缀（不变部分）
    ToolUseContext     *tools.UseContext
}

// ForkedAgentConfig 配置一个后台 Fork Agent
type ForkedAgentConfig struct {
    PromptMessages     []types.Message  // Fork 的额外消息
    CacheSafeParams    *CacheSafeParams // 共享的缓存参数
    QuerySource        string           // 来源标识
    MaxTurns           int              // 最大轮数
    AllowedTools       []string         // 允许的工具列表
    OnMessage          func(types.Message) // 消息回调
}

// RunForkedAgent 运行一个隔离的后台 Agent
func RunForkedAgent(ctx context.Context, cfg ForkedAgentConfig) error {
    // 1. 创建隔离的上下文（克隆 readFileState，独立 AbortController）
    // 2. 复用 CacheSafeParams 命中 Prompt Cache
    // 3. 执行 maxTurns 轮对话
    // 4. 结果通过 OnMessage 回调返回
}
```

### 5.3 记忆时效性管理（P2 — 快速实现）

```go
// 建议位置: internal/memdir/freshness.go

// MemoryAge 返回记忆文件的年龄描述
func MemoryAge(updatedAt time.Time) string {
    days := int(time.Since(updatedAt).Hours() / 24)
    switch {
    case days == 0:
        return "today"
    case days == 1:
        return "yesterday"
    default:
        return fmt.Sprintf("%d days ago", days)
    }
}

// MemoryFreshnessText 返回时效性警告文本
func MemoryFreshnessText(updatedAt time.Time) string {
    days := int(time.Since(updatedAt).Hours() / 24)
    if days <= 1 {
        return ""
    }
    return fmt.Sprintf("⚠️ This memory was last updated %s. It may be outdated.", MemoryAge(updatedAt))
}
```

### 5.4 StopHooks 触发框架（P1）

```go
// 建议位置: internal/engine/stop_hooks.go

// StopHookContext 提供 Turn 结束后的上下文
type StopHookContext struct {
    Messages       []types.Message
    ToolUseContext *tools.UseContext
    QuerySource    string
    IsBareMode     bool
}

// HandleStopHooks 在每次 Turn 结束后执行
func HandleStopHooks(ctx context.Context, hookCtx StopHookContext) {
    if hookCtx.IsBareMode {
        return
    }
    
    // 1. 保存 CacheSafeParams
    saveCacheSafeParams(createCacheSafeParams(hookCtx))
    
    // 2. 触发记忆提取（后台，不阻塞）
    if isExtractMemoriesEnabled() && hookCtx.QuerySource == "repl_main_thread" {
        go executeExtractMemories(ctx, hookCtx)
    }
    
    // 3. 触发记忆整合（后台，不阻塞）
    if hookCtx.QuerySource == "repl_main_thread" {
        go executeAutoDream(ctx, hookCtx)
    }
}
```

---

## 六、代码质量问题

### 6.1 MemoryStore 每次创建新实例

```go
// internal/tools/memory/memory.go:373-386
func getStore(_ *tools.UseContext) *memdir.MemoryStore {
    wd, err := os.Getwd()   // ❌ 每次调用都获取 wd
    if err != nil { return nil }
    store, err := memdir.NewMemoryStore(wd)  // ❌ 每次创建新实例
    if err != nil { return nil }
    return store
}
```

**建议**：通过 `UseContext` 或 `AppContainer` 注入单例 MemoryStore。

### 6.2 /memory 命令不展示实际状态

```go
// internal/commands/builtins.go:274-285
sb.WriteString("CLAUDE.md files are auto-discovered from the working directory\n")
// ❌ 仅输出固定文本，不展示实际发现的文件路径和记忆列表
```

**建议**：接入 `DiscoverClaudeMd` 和 `MemoryStore.ListMemories`，展示实际文件路径和记忆数量。

### 6.3 CLAUDE.md 加载仅在 Init 时执行一次

```go
// internal/tui/update.go:34-37
case MemdirLoadedMsg:
    m.memdirPaths = msg.Paths
    m.memdirPrompt = memdir.LoadMemoryPrompt(msg.Paths)
    return m, nil
```

**建议**：TS 原版使用缓存 + 失效机制（Compact 时重置缓存），考虑在 Compact 后重新加载，或使用 `fsnotify` 监视文件变更。

### 6.4 MEMORY.md 索引未注入系统提示

**当前流程**：
```
TUI Init() → loadMemdirCmd → DiscoverClaudeMd → LoadMemoryPrompt → 注入 SystemPrompt
```

**问题**：`LoadMemoryPrompt` 只加载 CLAUDE.md 文件，不加载 MEMORY.md 索引。虽然 `LoadAllMemory` 函数支持联合加载，但 TUI 层并未调用它。

**建议**：在 `MemdirLoadedMsg` 处理中改用 `LoadAllMemory`，同时传入 MemoryStore。

---

## 关键依赖关系图

```
                    Forked Agent 基础设施 (P0)
                    /          |          \
                   /           |           \
    Extract Memories (P1)  Auto Dream (P2)  Session Memory (P2)
           |                   |                |
           v                   v                v
    StopHooks 触发 (P1) ←─────────────────────────
           |
           v
    附件注入系统 (P2)
           |
           v
    相关记忆检索 (P1)
```

**实现顺序建议**：
1. `P0` Forked Agent 基础设施
2. `P1` CLAUDE.md 多层作用域（独立，无前置依赖）
3. `P1` StopHooks 触发框架
4. `P1` 记忆提取（Extract Memories）
5. `P2` 附件注入系统
6. `P1` 相关记忆检索
7. `P2` 其余功能按优先级实现

---

## 相关 Issue

完成本文档后，建议创建以下 GitHub Issues：

1. **[Epic] 记忆系统完善** — 跟踪整体进度
2. **[P0] 实现 Forked Agent 基础设施** — 关键前置依赖
3. **[P1] 实现 CLAUDE.md 多层作用域** — 核心功能差距
4. **[P1] 实现 StopHooks 触发框架** — 后台任务入口
5. **[P1] 实现记忆自动提取（Extract Memories）** — 核心智能功能
6. **[P1] 实现相关记忆检索（Relevant Memory）** — 记忆利用率
7. **[P2] 实现附件注入系统** — 记忆注入基础
8. **[P2] 实现 Auto Dream 记忆整合** — 记忆质量维护

---

## 变更历史

| 日期 | 版本 | 变更内容 |
|------|------|----------|
| 2026-04-23 | v1.0 | 初始版本，完成差异分析（覆盖 18 个功能点） |
