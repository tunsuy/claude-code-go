# Claude Code Skill 系统完整技术文档

> 基于源码分析，版本对应 Claude Code 主仓库（2026 年 4 月）

---

## 目录

1. [概述](#1-概述)
2. [核心概念：Skill 与 Command 的关系](#2-核心概念skill-与-command-的关系)
3. [源码文件索引](#3-源码文件索引)
4. [类型系统](#4-类型系统)
5. [Skill 定义格式（SKILL.md）](#5-skill-定义格式skillmd)
6. [Skill 来源体系（7 种来源）](#6-skill-来源体系7-种来源)
7. [Skill 加载管线（loadSkillsDir.ts）](#7-skill-加载管线loadskillsdirts)
8. [SkillTool —— LLM 调用入口](#8-skilltool--llm-调用入口)
9. [执行模式](#9-执行模式)
10. [权限安全模型](#10-权限安全模型)
11. [动态发现与条件激活](#11-动态发现与条件激活)
12. [Bundled Skills 注册机制](#12-bundled-skills-注册机制)
13. [内置 Skill 清单](#13-内置-skill-清单)
14. [MCP Skill 桥接](#14-mcp-skill-桥接)
15. [Skillify —— 交互式 Skill 创建向导](#15-skillify--交互式-skill-创建向导)
16. [Prompt 生成与预算管理](#16-prompt-生成与预算管理)
17. [Skill 使用追踪](#17-skill-使用追踪)
18. [遥测与分析](#18-遥测与分析)
19. [完整生命周期流程图](#19-完整生命周期流程图)
20. [扩展指南：如何添加新 Skill](#20-扩展指南如何添加新-skill)

---

## 1. 概述

Claude Code 的 Skill 系统是一套**基于自然语言 Prompt 的声明式命令框架**。与硬编码的 JS 函数命令不同，Skill 使用 Markdown 文件（`SKILL.md`）定义行为逻辑，由 LLM 在运行时解释执行。

**核心特性：**
- 7 种来源自动发现注册（bundled、user、project、managed、plugin、MCP、dynamic）
- 两种执行模式（inline 内联展开 / fork 子代理隔离）
- 3 层权限安全检查（deny → allow → auto-allow → ask）
- 运行时动态发现 + 条件激活
- 上下文窗口预算感知的 Prompt 生成

---

## 2. 核心概念：Skill 与 Command 的关系

```
Command（联合类型 — 所有命令的顶层抽象）
  │
  ├─ PromptCommand（type: 'prompt'）  ← 【这就是 Skill】
  │   ├─ Bundled Skills      — 编译时打包（review, init, simplify…）
  │   ├─ User Skills         — ~/.claude/skills/*/SKILL.md
  │   ├─ Project Skills      — .claude/skills/*/SKILL.md
  │   ├─ Managed Skills      — Policy 管控的 skills
  │   ├─ Plugin Skills       — 插件导出（source: 'plugin'）
  │   ├─ MCP Skills          — MCP Server 提供（loadedFrom: 'mcp'）
  │   └─ Legacy Commands     — .commands/*.md（已废弃）
  │
  ├─ LocalCommand（type: 'local'） — JS 函数命令
  │
  └─ LocalJSXCommand（type: 'local-jsx'） — React UI 命令
```

**关键区分：**
- **Skills** = 用户/开发者在磁盘上定义的 `PromptCommand`
- **Commands** = 运行时的统一抽象（包含 Skills + 其他类型命令）
- **Plugins** = 用于分发命令的打包系统（其导出的命令包装为 PromptCommand）

---

## 3. 源码文件索引

### 核心文件

| 文件路径 | 行数 | 职责 |
|---------|------|------|
| `src/tools/SkillTool/SkillTool.ts` | 1,109 | LLM 调用 Skill 的工具入口、权限检查、执行调度 |
| `src/skills/loadSkillsDir.ts` | 1,087 | 从磁盘发现、解析、加载 SKILL.md 文件 |
| `src/skills/bundledSkills.ts` | 221 | 内置 Skill 注册表、文件提取、安全写入 |
| `src/skills/mcpSkillBuilders.ts` | 45 | MCP Skill 构建器注册（打破循环依赖） |
| `src/tools/SkillTool/prompt.ts` | 242 | Skill 列表 Prompt 生成与预算管理 |
| `src/tools/SkillTool/constants.ts` | - | SKILL_TOOL_NAME 常量 |
| `src/tools/SkillTool/UI.tsx` | - | Skill 工具的 React 渲染组件 |
| `src/types/command.ts` | 217 | Command/PromptCommand/CommandBase 类型定义 |

### Bundled Skill 文件

| 文件路径 | Skill 名称 | 是否特性门控 |
|---------|-----------|------------|
| `src/skills/bundled/index.ts` | 初始化入口 | - |
| `src/skills/bundled/updateConfig.ts` | update-config | 否 |
| `src/skills/bundled/keybindings.ts` | keybindings-help | 否 |
| `src/skills/bundled/verify.ts` | verify | 否 |
| `src/skills/bundled/debug.ts` | debug | 否 |
| `src/skills/bundled/loremIpsum.ts` | lorem-ipsum | 否 |
| `src/skills/bundled/skillify.ts` | skillify | 否（但限 ant 用户） |
| `src/skills/bundled/remember.ts` | remember | 否 |
| `src/skills/bundled/simplify.ts` | simplify | 否 |
| `src/skills/bundled/batch.ts` | batch | 否 |
| `src/skills/bundled/stuck.ts` | stuck | 否 |
| `src/skills/bundled/loop.ts` | loop | `AGENT_TRIGGERS` |
| `src/skills/bundled/scheduleRemoteAgents.ts` | schedule-remote-agents | `AGENT_TRIGGERS_REMOTE` |
| `src/skills/bundled/claudeApi.ts` | claude-api | `BUILDING_CLAUDE_APPS` |
| `src/skills/bundled/claudeInChrome.ts` | claude-in-chrome | `shouldAutoEnableClaudeInChrome()` |
| `src/skills/bundled/dream.ts` | dream | `KAIROS` / `KAIROS_DREAM` |
| `src/skills/bundled/hunter.ts` | hunter | `REVIEW_ARTIFACT` |
| `src/skills/bundled/runSkillGenerator.ts` | run-skill-generator | `RUN_SKILL_GENERATOR` |

### 辅助文件

| 文件路径 | 职责 |
|---------|------|
| `src/hooks/useSkillsChange.ts` | 文件变更监听，触发 Skill 热加载 |
| `src/utils/hooks/registerSkillHooks.ts` | Skill 调用时的 Hook 注册 |
| `src/utils/suggestions/skillUsageTracking.ts` | Skill 使用频率追踪 |

---

## 4. 类型系统

### PromptCommand（Skill 的运行时表示）

```typescript
// src/types/command.ts
type PromptCommand = {
  type: 'prompt'
  progressMessage: string              // 进度提示，通常为 'running'
  contentLength: number                // Markdown 内容长度（用于 Token 估算）
  argNames?: string[]                  // 参数名列表
  allowedTools?: string[]              // 允许使用的工具列表
  model?: string                       // 模型覆盖
  source: SettingSource | 'builtin' | 'mcp' | 'plugin' | 'bundled'
  pluginInfo?: {                       // 插件来源信息
    pluginManifest: PluginManifest
    repository: string
  }
  disableNonInteractive?: boolean
  hooks?: HooksSettings                // 调用时注册的 Hooks
  skillRoot?: string                   // Skill 资源的基目录
  context?: 'inline' | 'fork'         // 执行模式
  agent?: string                       // fork 模式下的 Agent 类型
  effort?: EffortValue                 // 推理努力级别覆盖
  paths?: string[]                     // 条件激活的 glob 模式
  getPromptForCommand(                 // 运行时获取完整 Prompt
    args: string,
    context: ToolUseContext,
  ): Promise<ContentBlockParam[]>
}
```

### CommandBase（所有命令共享的基础属性）

```typescript
// src/types/command.ts
type CommandBase = {
  availability?: CommandAvailability[] // 可用环境（'claude-ai' | 'console'）
  description: string
  hasUserSpecifiedDescription?: boolean
  isEnabled?: () => boolean            // 特性门控
  isHidden?: boolean                   // 是否隐藏
  name: string
  aliases?: string[]                   // 别名
  isMcp?: boolean
  argumentHint?: string                // 参数提示文本
  whenToUse?: string                   // 详细使用场景
  version?: string
  disableModelInvocation?: boolean     // 禁止 LLM 自动调用
  userInvocable?: boolean              // 用户可否手动调用
  loadedFrom?: LoadedFrom              // 加载来源
  kind?: 'workflow'                    // 工作流标记
  immediate?: boolean                  // 立即执行
  isSensitive?: boolean                // 参数脱敏
  userFacingName?: () => string        // 显示名覆盖
}

// 来源类型
type LoadedFrom =
  | 'commands_DEPRECATED'
  | 'skills'
  | 'plugin'
  | 'managed'
  | 'bundled'
  | 'mcp'
```

### BundledSkillDefinition（内置 Skill 定义）

```typescript
// src/skills/bundledSkills.ts
type BundledSkillDefinition = {
  name: string
  description: string
  aliases?: string[]
  whenToUse?: string
  argumentHint?: string
  allowedTools?: string[]
  model?: string
  disableModelInvocation?: boolean
  userInvocable?: boolean
  isEnabled?: () => boolean
  hooks?: HooksSettings
  context?: 'inline' | 'fork'
  agent?: string
  files?: Record<string, string>        // 附带的参考文件
  getPromptForCommand: (
    args: string,
    context: ToolUseContext,
  ) => Promise<ContentBlockParam[]>
}
```

### SkillTool I/O Schema

```typescript
// src/tools/SkillTool/SkillTool.ts

// 输入
type InputSchema = {
  skill: string     // Skill 名称，如 "commit", "review-pr"
  args?: string     // 可选参数
}

// 输出（联合类型）
type Output =
  | {  // Inline 执行结果
      success: boolean
      commandName: string
      allowedTools?: string[]
      model?: string
      status?: 'inline'
    }
  | {  // Fork 执行结果
      success: boolean
      commandName: string
      status: 'forked'
      agentId: string
      result: string
    }
```

---

## 5. Skill 定义格式（SKILL.md）

### 文件结构

```
.claude/skills/
  └── my-skill/
      └── SKILL.md          ← 唯一入口文件
```

### SKILL.md 模板

```markdown
---
name: My Skill                    # 显示名称
description: 一行描述              # Skill 描述
allowed-tools:                     # 允许使用的工具
  - Read
  - Write
  - Bash(gh:*)                     # 支持通配符模式
model: sonnet                      # 模型覆盖（可选）
effort: high                       # 推理努力级别（可选）
context: fork                      # 执行模式：fork 或 inline（默认）
agent: general-purpose             # fork 模式下的 Agent 类型
when_to_use: |                     # 详细使用场景（影响 LLM 自动调用）
  Use when the user wants to review pull requests.
  Examples: "review this PR", "code review"
argument-hint: "[PR number]"       # 参数提示
arguments:                         # 参数定义
  - pr_number
  - branch
user-invocable: true               # 用户可否手动调用（默认 true）
disable-model-invocation: false    # 禁止 LLM 自动调用（默认 false）
version: "1.0.0"                   # 版本号
paths:                             # 条件激活路径（glob 模式）
  - "src/**/*.ts"
  - "tests/**"
hooks:                             # Skill 级 Hooks
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: "echo 'pre-check'"
shell:                             # Shell 执行配置
  command: "echo $VARIABLE"
  cwd: "${CLAUDE_SKILL_DIR}"
---

# Skill 标题

详细的 Skill 指令内容...

## 输入
- `$pr_number`: PR 编号
- `$branch`: 目标分支

## 步骤

### 1. 获取 PR 信息
!`gh pr view $pr_number --json title,body`

使用上面命令获取 PR 信息...

### 2. 审查代码
阅读变更的文件并提供反馈...

**成功标准**: 审查完成并输出评审意见
```

### Frontmatter 字段完整说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | string | 目录名 | 显示名称 |
| `description` | string | 从 Markdown 提取 | 一行描述 |
| `allowed-tools` | string[] | `[]` | 允许使用的工具模式列表 |
| `model` | string | 继承 | 模型覆盖（`inherit` = 使用父级） |
| `effort` | string/number | 继承 | 推理努力级别 |
| `context` | `'inline'` \| `'fork'` | `'inline'` | 执行模式 |
| `agent` | string | - | fork 模式的 Agent 类型 |
| `when_to_use` | string | - | 详细使用场景（重要：影响 LLM 自动调用） |
| `argument-hint` | string | - | 参数提示文本 |
| `arguments` | string[] | - | 参数名列表 |
| `user-invocable` | boolean | `true` | 用户可否通过 `/name` 调用 |
| `disable-model-invocation` | boolean | `false` | 禁止 LLM 自动调用 |
| `version` | string | - | 版本号 |
| `paths` | string[] | - | 条件激活的 glob 模式 |
| `hooks` | object | - | Skill 级 Hooks 定义 |
| `shell` | object | - | Shell 命令执行配置 |

### 内置变量替换

| 变量 | 说明 |
|------|------|
| `$ARGUMENTS` | 用户传入的原始参数 |
| `$arg_name` | 具名参数替换（通过 `arguments` 定义） |
| `${CLAUDE_SKILL_DIR}` | Skill 文件所在目录的绝对路径 |
| `${CLAUDE_SESSION_ID}` | 当前会话 ID |

### 内联 Shell 执行

```markdown
<!-- 使用 ! 前缀执行 Shell 命令，结果内联到 Prompt 中 -->
!`gh pr view 123 --json title`

<!-- 代码块形式 -->
```! sh
gh pr list --json number,title
`` `
```

> **安全限制**：MCP 来源的 Skill 禁止执行内联 Shell 命令。

---

## 6. Skill 来源体系（7 种来源）

### 加载优先级（从高到低）

```
┌─────────────────────────────────────────────────────┐
│  1. Managed Skills     — 策略管控目录                │
│     路径: $MANAGED_PATH/.claude/skills/              │
│     来源: policySettings                             │
│     控制: CLAUDE_CODE_DISABLE_POLICY_SKILLS 环境变量 │
├─────────────────────────────────────────────────────┤
│  2. User Skills        — 用户全局目录                │
│     路径: ~/.claude/skills/                          │
│     来源: userSettings                               │
│     控制: isSettingSourceEnabled('userSettings')     │
├─────────────────────────────────────────────────────┤
│  3. Project Skills     — 项目目录（向上遍历至 HOME） │
│     路径: .claude/skills/ (从 cwd 向上到 ~/)         │
│     来源: projectSettings                            │
│     控制: isSettingSourceEnabled('projectSettings')  │
├─────────────────────────────────────────────────────┤
│  4. Additional Skills  — --add-dir 指定的附加目录    │
│     路径: <dir>/.claude/skills/                      │
│     来源: projectSettings                            │
├─────────────────────────────────────────────────────┤
│  5. Legacy Commands    — 废弃的 .commands/ 目录      │
│     路径: .commands/*.md                             │
│     来源: commands_DEPRECATED                        │
│     格式: 支持 SKILL.md + 单 .md 文件               │
├─────────────────────────────────────────────────────┤
│  6. Bundled Skills     — 编译时内置                  │
│     注册: registerBundledSkill()                     │
│     来源: bundled                                    │
├─────────────────────────────────────────────────────┤
│  7. MCP Skills         — MCP Server 远程提供         │
│     注册: MCP 连接后动态加入                         │
│     来源: mcp                                        │
│     限制: 不能执行内联 Shell                         │
└─────────────────────────────────────────────────────┘
```

### 去重规则

当同一个 SKILL.md 文件被多条路径发现时（例如通过符号链接），使用 `realpath()` 解析后去重，**先发现者优先**。

```typescript
// loadSkillsDir.ts:725-763
// 并行预计算所有文件的 realpath
const fileIds = await Promise.all(
  allSkillsWithPaths.map(({ skill, filePath }) =>
    skill.type === 'prompt' ? getFileIdentity(filePath) : Promise.resolve(null),
  ),
)
// 同步去重：先入者保留
const seenFileIds = new Map<string, source>()
for (/* each entry */) {
  const existingSource = seenFileIds.get(fileId)
  if (existingSource !== undefined) continue  // 跳过重复
  seenFileIds.set(fileId, skill.source)
  deduplicatedSkills.push(skill)
}
```

### --bare 模式

在 `--bare` 模式下，跳过自动发现（managed/user/project/legacy），仅加载 `--add-dir` 显式指定的目录中的 Skills。

---

## 7. Skill 加载管线（loadSkillsDir.ts）

### 主函数：`getSkillDirCommands(cwd)`

这是一个 **memoized 异步函数**，在启动时执行一次。

```
getSkillDirCommands(cwd)
  │
  ├─ 1. 计算搜索路径
  │    ├─ managed: getManagedFilePath()/.claude/skills
  │    ├─ user: ~/.claude/skills
  │    ├─ project: getProjectDirsUpToHome('skills', cwd)
  │    └─ additional: getAdditionalDirectoriesForClaudeMd()
  │
  ├─ 2. 检查策略与设置
  │    ├─ skillsLocked = isRestrictedToPluginOnly('skills')
  │    ├─ projectSettingsEnabled = 项目设置是否启用 && !locked
  │    └─ --bare 模式：仅加载 --add-dir
  │
  ├─ 3. 并行加载（5 路并行）
  │    ├─ loadSkillsFromSkillsDir(managedDir, 'policySettings')
  │    ├─ loadSkillsFromSkillsDir(userDir, 'userSettings')
  │    ├─ loadSkillsFromSkillsDir(projectDirs[], 'projectSettings')
  │    ├─ loadSkillsFromSkillsDir(additionalDirs[], 'projectSettings')
  │    └─ loadSkillsFromCommandsDir(cwd)  ← Legacy
  │
  ├─ 4. 合并 & 去重
  │    ├─ 按 realpath 去重（处理符号链接）
  │    └─ 先入者优先
  │
  ├─ 5. 分离条件 Skill
  │    ├─ 有 paths 且未激活 → conditionalSkills Map（休眠）
  │    └─ 无 paths 或已激活 → unconditionalSkills[]（返回）
  │
  └─ 6. 返回无条件 Skills
```

### 单目录加载：`loadSkillsFromSkillsDir(basePath, source)`

```
loadSkillsFromSkillsDir(basePath, source)
  │
  ├─ readdir(basePath)
  │
  ├─ 遍历每个子目录
  │    ├─ 跳过非目录/非符号链接（/skills/ 只支持目录格式）
  │    ├─ 读取 <dir>/SKILL.md
  │    ├─ parseFrontmatter() → 提取 YAML 前言 + Markdown 内容
  │    ├─ parseSkillPaths() → 解析 paths 前言
  │    ├─ parseSkillFrontmatterFields() → 解析所有前言字段
  │    └─ createSkillCommand() → 构建 Command 对象
  │
  └─ 返回 SkillWithPath[]
```

### Frontmatter 解析：`parseSkillFrontmatterFields()`

```typescript
// 返回结构
{
  displayName,               // frontmatter.name → 显示名
  description,               // 用户指定 > Markdown 提取 > 回退标签
  hasUserSpecifiedDescription,
  allowedTools,              // parseSlashCommandToolsFromFrontmatter()
  argumentHint,              // frontmatter['argument-hint']
  argumentNames,             // parseArgumentNames(frontmatter.arguments)
  whenToUse,                 // frontmatter.when_to_use
  version,                   // frontmatter.version
  model,                     // parseUserSpecifiedModel()，'inherit' → undefined
  disableModelInvocation,    // parseBooleanFrontmatter()
  userInvocable,             // 默认 true
  hooks,                     // parseHooksFromFrontmatter() → HooksSchema 验证
  executionContext,           // 'fork' | undefined
  agent,                     // frontmatter.agent
  effort,                    // parseEffortValue()
  shell,                     // parseShellFrontmatter()
}
```

### Skill 命令创建：`createSkillCommand()`

`createSkillCommand()` 是工厂函数，构建最终的 `Command` 对象。关键在于 `getPromptForCommand()` 方法的实现：

```typescript
async getPromptForCommand(args, toolUseContext) {
  // 1. 如有 baseDir，添加基目录头
  let finalContent = baseDir
    ? `Base directory for this skill: ${baseDir}\n\n${markdownContent}`
    : markdownContent

  // 2. $ARGUMENTS 和具名参数替换
  finalContent = substituteArguments(finalContent, args, true, argumentNames)

  // 3. ${CLAUDE_SKILL_DIR} → Skill 目录路径
  if (baseDir) {
    finalContent = finalContent.replace(/\$\{CLAUDE_SKILL_DIR\}/g, skillDir)
  }

  // 4. ${CLAUDE_SESSION_ID} → 会话 ID
  finalContent = finalContent.replace(/\$\{CLAUDE_SESSION_ID\}/g, getSessionId())

  // 5. Shell 命令执行（MCP Skill 禁止！）
  if (loadedFrom !== 'mcp') {
    finalContent = await executeShellCommandsInPrompt(finalContent, ...)
  }

  return [{ type: 'text', text: finalContent }]
}
```

---

## 8. SkillTool —— LLM 调用入口

`SkillTool` 是一个标准的 `Tool` 实现（`src/tools/SkillTool/SkillTool.ts`），是 LLM 调用 Skill 的唯一入口。

### 输入验证：`validateInput()`

```
validateInput({ skill }, context)
  │
  ├─ 空值检查 → errorCode: 1
  ├─ 去除前导 / 斜杠（兼容处理）
  ├─ Remote Canonical Skill 检查（实验性，ant-only）
  │    └─ _canonical_<slug> 前缀 → 检查是否已发现
  ├─ 获取所有命令（本地 + MCP）
  ├─ 查找命令 → 未找到则 errorCode: 2
  ├─ disableModelInvocation 检查 → errorCode: 4
  └─ type !== 'prompt' 检查 → errorCode: 5
```

### 权限检查：`checkPermissions()`

详见 [第 10 节](#10-权限安全模型)。

### 主执行逻辑：`call()`

```
call({ skill, args }, context, canUseTool, parentMessage, onProgress)
  │
  ├─ 1. 规范化 Skill 名称（去除前导 /）
  │
  ├─ 2. Remote Canonical Skill?
  │    └─ 是 → executeRemoteSkill() → 返回
  │
  ├─ 3. 获取命令 & 记录使用
  │    ├─ getAllCommands() → 本地 + MCP Skills
  │    ├─ findCommand(commandName)
  │    └─ recordSkillUsage(commandName)
  │
  ├─ 4. Fork 模式?
  │    └─ command.context === 'fork' → executeForkedSkill() → 返回
  │
  ├─ 5. Inline 执行（默认）
  │    ├─ processPromptSlashCommand() → 展开 Prompt
  │    │    ├─ !command 替换
  │    │    ├─ $ARGUMENTS 插值
  │    │    └─ Frontmatter 解析
  │    ├─ 提取 metadata: allowedTools, model, effort
  │    ├─ 遥测日志
  │    └─ 标记消息: tagMessagesWithToolUseID()
  │
  └─ 6. 返回结果
       ├─ data: { success, commandName, allowedTools, model }
       ├─ newMessages: 展开的 Skill 内容
       └─ contextModifier: (ctx) => {
            ├─ 注入 allowedTools 到权限规则
            ├─ 应用 model 覆盖（保持 [1m] 后缀）
            └─ 应用 effort 覆盖
          }
```

### getAllCommands() —— 命令聚合

```typescript
// SkillTool 需要同时获取本地和 MCP Skills
async function getAllCommands(context: ToolUseContext): Promise<Command[]> {
  // 仅包含 MCP Skills（loadedFrom === 'mcp'），排除原始 MCP Prompts
  const mcpSkills = context.getAppState().mcp.commands.filter(
    cmd => cmd.type === 'prompt' && cmd.loadedFrom === 'mcp',
  )
  if (mcpSkills.length === 0) return getCommands(getProjectRoot())
  const localCommands = await getCommands(getProjectRoot())
  return uniqBy([...localCommands, ...mcpSkills], 'name')
}
```

---

## 9. 执行模式

### Inline 模式（默认）

Skill 的 Prompt 内容**展开到当前对话上下文**中，LLM 在同一会话中继续执行。

```
用户消息 → LLM 调用 SkillTool → Skill Prompt 展开为 newMessages
                                  ↓
                             LLM 在同一上下文中处理
                                  ↓
                             继续对话
```

**适用场景**：
- 需要访问当前对话历史
- 用户需要中途介入/引导
- Skill 需要在当前上下文中修改工具权限

**contextModifier 机制**：
- `allowedTools` → 注入到 `alwaysAllowRules.command`，扩展工具权限
- `model` → 通过 `resolveSkillModelOverride()` 覆盖模型（保持 `[1m]` 后缀避免窗口降级）
- `effort` → 覆盖 `effortValue`

### Fork 模式

Skill 在**隔离的子 Agent** 中运行，拥有独立的 Token 预算。

```
用户消息 → LLM 调用 SkillTool → executeForkedSkill()
                                  ↓
                             prepareForkedCommandContext()
                                  ↓
                             runAgent({
                               agentDefinition,
                               promptMessages,    ← Skill Prompt
                               toolUseContext,
                               model: command.model,
                               availableTools,
                             })
                                  ↓
                             子 Agent 独立执行
                                  ↓
                             extractResultText() → 返回结果摘要
```

**适用场景**：
- 自包含任务，不需要对话上下文
- 不需要用户中途介入
- 需要隔离 Token 预算

```typescript
// Fork 执行核心逻辑
async function executeForkedSkill(command, commandName, args, context, ...) {
  const agentId = createAgentId()

  // 准备隔离上下文
  const { modifiedGetAppState, baseAgent, promptMessages, skillContent } =
    await prepareForkedCommandContext(command, args || '', context)

  // 合并 effort 到 Agent 定义
  const agentDefinition = command.effort !== undefined
    ? { ...baseAgent, effort: command.effort }
    : baseAgent

  // 运行子 Agent
  for await (const message of runAgent({ agentDefinition, promptMessages, ... })) {
    agentMessages.push(message)
    // 报告进度（与 AgentTool 相同）
  }

  // 提取结果
  const resultText = extractResultText(agentMessages, 'Skill execution completed')
  agentMessages.length = 0  // 释放内存
  clearInvokedSkillsForAgent(agentId)

  return { data: { success: true, commandName, status: 'forked', agentId, result: resultText } }
}
```

### Remote 模式（实验性，ant-only）

从远程（AKI/GCS）加载 Canonical Skill，注入到会话中。

```
_canonical_<slug> → loadRemoteSkill(slug, url) → 缓存 + 注入

特点：
- 声明式 Markdown，不进行 !command / $ARGUMENTS 展开
- 自动缓存
- 注册到 addInvokedSkill 以在压缩时保留
```

---

## 10. 权限安全模型

### 3 层权限检查流程

```
checkPermissions({ skill, args }, context)
  │
  ├─ 1. 规范化名称（去除前导 /）
  │
  ├─ 2. 【Deny 规则】
  │    ├─ 获取所有 deny 规则
  │    ├─ ruleMatches()：精确匹配 + 前缀通配
  │    │    ├─ 精确: "review-pr" === "review-pr"
  │    │    └─ 前缀: "review:*" → commandName.startsWith("review")
  │    └─ 匹配 → return { behavior: 'deny' }
  │
  ├─ 3. 【Remote Canonical Auto-Allow】（实验性）
  │    └─ _canonical_ 前缀 → return { behavior: 'allow' }
  │
  ├─ 4. 【Allow 规则】
  │    ├─ 获取所有 allow 规则
  │    ├─ 相同 ruleMatches() 逻辑
  │    └─ 匹配 → return { behavior: 'allow' }
  │
  ├─ 5. 【Auto-Allow 安全属性检查】
  │    ├─ skillHasOnlySafeProperties(command) ?
  │    │    ├─ 遍历 command 所有属性键
  │    │    ├─ 在 SAFE_SKILL_PROPERTIES 白名单中? → 安全
  │    │    ├─ 值为 null/undefined/空数组/空对象? → 安全
  │    │    └─ 否则 → 不安全
  │    └─ 全部安全 → return { behavior: 'allow' }
  │
  └─ 6. 【Ask 用户】
       ├─ 生成两个建议规则：
       │    ├─ 精确规则: Skill(review-pr)
       │    └─ 前缀规则: Skill(review-pr:*)
       └─ return { behavior: 'ask', suggestions }
```

### SAFE_SKILL_PROPERTIES 白名单

这是一个**允许列表**——新增的属性默认需要权限审批，直到被显式添加到白名单。

```typescript
const SAFE_SKILL_PROPERTIES = new Set([
  // PromptCommand 属性
  'type', 'progressMessage', 'contentLength', 'argNames',
  'model', 'effort', 'source', 'pluginInfo',
  'disableNonInteractive', 'skillRoot', 'context', 'agent',
  'getPromptForCommand', 'frontmatterKeys',
  // CommandBase 属性
  'name', 'description', 'hasUserSpecifiedDescription',
  'isEnabled', 'isHidden', 'aliases', 'isMcp', 'argumentHint',
  'whenToUse', 'paths', 'version', 'disableModelInvocation',
  'userInvocable', 'loadedFrom', 'immediate', 'userFacingName',
])
```

### MCP Skill 安全限制

MCP 来源的 Skill 被视为**不受信任的远程内容**：

1. **禁止内联 Shell 执行** —— `!command` 和 ` ```! ``` ` 块不会被执行
2. **`${CLAUDE_SKILL_DIR}` 无效** —— 远程 Skill 没有本地目录
3. **权限检查** —— 需通过 SkillTool 的完整权限矩阵

### Bundled Skill 文件安全

内置 Skill 的参考文件提取使用多层防御：

```typescript
// 1. 每进程随机 nonce 目录
const dir = getBundledSkillExtractDir(definition.name)
// → getBundledSkillsRoot() 包含 per-process nonce

// 2. 严格文件权限
await mkdir(parent, { recursive: true, mode: 0o700 })  // 目录：仅所有者

// 3. 防符号链接攻击的原子写入
const SAFE_WRITE_FLAGS =
  O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW
//  写入     创建      排他      不跟随符号链接

await open(path, SAFE_WRITE_FLAGS, 0o600)  // 文件：仅所有者读写

// 4. 路径遍历验证
function resolveSkillFilePath(baseDir, relPath) {
  const normalized = normalize(relPath)
  if (isAbsolute(normalized) ||
      normalized.split(pathSep).includes('..') ||
      normalized.split('/').includes('..')) {
    throw new Error(`bundled skill file path escapes skill dir: ${relPath}`)
  }
  return join(baseDir, normalized)
}

// 5. EEXIST 不重试（unlink 跟随符号链接，不安全）
```

---

## 11. 动态发现与条件激活

### 动态 Skill 发现

当用户操作文件时（Read/Write/Edit），系统从文件路径**向上遍历**查找 `.claude/skills/` 目录。

```
状态存储：
├─ dynamicSkillDirs: Set<string>        — 已检查的目录（避免重复 stat）
├─ dynamicSkills: Map<string, Command>  — 运行时发现的 Skills
├─ conditionalSkills: Map<string, Command> — 有 paths 条件的休眠 Skills
├─ activatedConditionalSkillNames: Set<string> — 已激活的条件 Skill 名
└─ skillsLoaded: Signal                 — Skills 加载完成信号
```

### 发现流程：`discoverSkillDirsForPaths()`

```
discoverSkillDirsForPaths(filePaths, cwd)
  │
  ├─ 对每个 filePath:
  │    ├─ currentDir = dirname(filePath)
  │    ├─ while currentDir.startsWith(cwd + pathSep):  ← 不含 cwd 本身
  │    │    ├─ skillDir = join(currentDir, '.claude', 'skills')
  │    │    ├─ dynamicSkillDirs 已包含? → 跳过
  │    │    ├─ 标记为已检查
  │    │    ├─ stat(skillDir) 存在?
  │    │    │    ├─ isPathGitignored? → 跳过（阻止 node_modules）
  │    │    │    └─ 存在且未忽略 → 加入 newDirs
  │    │    └─ 上移到父目录
  │    └─ 直到到达 cwd 或文件系统根
  │
  └─ 按路径深度排序（最深优先） → 返回
```

### 加载并合并：`addSkillDirectories()`

```
addSkillDirectories(dirs)
  │
  ├─ 检查: projectSettings 启用 && 非 plugin-only 策略
  ├─ 并行加载所有目录
  ├─ 逆序处理（浅层先入，深层覆盖）
  ├─ 合并到 dynamicSkills Map
  ├─ 遥测日志
  └─ skillsLoaded.emit()  → 通知监听器清除缓存
```

### 条件激活：`activateConditionalSkillsForPaths()`

有 `paths` 前言的 Skill 在启动时处于**休眠状态**，当 LLM 操作匹配的文件时才激活：

```
activateConditionalSkillsForPaths(filePaths, cwd)
  │
  ├─ 遍历 conditionalSkills Map
  │    ├─ 使用 ignore 库（gitignore 风格匹配）
  │    ├─ 文件路径相对于 cwd 计算
  │    ├─ 匹配成功:
  │    │    ├─ 从 conditionalSkills → 移到 dynamicSkills
  │    │    └─ 记录到 activatedConditionalSkillNames
  │    └─ 匹配失败: 继续休眠
  │
  ├─ 遥测: tengu_dynamic_skills_changed (source: 'conditional_paths')
  └─ skillsLoaded.emit()
```

### 缓存失效机制

```
skillsLoaded.emit()
  → onDynamicSkillsLoaded() 回调
  → 清除 getCommands() 缓存
  → 清除 prompt 缓存
  → 下次查询时重新包含新 Skill
```

---

## 12. Bundled Skills 注册机制

### 注册流程

```typescript
// src/skills/bundledSkills.ts

// 内部数组存储
const bundledSkills: Command[] = []

export function registerBundledSkill(definition: BundledSkillDefinition): void {
  const { files } = definition

  let skillRoot: string | undefined
  let getPromptForCommand = definition.getPromptForCommand

  // 如果有附带文件，包装 getPromptForCommand 以延迟提取
  if (files && Object.keys(files).length > 0) {
    skillRoot = getBundledSkillExtractDir(definition.name)
    let extractionPromise: Promise<string | null> | undefined  // 并发安全

    const inner = definition.getPromptForCommand
    getPromptForCommand = async (args, ctx) => {
      // 首次调用时提取文件到磁盘
      extractionPromise ??= extractBundledSkillFiles(definition.name, files)
      const extractedDir = await extractionPromise
      const blocks = await inner(args, ctx)
      if (extractedDir === null) return blocks
      return prependBaseDir(blocks, extractedDir)  // 添加基目录头
    }
  }

  // 构建 Command 对象
  const command: Command = {
    type: 'prompt',
    name: definition.name,
    source: 'bundled',
    loadedFrom: 'bundled',
    isHidden: !(definition.userInvocable ?? true),
    progressMessage: 'running',
    // ... 其余字段
  }
  bundledSkills.push(command)
}
```

### 初始化入口

```typescript
// src/skills/bundled/index.ts
export function initBundledSkills(): void {
  // 无条件注册
  registerUpdateConfigSkill()
  registerKeybindingsSkill()
  registerVerifySkill()
  registerDebugSkill()
  registerLoremIpsumSkill()
  registerSkillifySkill()
  registerRememberSkill()
  registerSimplifySkill()
  registerBatchSkill()
  registerStuckSkill()

  // 特性门控注册
  if (feature('KAIROS') || feature('KAIROS_DREAM')) {
    registerDreamSkill()
  }
  if (feature('REVIEW_ARTIFACT')) {
    registerHunterSkill()
  }
  if (feature('AGENT_TRIGGERS')) {
    registerLoopSkill()
  }
  if (feature('AGENT_TRIGGERS_REMOTE')) {
    registerScheduleRemoteAgentsSkill()
  }
  if (feature('BUILDING_CLAUDE_APPS')) {
    registerClaudeApiSkill()
  }
  if (shouldAutoEnableClaudeInChrome()) {
    registerClaudeInChromeSkill()
  }
  if (feature('RUN_SKILL_GENERATOR')) {
    registerRunSkillGeneratorSkill()
  }
}
```

---

## 13. 内置 Skill 清单

### 始终可用

| Skill | 描述 | 用户调用 | LLM 调用 |
|-------|------|---------|---------|
| update-config | 配置 settings.json（hooks, permissions, env vars） | ✅ | ✅ |
| keybindings-help | 自定义键盘快捷键 | ✅ | ✅ |
| verify | 验证代码变更 | ✅ | ✅ |
| debug | 调试辅助 | ✅ | ✅ |
| lorem-ipsum | 生成测试文本 | ✅ | ✅ |
| remember | 持久化记忆管理 | ✅ | ✅ |
| simplify | 审查代码复用、质量和效率 | ✅ | ✅ |
| batch | 批量操作 | ✅ | ✅ |
| stuck | 帮助处理卡住的情况 | ✅ | ✅ |

### 条件可用

| Skill | 描述 | 门控条件 |
|-------|------|---------|
| skillify | 交互式 Skill 创建向导 | `USER_TYPE === 'ant'` |
| loop | 循环执行任务 | `AGENT_TRIGGERS` |
| schedule-remote-agents | 调度远程 Agent | `AGENT_TRIGGERS_REMOTE` |
| claude-api | Claude API 开发辅助 | `BUILDING_CLAUDE_APPS` |
| claude-in-chrome | Chrome 集成 | `shouldAutoEnableClaudeInChrome()` |
| dream | 辅助模式 | `KAIROS` / `KAIROS_DREAM` |
| hunter | 审查工件 | `REVIEW_ARTIFACT` |
| run-skill-generator | Skill 生成器 | `RUN_SKILL_GENERATOR` |

---

## 14. MCP Skill 桥接

### 循环依赖问题

MCP Skill 发现代码（`mcpSkills.ts`）需要使用 `loadSkillsDir.ts` 中的 `createSkillCommand` 和 `parseSkillFrontmatterFields`，但直接导入会产生循环依赖。

### 解决方案：写一次注册表

```typescript
// src/skills/mcpSkillBuilders.ts — 依赖图叶节点

export type MCPSkillBuilders = {
  createSkillCommand: typeof createSkillCommand
  parseSkillFrontmatterFields: typeof parseSkillFrontmatterFields
}

let builders: MCPSkillBuilders | null = null

export function registerMCPSkillBuilders(b: MCPSkillBuilders): void {
  builders = b
}

export function getMCPSkillBuilders(): MCPSkillBuilders {
  if (!builders) {
    throw new Error(
      'MCP skill builders not registered — loadSkillsDir.ts has not been evaluated yet',
    )
  }
  return builders
}
```

```
注册时机：
  loadSkillsDir.ts 模块初始化
  → registerMCPSkillBuilders({ createSkillCommand, parseSkillFrontmatterFields })

使用时机：
  MCP Server 连接 → 发现 Skills
  → getMCPSkillBuilders() → 获取构建器
  → createSkillCommand() → 构建 Command 对象
```

---

## 15. Skillify —— 交互式 Skill 创建向导

### 概述

Skillify 是一个内置 Skill，用于**将当前会话中的可重复流程捕获为新的 Skill 文件**。

### 配置

```typescript
registerBundledSkill({
  name: 'skillify',
  description: "Capture this session's repeatable process into a skill...",
  allowedTools: ['Read', 'Write', 'Edit', 'Glob', 'Grep',
                 'AskUserQuestion', 'Bash(mkdir:*)'],
  userInvocable: true,
  disableModelInvocation: true,  // 仅用户手动调用
  argumentHint: '[description of the process you want to capture]',
})
```

### 4 轮交互流程

```
/skillify [可选描述]
  │
  ├─ Step 1: 分析会话
  │    ├─ 获取 sessionMemory
  │    ├─ 提取用户消息
  │    └─ 识别可重复流程、输入参数、步骤、成功标准
  │
  ├─ Step 2: 用户访谈（通过 AskUserQuestion）
  │    ├─ Round 1: 确认名称、描述、目标
  │    ├─ Round 2: 展示步骤、建议参数、inline/fork、保存位置
  │    ├─ Round 3: 每步细节（产出物、成功标准、人工确认点、并行性）
  │    └─ Round 4: 触发条件、注意事项
  │
  ├─ Step 3: 编写 SKILL.md
  │    ├─ 生成 YAML Frontmatter
  │    ├─ 编写步骤说明（含成功标准、执行模式、规则）
  │    └─ 输出完整文件供审查
  │
  └─ Step 4: 确认并保存
       ├─ 用户确认
       ├─ 写入文件
       └─ 告知调用方式
```

### 保存位置选项

- **项目级**: `.claude/skills/<name>/SKILL.md` — 项目特定工作流
- **个人级**: `~/.claude/skills/<name>/SKILL.md` — 跨项目个人工作流

---

## 16. Prompt 生成与预算管理

### Skill 列表在系统 Prompt 中的呈现

SkillTool 在系统提示中包含**所有可用 Skill 的列表**，但受**上下文窗口预算**约束。

```typescript
// src/tools/SkillTool/prompt.ts

// 预算: 上下文窗口的 1%
const SKILL_BUDGET_CONTEXT_PERCENT = 0.01
const CHARS_PER_TOKEN = 4
const DEFAULT_CHAR_BUDGET = 8_000  // 回退值: 200K * 4 * 1%

// 每条目描述上限
const MAX_LISTING_DESC_CHARS = 250
```

### 截断策略

```
formatCommandsWithinBudget(commands, contextWindowTokens)
  │
  ├─ 1. 尝试完整描述
  │    └─ 总字符 ≤ 预算 → 全部保留
  │
  ├─ 2. 分区：Bundled（永不截断） vs 其他
  │    ├─ Bundled Skills 始终保持完整描述
  │    └─ 其他 Skills 按剩余预算分配
  │
  ├─ 3. 计算最大描述长度
  │    ├─ remainingBudget = budget - bundledChars
  │    └─ maxDescLen = availableForDescs / restCommands.length
  │
  ├─ 4. maxDescLen < 20?
  │    └─ 极端情况：非 Bundled 仅显示名称
  │
  └─ 5. 正常截断
       └─ truncate(description, maxDescLen)
```

### Prompt 模板

```
Execute a skill within the main conversation

When users ask you to perform tasks, check if any of the available skills match.
Skills provide specialized capabilities and domain knowledge.

How to invoke:
- Use this tool with the skill name and optional arguments
- Examples:
  - skill: "pdf" - invoke the pdf skill
  - skill: "commit", args: "-m 'Fix bug'" - invoke with arguments
  - skill: "review-pr", args: "123" - invoke with arguments

Important:
- Available skills are listed in system-reminder messages in the conversation
- When a skill matches the user's request, invoke the relevant Skill tool BEFORE
  generating any other response about the task
- NEVER mention a skill without actually calling this tool
- Do not invoke a skill that is already running
```

---

## 17. Skill 使用追踪

```typescript
// src/utils/suggestions/skillUsageTracking.ts

// 每次 Skill 调用时记录
recordSkillUsage(commandName)

// 用于排序和推荐：
// - 使用频率排名
// - 7 天指数衰减
// - 影响 Skill 列表中的排序
```

---

## 18. 遥测与分析

### 遥测事件：`tengu_skill_tool_invocation`

每次 Skill 调用时记录，包含以下字段：

| 字段 | 脱敏 | 说明 |
|------|------|------|
| `command_name` | 脱敏 | 内置/bundled/official = 真名, 否则 = "custom" |
| `_PROTO_skill_name` | PII 标记 | 真实 Skill 名（路由到特权 BQ 列） |
| `execution_context` | 否 | 'inline' / 'fork' / 'remote' |
| `invocation_trigger` | 否 | 'nested-skill' / 'claude-proactive' |
| `query_depth` | 否 | 查询深度 |
| `parent_agent_id` | 否 | 父 Agent ID |
| `was_discovered` | 否 | 是否通过 SkillSearch 发现 |
| `skill_source` | ant-only | 来源 |
| `skill_loaded_from` | ant-only | 加载来源 |
| `skill_kind` | ant-only | 类型标记 |
| `_PROTO_plugin_name` | PII 标记 | 插件名 |
| `_PROTO_marketplace_name` | PII 标记 | 市场名 |

### 其他遥测事件

| 事件 | 触发 |
|------|------|
| `tengu_skill_tool_slash_prefix` | Skill 名含前导 / |
| `tengu_skill_descriptions_truncated` | Prompt 预算触发截断 |
| `tengu_dynamic_skills_changed` | 动态发现或条件激活 |
| `tengu_remote_skill_loaded` | 远程 Skill 加载 |

---

## 19. 完整生命周期流程图

```
═══════════════════════════════════════════════════════════════
                     SKILL 完整生命周期
═══════════════════════════════════════════════════════════════

【定义阶段】

  开发者编写 SKILL.md              Bundled Skill 注册
  ──────────────────               ─────────────────
  .claude/skills/                  registerBundledSkill({
    my-skill/                        name: 'review',
      SKILL.md                       getPromptForCommand: ...
                                   })

                   ↓                         ↓

【发现阶段】

  getSkillDirCommands(cwd)         initBundledSkills()
  ├─ managed skills                ├─ 无条件注册
  ├─ user skills                   └─ 特性门控注册
  ├─ project skills
  ├─ additional dirs               MCP 连接
  └─ legacy commands               └─ getMCPSkillBuilders()
                                     → createSkillCommand()
          ↓                              ↓

  parseFrontmatter()               ────→ Command[]
  parseSkillFrontmatterFields()          (统一注册表)
  createSkillCommand()

                   ↓

【注册阶段】

  ┌──────────────────────────────────┐
  │  Command Registry (getCommands)  │
  │  ├─ Local Commands               │
  │  ├─ Bundled Skills               │
  │  ├─ Disk-based Skills            │
  │  ├─ Dynamic Skills               │
  │  ├─ Conditional Skills (休眠)    │
  │  ├─ Plugin Skills                │
  │  └─ MCP Skills                   │
  └──────────────────────────────────┘

                   ↓

【发现阶段 (运行时)】

  用户编辑 src/deep/file.ts
         ↓
  discoverSkillDirsForPaths()
  → 沿路径向上查找 .claude/skills/
  → addSkillDirectories()
  → skillsLoaded.emit()

  activateConditionalSkillsForPaths()
  → paths 模式匹配
  → 休眠 Skill → 激活

                   ↓

【调用阶段】

  用户: /review-pr 123            LLM 自动: SkillTool({ skill: "review-pr" })
         ↓                                  ↓

  ┌─────────────────────────────────────────────────┐
  │  SkillTool                                      │
  │  ├─ validateInput()  → 验证存在性 + 可调用性    │
  │  ├─ checkPermissions() → 3层权限检查            │
  │  │    ├─ deny rules                             │
  │  │    ├─ allow rules                            │
  │  │    ├─ auto-allow (safe properties)           │
  │  │    └─ ask user                               │
  │  └─ call() → 执行                              │
  └─────────────────────────────────────────────────┘

                   ↓

【执行阶段】

  ┌───── Inline ─────┐      ┌───── Fork ──────┐
  │                   │      │                  │
  │ processPrompt     │      │ runAgent({       │
  │ SlashCommand()    │      │   agentDef,      │
  │   ↓               │      │   promptMsgs,    │
  │ 展开 Prompt       │      │   ...            │
  │   ↓               │      │ })               │
  │ 注入 newMessages  │      │   ↓              │
  │   ↓               │      │ 子 Agent 独立    │
  │ contextModifier:  │      │ 执行             │
  │  + allowedTools   │      │   ↓              │
  │  + model override │      │ extractResult()  │
  │  + effort override│      │   ↓              │
  │   ↓               │      │ 返回结果摘要     │
  │ LLM 继续处理      │      │                  │
  │                   │      │                  │
  └───────────────────┘      └──────────────────┘

                   ↓

【追踪阶段】

  recordSkillUsage()          → 使用频率追踪
  logEvent('tengu_skill_...')  → 遥测分析
  addInvokedSkill()           → 压缩保留标记

═══════════════════════════════════════════════════════════════
```

---

## 20. 扩展指南：如何添加新 Skill

### 方式一：文件系统 Skill（推荐）

1. 创建目录和文件：

```bash
mkdir -p .claude/skills/my-skill
```

2. 编写 `SKILL.md`：

```markdown
---
name: My Skill
description: 做一件很棒的事情
allowed-tools:
  - Read
  - Bash(npm:*)
when_to_use: 当用户想要做很棒的事情时使用
---

# My Skill

你的详细指令...
```

3. 保存后自动发现（或重启 Claude Code）。

### 方式二：Bundled Skill（仓库贡献者）

1. 创建 `src/skills/bundled/mySkill.ts`：

```typescript
import { registerBundledSkill } from '../bundledSkills.js'

export function registerMySkill(): void {
  registerBundledSkill({
    name: 'my-skill',
    description: '描述',
    whenToUse: '当用户想要...',
    allowedTools: ['Read', 'Write'],
    async getPromptForCommand(args, context) {
      return [{ type: 'text', text: `你的 Prompt 内容...\n参数: ${args}` }]
    },
  })
}
```

2. 在 `src/skills/bundled/index.ts` 中注册：

```typescript
import { registerMySkill } from './mySkill.js'

export function initBundledSkills(): void {
  // ... 现有注册
  registerMySkill()  // 添加这行
}
```

### 方式三：使用 Skillify

在完成一个工作流后，输入 `/skillify` 并按照 4 轮交互向导自动生成 SKILL.md。

---

## 附录：关键设计决策

### 为什么 Skill 是 Markdown 而不是代码？

1. **声明式** —— 任何人都能编写，无需编程能力
2. **安全** —— Markdown 本身无副作用，Shell 执行可控
3. **可组合** —— LLM 理解自然语言，能灵活组合多个 Skill
4. **可版本控制** —— 纯文本，Git 友好

### 为什么使用 realpath 去重而不是 inode？

某些文件系统（虚拟/容器/NFS/ExFAT）报告不可靠的 inode 值（如 0 或精度丢失）。`realpath()` 解析符号链接到规范路径，跨文件系统可靠。

### 为什么 MCP Skill 禁止 Shell 执行？

MCP Skills 来自远程、不受信任的来源。允许其中的 `!command` 执行将构成远程代码执行漏洞。权限对话框是实际安全边界，Shell 禁止是纵深防御。

### 为什么 auto-allow 用白名单而不是黑名单？

白名单（SAFE_SKILL_PROPERTIES）确保未来新增的属性**默认需要权限审批**。黑名单无法防范未知风险。
