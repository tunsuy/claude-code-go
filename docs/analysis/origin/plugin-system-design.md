# Claude Code 插件系统设计深度分析

> 本文档全面分析 Claude Code 的插件系统架构设计，涵盖设计理念、核心技术、数据流、LLM 集成、UI 呈现等各方面。

---

## 目录

1. [设计理念与宏观架构](#1-设计理念与宏观架构)
2. [核心类型系统](#2-核心类型系统)
3. [插件开发规范](#3-插件开发规范)
4. [插件注册与加载机制](#4-插件注册与加载机制)
5. [Marketplace 市场系统](#5-marketplace-市场系统)
6. [插件在 LLM Loop 中的工作原理](#6-插件在-llm-loop-中的工作原理)
7. [Hooks 钩子系统](#7-hooks-钩子系统)
8. [MCP/LSP 服务器集成](#8-mcplsp-服务器集成)
9. [权限与安全体系](#9-权限与安全体系)
10. [UI 呈现与交互](#10-ui-呈现与交互)
11. [插件管理生命周期](#11-插件管理生命周期)
12. [数据流全景图](#12-数据流全景图)
13. [关键设计模式与技术点](#13-关键设计模式与技术点)

---

## 1. 设计理念与宏观架构

### 1.1 设计哲学

Claude Code 的插件系统遵循以下核心设计理念：

**1. 声明式优先 (Declarative-First)**
- 插件主要通过 Markdown 文件 + JSON 清单来声明能力，而非编写可执行代码
- 技能(Skill)的核心是一个 `.md` 文件，其中包含给 LLM 的 prompt 指令
- 钩子(Hook)通过 JSON 配置 + shell 命令来声明，无需引入运行时依赖

**2. 安全沙箱 (Security Sandbox)**
- 插件不直接运行在主进程中，而是通过受限的接口与系统交互
- MCP 服务器作为子进程运行，通过标准协议通信
- Hooks 通过 `execFile` 在独立进程执行，有超时和权限控制
- 路径遍历防护：所有插件路径都经过 `validatePathWithinBase()` 校验

**3. 渐进式能力扩展 (Progressive Enhancement)**
- 一个最简插件只需一个 `commands/` 目录和一个 `.md` 文件
- 插件可按需添加 agents、skills、hooks、MCP servers、LSP servers、output-styles 等组件
- 不需要 `plugin.json` manifest——缺失时系统会自动生成默认值

**4. 市场化分发 (Marketplace-driven Distribution)**
- 插件通过"Marketplace"（类似包仓库）来发现和分发
- 支持多种来源：GitHub、Git URL、NPM、本地目录
- 版本化缓存系统避免重复下载

**5. 企业级治理 (Enterprise Governance)**
- 支持 managed settings（企业管理策略）
- Marketplace 白名单/黑名单策略
- 官方市场名称防伪机制（homograph 攻击防护）

### 1.2 宏观架构层次

```
┌─────────────────────────────────────────────────────────┐
│                      UI Layer                           │
│  /plugin 命令 → PluginSettings → ManagePlugins/Browse   │
│  Tab 导航: Discover | Installed | Marketplaces | Errors  │
├─────────────────────────────────────────────────────────┤
│                   Integration Layer                      │
│  SkillTool ─── LLM Query Loop ─── Hook System           │
│  MCP Client ─── LSP Client ─── Permission System        │
├─────────────────────────────────────────────────────────┤
│                   Core Plugin Layer                      │
│  PluginLoader ─── MarketplaceManager ─── BuiltinPlugins │
│  PluginInstallationManager ─── DependencyResolver       │
├─────────────────────────────────────────────────────────┤
│                   Storage Layer                          │
│  ~/.claude/plugins/cache/   → 版本化插件缓存             │
│  ~/.claude/settings.json    → 用户设置/启用状态           │
│  installed_plugins.json     → 安装元数据                 │
│  known_marketplaces.json    → 已注册市场源               │
└─────────────────────────────────────────────────────────┘
```

### 1.3 插件组件全景

一个完整的插件可以提供以下组件：

| 组件类型 | 说明 | 对应文件/目录 |
|---------|------|-------------|
| **Commands** | 斜杠命令 (如 `/build`) | `commands/*.md` |
| **Agents** | 自定义 AI Agent 定义 | `agents/*.md` |
| **Skills** | 技能目录 (包含 SKILL.md) | `skills/*/SKILL.md` |
| **Hooks** | 生命周期钩子 | `hooks/hooks.json` |
| **MCP Servers** | Model Context Protocol 服务器 | `.mcp.json` 或 manifest 配置 |
| **LSP Servers** | Language Server Protocol 服务器 | manifest `lspServers` 配置 |
| **Output Styles** | 输出样式定制 | `output-styles/*.md` |
| **Settings** | 设置合并（如 agent 配置）| `settings.json` 或 manifest |

---

## 2. 核心类型系统

### 2.1 插件类型定义

**文件**: `src/types/plugin.ts`

系统定义了三种核心插件类型：

```typescript
// 内置插件定义——随 CLI 一起发布
type BuiltinPluginDefinition = {
  name: string                        // 插件名称
  description: string                  // UI 中展示的描述
  version?: string                     // 可选的版本号
  skills?: BundledSkillDefinition[]    // 提供的技能
  hooks?: HooksSettings                // 提供的钩子
  mcpServers?: Record<string, McpServerConfig>  // 提供的 MCP 服务器
  isAvailable?: () => boolean          // 可用性检查（如系统能力检测）
  defaultEnabled?: boolean             // 默认启用状态
}

// 已加载的插件——运行时的完整插件表示
type LoadedPlugin = {
  name: string                         // 插件名称
  manifest: PluginManifest             // 插件清单
  path: string                         // 插件目录绝对路径
  source: string                       // 来源标识 (如 "my-plugin@my-marketplace")
  repository: string                   // 仓库标识
  enabled?: boolean                    // 当前启用状态
  isBuiltin?: boolean                  // 是否为内置插件
  sha?: string                         // Git commit SHA (用于版本固定)
  commandsPath?: string                // commands/ 目录路径
  commandsPaths?: string[]             // 额外的命令路径
  commandsMetadata?: Record<string, CommandMetadata>  // 命令元数据
  agentsPath?: string                  // agents/ 目录路径
  agentsPaths?: string[]               // 额外 agent 路径
  skillsPath?: string                  // skills/ 目录路径
  skillsPaths?: string[]               // 额外 skill 路径
  outputStylesPath?: string            // output-styles/ 目录路径
  hooksConfig?: HooksSettings          // 钩子配置
  mcpServers?: Record<string, McpServerConfig>   // MCP 服务器配置
  lspServers?: Record<string, LspServerConfig>   // LSP 服务器配置
  settings?: Record<string, unknown>   // 插件设置（合并到全局设置）
}

// 插件加载结果
type PluginLoadResult = {
  enabled: LoadedPlugin[]   // 已启用的插件
  disabled: LoadedPlugin[]  // 已禁用的插件
  errors: PluginError[]     // 加载错误
}
```

### 2.2 错误类型系统

**设计亮点**: 采用 **Discriminated Union（可辨识联合）** 模式，定义了 24+ 种精确的错误类型：

```typescript
type PluginError =
  | { type: 'path-not-found'; source: string; path: string; component: PluginComponent }
  | { type: 'git-auth-failed'; source: string; gitUrl: string; authType: 'ssh' | 'https' }
  | { type: 'git-timeout'; source: string; gitUrl: string; operation: 'clone' | 'pull' }
  | { type: 'network-error'; source: string; url: string; details?: string }
  | { type: 'manifest-parse-error'; source: string; manifestPath: string; parseError: string }
  | { type: 'manifest-validation-error'; source: string; validationErrors: string[] }
  | { type: 'plugin-not-found'; source: string; pluginId: string; marketplace: string }
  | { type: 'marketplace-blocked-by-policy'; source: string; marketplace: string; blockedByBlocklist?: boolean }
  | { type: 'mcp-server-suppressed-duplicate'; source: string; serverName: string; duplicateOf: string }
  | { type: 'dependency-unsatisfied'; source: string; dependency: string; reason: 'not-enabled' | 'not-found' }
  | { type: 'plugin-cache-miss'; source: string; installPath: string }
  | { type: 'lsp-server-crashed'; source: string; serverName: string; exitCode: number | null }
  // ... 更多类型
```

**技术价值**: 每种错误类型携带特定的上下文数据，使得：
- UI 层可以为不同错误类型提供精准的用户引导
- 遥测系统可以按类型分类分析错误
- 类型安全避免了字符串匹配的脆弱性

### 2.3 Schema 验证体系

**文件**: `src/utils/plugins/schemas.ts`

使用 Zod v4 构建了完整的运行时验证体系：

```typescript
// 插件 Manifest Schema——验证 plugin.json 的结构
const PluginManifestSchema = z.object({
  name: z.string().min(1).refine(name => !name.includes(' ')),
  version: z.string().optional(),
  description: z.string().optional(),
  author: PluginAuthorSchema().optional(),
  homepage: z.string().url().optional(),
  keywords: z.array(z.string()).optional(),
  dependencies: z.array(DependencyRefSchema()).optional(),
  // ... 合并多个子 Schema
  ...PluginManifestHooksSchema().partial().shape,
  ...PluginManifestCommandsSchema().partial().shape,
  ...PluginManifestMcpServerSchema().partial().shape,
  ...PluginManifestLspServerSchema().partial().shape,
  ...PluginManifestUserConfigSchema().partial().shape,
})
```

**关键设计决策**: 未知的顶层字段会被静默剥离（zod 默认 strip 行为），而不是拒绝。这保证了：
- 旧版客户端能加载新版插件（前向兼容）
- 插件作者可以添加自定义扩展字段
- 嵌套配置对象仍然使用 strict 模式（更可能是拼写错误）

---

## 3. 插件开发规范

### 3.1 目录结构

```
my-plugin/
├── .claude-plugin/
│   └── plugin.json          # 插件清单（可选但推荐）
├── commands/                # 斜杠命令（自动发现）
│   ├── build.md             # → /plugin-name:build 命令
│   └── deploy.md            # → /plugin-name:deploy 命令
├── agents/                  # Agent 定义（自动发现）
│   └── test-runner.md       # 自定义 Agent
├── skills/                  # 技能目录（自动发现）
│   └── code-review/
│       └── SKILL.md         # 技能 prompt
├── hooks/                   # 钩子配置
│   └── hooks.json           # 钩子定义
├── output-styles/           # 输出样式
│   └── concise.md           # 输出样式定义
├── .mcp.json                # MCP 服务器配置（可选）
└── settings.json            # 插件设置（可选，仅允许白名单键）
```

### 3.2 plugin.json 清单文件

```json
{
  "name": "code-assistant",
  "version": "1.2.0",
  "description": "AI-powered code assistance tools",
  "author": {
    "name": "Acme Corp",
    "email": "team@acme.com",
    "url": "https://acme.com"
  },
  "keywords": ["coding", "assistant"],
  "license": "MIT",
  "dependencies": ["base-tools@official-marketplace"],

  "commands": {
    "about": {
      "source": "./README.md",
      "description": "Show plugin information"
    }
  },
  "agents": ["./custom-agents/reviewer.md"],
  "skills": ["./custom-skills"],
  "hooks": "./custom-hooks.json",

  "mcpServers": {
    "code-search": {
      "command": "node",
      "args": ["./mcp-server/index.js"],
      "env": { "API_KEY": "${user_config.API_KEY}" }
    }
  },

  "lspServers": {
    "typescript": {
      "command": "typescript-language-server",
      "args": ["--stdio"],
      "extensionToLanguage": { ".ts": "typescript", ".tsx": "typescriptreact" }
    }
  },

  "userConfig": {
    "API_KEY": {
      "type": "string",
      "title": "API Key",
      "description": "Your API key for the code search service",
      "required": true,
      "sensitive": true
    }
  }
}
```

### 3.3 Command/Skill 编写规范

**命令文件** (`commands/build.md`):

```markdown
---
description: Build the project
allowed-tools: Bash, Read
model: sonnet
argument-hint: "[target]"
when-to-use: When the user asks to build, compile, or package the project
---

Build the project using the appropriate build system.

Arguments: $ARGUMENTS

Steps:
1. Detect the build system (package.json, Makefile, etc.)
2. Run the appropriate build command
3. Report success or failure with relevant output
```

**Frontmatter 支持的字段**:
- `description`: 命令描述
- `allowed-tools`: 允许使用的工具列表
- `model`: 模型覆盖 (如 `sonnet`, `opus`, `haiku`)
- `argument-hint`: 参数提示
- `when-to-use`: 告诉 LLM 何时应该使用此技能（关键！）
- `disable-model-invocation`: 禁止 LLM 自动调用
- `user-invocable`: 是否允许用户手动调用
- `context`: 执行上下文 (`inline` 或 `fork`)
- `agent`: 指定的 Agent 类型

### 3.4 Hooks 配置规范

**hooks/hooks.json**:

```json
{
  "description": "Code quality hooks for the assistant",
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Running pre-tool check...'",
            "timeout": 5000
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Write",
        "hooks": [
          {
            "type": "command",
            "command": "./scripts/lint-check.sh",
            "timeout": 10000
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "./scripts/post-session-report.sh"
          }
        ]
      }
    ]
  }
}
```

---

## 4. 插件注册与加载机制

### 4.1 加载入口与优先级

**文件**: `src/utils/plugins/pluginLoader.ts`

插件加载遵循严格的优先级顺序：

```
加载优先级（高 → 低）:
1. Session-only 插件 (--plugin-dir CLI 参数)
2. Marketplace 插件 (settings.json enabledPlugins)
3. Built-in 插件 (内置于 CLI)
```

**核心加载函数**:

```typescript
// 完整加载——包含网络操作（git clone, npm install 等）
export const loadAllPlugins = memoize(async (): Promise<PluginLoadResult> => {
  const result = await assemblePluginLoadResult(() =>
    loadPluginsFromMarketplaces({ cacheOnly: false })
  )
  // 用完整结果预热 cache-only 的缓存
  loadAllPluginsCacheOnly.cache?.set(undefined, Promise.resolve(result))
  return result
})

// 仅缓存加载——不触发网络（用于启动优化）
export const loadAllPluginsCacheOnly = memoize(async (): Promise<PluginLoadResult> => {
  if (isEnvTruthy(process.env.CLAUDE_CODE_SYNC_PLUGIN_INSTALL)) {
    return loadAllPlugins()  // 同步安装模式退化到完整加载
  }
  return assemblePluginLoadResult(() =>
    loadPluginsFromMarketplaces({ cacheOnly: true })
  )
})
```

**设计要点**: 两个加载模式使用**独立的 memoize 缓存**，防止 cache-only 的结果替代需要完整加载的调用者。但反向是安全的——完整加载会预热 cache-only 缓存。

### 4.2 插件发现流程

```
assemblePluginLoadResult()
├── 1. 并行加载:
│   ├── loadPluginsFromMarketplaces()  // Marketplace 插件
│   └── loadSessionOnlyPlugins()        // --plugin-dir 插件
├── 2. 获取内置插件:
│   └── getBuiltinPlugins()
├── 3. 合并去重:
│   └── mergePluginSources({session, marketplace, builtin, managedNames})
│       ├── 管理策略检查 (managed settings 不可被覆盖)
│       ├── Session 插件覆盖同名 Marketplace 插件
│       └── 最终顺序: session → marketplace → builtin
├── 4. 依赖验证:
│   └── verifyAndDemote(allPlugins)  // 不满足依赖的插件降级为禁用
├── 5. 缓存设置:
│   └── cachePluginSettings(enabledPlugins)  // 合并插件设置到全局
└── 6. 返回 {enabled, disabled, errors}
```

### 4.3 单个插件的加载过程

```typescript
// createPluginFromPath() 的核心流程
async function createPluginFromPath(pluginPath, source, enabled, fallbackName, strict) {
  // Step 1: 加载或创建 manifest
  const manifest = await loadPluginManifest(manifestPath, fallbackName, source)

  // Step 2: 创建基础插件对象
  const plugin: LoadedPlugin = { name: manifest.name, manifest, path: pluginPath, ... }

  // Step 3: 并行检测可选目录
  const [commandsDirExists, agentsDirExists, skillsDirExists, outputStylesDirExists] =
    await Promise.all([
      pathExists(join(pluginPath, 'commands')),
      pathExists(join(pluginPath, 'agents')),
      pathExists(join(pluginPath, 'skills')),
      pathExists(join(pluginPath, 'output-styles')),
    ])

  // Step 4: 处理 manifest 中声明的额外路径
  // - commands: 支持路径字符串、路径数组、对象映射三种格式
  // - agents/skills: 支持路径字符串或路径数组
  // 所有路径检查都并行化执行

  // Step 5: 加载 hooks 配置
  // - 先从标准路径 hooks/hooks.json 加载
  // - 再从 manifest.hooks 加载（支持路径引用或内联定义）
  // - 多个 hooks 源合并，重复路径检测

  // Step 6: 加载插件设置（仅白名单键）
  const pluginSettings = await loadPluginSettings(pluginPath, manifest)

  return { plugin, errors }
}
```

### 4.4 版本化缓存系统

```
缓存目录结构:
~/.claude/plugins/
├── cache/
│   ├── {marketplace}/           # 按 marketplace 分组
│   │   └── {plugin}/           # 按插件名分组
│   │       └── {version}/      # 按版本分组
│   │           ├── .claude-plugin/
│   │           │   └── plugin.json
│   │           ├── commands/
│   │           └── ...
│   └── npm-cache/              # NPM 包全局缓存
└── installed_plugins.json       # V2 格式安装记录
```

**版本计算优先级**: manifest.version > marketplace entry.version > installed version > git SHA > `'unknown'`

**Seed 缓存**: 支持预置的种子目录（如 CCR 预构建镜像），只读读取、无需拷贝。

**ZIP 缓存**: 可选模式，将插件目录压缩为 ZIP 存储，运行时解压到会话临时目录。

### 4.5 内置插件注册

**文件**: `src/plugins/builtinPlugins.ts`

```typescript
// 全局 Map 存储注册的内置插件
const BUILTIN_PLUGINS: Map<string, BuiltinPluginDefinition> = new Map()

// 注册（在 initBuiltinPlugins() 中调用）
export function registerBuiltinPlugin(definition: BuiltinPluginDefinition): void {
  BUILTIN_PLUGINS.set(definition.name, definition)
}

// 获取（按用户设置分 enabled/disabled）
export function getBuiltinPlugins(): { enabled: LoadedPlugin[]; disabled: LoadedPlugin[] } {
  for (const [name, definition] of BUILTIN_PLUGINS) {
    // 可用性检查
    if (definition.isAvailable && !definition.isAvailable()) continue

    // 启用状态: 用户设置 > 插件默认 > true
    const isEnabled = userSetting !== undefined
      ? userSetting === true
      : (definition.defaultEnabled ?? true)

    // 转换为 LoadedPlugin（使用 'builtin' 标记路径）
    const plugin: LoadedPlugin = {
      name,
      manifest: { name, description: definition.description },
      path: 'builtin',  // 哨兵值——无实际文件系统路径
      source: `${name}@builtin`,
      isBuiltin: true,
      // ...
    }
  }
}
```

**ID 格式**: 内置插件使用 `{name}@builtin` 格式，与 marketplace 插件 `{name}@{marketplace}` 区分。

---

## 5. Marketplace 市场系统

### 5.1 市场源类型

市场支持多种来源，通过 Discriminated Union 建模：

```typescript
MarketplaceSourceSchema = z.discriminatedUnion('source', [
  { source: 'url', url: string, headers?: Record<string, string> },
  { source: 'github', repo: string, ref?: string, path?: string, sparsePaths?: string[] },
  { source: 'git', url: string, ref?: string, path?: string },
  { source: 'npm', package: string },
  { source: 'file', path: string },
  { source: 'directory', path: string },
  { source: 'hostPattern', hostPattern: string },       // 企业策略: 按主机匹配
  { source: 'pathPattern', pathPattern: string },       // 企业策略: 按路径匹配
  { source: 'settings', name: string, plugins: [...] }, // 内联在 settings.json 中
])
```

### 5.2 市场清单格式

```json
// marketplace.json
{
  "name": "my-marketplace",
  "owner": {
    "name": "Acme Corp",
    "email": "admin@acme.com"
  },
  "plugins": [
    {
      "name": "code-formatter",
      "source": "./plugins/code-formatter",  // 相对路径（市场内部）
      "description": "Auto-format code",
      "version": "2.0.0",
      "strict": true
    },
    {
      "name": "db-tools",
      "source": {                             // 外部源（GitHub）
        "source": "github",
        "repo": "acme/db-tools",
        "ref": "v1.3.0"
      },
      "category": "database"
    }
  ],
  "forceRemoveDeletedPlugins": false,
  "allowCrossMarketplaceDependenciesOn": ["official-marketplace"]
}
```

### 5.3 插件源类型

插件本身也支持多种来源：

```typescript
PluginSourceSchema = z.union([
  RelativePath(),                              // "./my-plugin" (市场内相对路径)
  { source: 'npm', package: string, version?: string, registry?: string },
  { source: 'pip', package: string, version?: string },
  { source: 'github', repo: string, ref?: string, sha?: string },
  { source: 'url', url: string, ref?: string, sha?: string },
  { source: 'git-subdir', url: string, path: string, ref?: string, sha?: string },
])
```

### 5.4 官方市场防伪

```typescript
// 保留的官方市场名称
const ALLOWED_OFFICIAL_MARKETPLACE_NAMES = new Set([
  'claude-code-marketplace',
  'claude-code-plugins',
  'anthropic-marketplace',
  'agent-skills',
  // ...
])

// 防止冒充的正则模式
const BLOCKED_OFFICIAL_NAME_PATTERN =
  /(?:official[^a-z0-9]*(anthropic|claude)|...)/i

// Unicode homograph 攻击防护
const NON_ASCII_PATTERN = /[^ -~]/

// 源验证: 保留名称只能来自官方 GitHub 组织
function validateOfficialNameSource(name, source): string | null {
  if (ALLOWED_OFFICIAL_MARKETPLACE_NAMES.has(name.toLowerCase())) {
    // 必须来自 github.com/anthropics/
    if (source.source === 'github' && !repo.startsWith('anthropics/')) {
      return 'reserved for official...'
    }
  }
}
```

---

## 6. 插件在 LLM Loop 中的工作原理

### 6.1 SkillTool——插件能力进入 LLM 的桥梁

**文件**: `src/tools/SkillTool/SkillTool.ts`

SkillTool 是插件系统与 LLM 对话循环之间的核心桥梁。它作为一个标准的 Tool 注册到 LLM 可用的工具列表中，使 LLM 能够调用插件提供的技能。

```typescript
export const SkillTool: Tool<InputSchema, Output, Progress> = buildTool({
  name: 'Skill',           // 工具名称
  searchHint: 'invoke a slash-command skill',
  maxResultSizeChars: 100_000,

  inputSchema: z.object({
    skill: z.string().describe('The skill name'),
    args: z.string().optional().describe('Optional arguments'),
  }),

  // 验证、权限检查、执行
  validateInput, checkPermissions, call,
  
  // UI 渲染方法
  renderToolUseMessage, renderToolResultMessage,
  renderToolUseProgressMessage, renderToolUseRejectedMessage,
})
```

### 6.2 技能发现机制

LLM 如何"知道"有哪些技能可用？

```
System Init 消息注入技能列表
           │
           ▼
system-reminder 消息格式:
"The following skills are available for use with the Skill tool:
- update-config: Use this skill to configure...
- review: Review a pull request
- security-review: Complete a security review..."
           │
           ▼
SkillTool 的 prompt 描述了如何使用:
"Execute a skill within the main conversation
 When users ask you to perform tasks, check if any
 of the available skills match..."
```

**技能列表预算控制**: 技能描述占据上下文窗口的 1%（约 8,000 字符），超出时非捆绑技能的描述会被截断：

```typescript
const SKILL_BUDGET_CONTEXT_PERCENT = 0.01
const MAX_LISTING_DESC_CHARS = 250

// 截断策略:
// 1. 捆绑(bundled)技能——永不截断
// 2. 其他技能——按比例缩减描述长度
// 3. 极端情况——非捆绑技能只保留名称
```

### 6.3 两种执行模式

#### Inline 模式 (默认)

技能的 prompt 内容直接注入到当前对话上下文中，LLM 在同一对话线程中处理：

```
LLM 调用 → Skill("review", args="PR #123")
      │
      ▼
  validateInput() → 检查技能是否存在
      │
      ▼
  checkPermissions() → 权限验证
      │
      ▼
  call() → processPromptSlashCommand()
      │
      ├── 加载 .md 文件内容
      ├── 替换 $ARGUMENTS 变量
      ├── 注册技能到 invokedSkills (用于压缩恢复)
      └── 返回 {data, newMessages, contextModifier}
              │
              ▼
         newMessages 注入到对话中 → LLM 继续处理
         contextModifier 修改:
           - 允许的工具列表
           - 模型覆盖
           - effort 级别
```

#### Fork 模式 (`context: 'fork'`)

技能在独立的子 Agent 中执行，有自己的 token 预算和工具集：

```
LLM 调用 → Skill("simplify", args="")
      │
      ▼
  command.context === 'fork'
      │
      ▼
  executeForkedSkill()
      │
      ├── createAgentId()
      ├── prepareForkedCommandContext()
      │   ├── 构建独立的 AppState
      │   └── 构建 Agent 定义和初始消息
      └── runAgent({agentDefinition, promptMessages, ...})
              │
              ├── 子 Agent 独立运行
              ├── 实时报告进度 (onProgress)
              └── 返回结果文本
                    │
                    ▼
              extractResultText() → 提取最终结果
              clearInvokedSkillsForAgent() → 清理状态
```

### 6.4 LLM Loop 中的完整数据流

```
用户输入 "review this PR"
           │
           ▼
   ┌───────────────────────────────────┐
   │         Query Loop (query.ts)      │
   │                                    │
   │  1. 构建消息 + System Prompt       │
   │     (包含 skill listing)           │
   │                                    │
   │  2. 发送到 Claude API ──────────┐  │
   │                                 │  │
   │  3. 接收流式响应 ◄──────────────┘  │
   │     ├── 文本内容 → 直接展示        │
   │     └── tool_use: Skill ──────┐    │
   │                               │    │
   │  4. SkillTool.call() ◄────────┘    │
   │     ├── 加载技能 prompt             │
   │     ├── 注入 newMessages            │
   │     └── 设置 contextModifier        │
   │              │                      │
   │  5. 继续 Loop ◄──────────────────   │
   │     ├── 消息列表增长               │
   │     ├── 上下文被 contextModifier    │
   │     │   修改 (允许的工具等)         │
   │     │                              │
   │  6. Claude 根据技能 prompt 执行     │
   │     ├── 使用 Bash 工具             │
   │     ├── 使用 Read 工具             │
   │     └── 生成 PR 评审结果            │
   │                                    │
   │  7. 执行 Post-Sampling Hooks       │
   │     └── 插件钩子可以观察/修改行为   │
   │                                    │
   │  8. 返回最终结果给用户              │
   └───────────────────────────────────┘
```

### 6.5 Context Modifier 机制

SkillTool 返回的 `contextModifier` 是一个函数，它在后续的查询循环中修改执行上下文：

```typescript
contextModifier(ctx) {
  let modifiedContext = ctx

  // 1. 更新允许的工具列表
  if (allowedTools.length > 0) {
    modifiedContext = {
      ...modifiedContext,
      getAppState() {
        const appState = previousGetAppState()
        return {
          ...appState,
          toolPermissionContext: {
            ...appState.toolPermissionContext,
            alwaysAllowRules: {
              ...appState.toolPermissionContext.alwaysAllowRules,
              command: [...existingRules, ...allowedTools],
            },
          },
        }
      },
    }
  }

  // 2. 覆盖模型 (保留 [1m] 后缀)
  if (model) {
    modifiedContext.options.mainLoopModel = resolveSkillModelOverride(model, ctx.options.mainLoopModel)
  }

  // 3. 覆盖 effort 级别
  if (effort !== undefined) {
    modifiedContext.getAppState().effortValue = effort
  }

  return modifiedContext
}
```

---

## 7. Hooks 钩子系统

### 7.1 支持的钩子事件

**文件**: `src/utils/plugins/loadPluginHooks.ts`, `src/utils/hooks/hookEvents.ts`

系统支持 26 种钩子事件，覆盖完整的生命周期：

| 事件类别 | 事件名 | 触发时机 |
|---------|--------|---------|
| **工具生命周期** | `PreToolUse` | 工具执行前 |
| | `PostToolUse` | 工具执行后 |
| | `PostToolUseFailure` | 工具执行失败后 |
| **权限相关** | `PermissionDenied` | 权限被拒绝时 |
| | `PermissionRequest` | 权限请求时 |
| **会话生命周期** | `SessionStart` | 会话开始 |
| | `SessionEnd` | 会话结束 |
| | `Stop` | 模型停止生成 |
| | `StopFailure` | 停止失败 |
| **Sub-Agent** | `SubagentStart` | 子 Agent 启动 |
| | `SubagentStop` | 子 Agent 停止 |
| **上下文管理** | `PreCompact` | 上下文压缩前 |
| | `PostCompact` | 上下文压缩后 |
| **用户交互** | `UserPromptSubmit` | 用户提交输入时 |
| | `Notification` | 通知事件 |
| **任务管理** | `TaskCreated` | 任务创建时 |
| | `TaskCompleted` | 任务完成时 |
| | `TeammateIdle` | 队友空闲时 |
| **配置变更** | `ConfigChange` | 配置更改时 |
| | `Setup` | 初始化设置时 |
| **工作区** | `WorktreeCreate` | 工作树创建时 |
| | `WorktreeRemove` | 工作树移除时 |
| **文件系统** | `InstructionsLoaded` | 指令加载时 |
| | `CwdChanged` | 工作目录变更时 |
| | `FileChanged` | 文件变更时 |
| **Elicitation** | `Elicitation` | 引出请求时 |
| | `ElicitationResult` | 引出结果时 |

### 7.2 钩子注册流程

```typescript
// 1. 加载所有已启用插件的钩子配置
export const loadPluginHooks = memoize(async (): Promise<void> => {
  const { enabled } = await loadAllPluginsCacheOnly()

  // 2. 转换每个插件的钩子为标准 Matcher 格式
  for (const plugin of enabled) {
    const matchers = convertPluginHooksToMatchers(plugin)
    // 每个 matcher 携带插件上下文:
    // { matcher, hooks, pluginRoot, pluginName, pluginId }
    mergeTo(allPluginHooks, matchers)
  }

  // 3. 原子性注册（先清除旧的，再注册新的）
  clearRegisteredPluginHooks()
  registerHookCallbacks(allPluginHooks)
})
```

### 7.3 钩子事件系统

**文件**: `src/utils/hooks/hookEvents.ts`

钩子执行事件通过独立的事件系统广播：

```typescript
// 事件类型
type HookExecutionEvent =
  | HookStartedEvent   // { type: 'started', hookId, hookName, hookEvent }
  | HookProgressEvent  // { type: 'progress', hookId, stdout, stderr, output }
  | HookResponseEvent  // { type: 'response', hookId, exitCode, outcome }

// 事件发射
emitHookStarted(hookId, hookName, hookEvent)
emitHookProgress({ hookId, stdout, stderr, output })
emitHookResponse({ hookId, exitCode, outcome: 'success' | 'error' | 'cancelled' })

// 进度轮询（带去重）
startHookProgressInterval({
  hookId, hookName, hookEvent,
  getOutput: async () => ({ stdout, stderr, output }),
  intervalMs: 1000,
})
```

**设计亮点**: 事件在 handler 注册前会被缓冲（最多 100 条），注册时立即回放——解决了启动时序问题。

### 7.4 热重载机制

```typescript
// 监听设置变化，自动重新加载插件钩子
const detector = settingsChangeDetector()
detector.onChange(async () => {
  const snapshot = jsonStringify(settings.enabledPlugins)
  if (snapshot !== lastPluginSettingsSnapshot) {
    lastPluginSettingsSnapshot = snapshot
    clearPluginCache('hot-reload')
    loadPluginHooks.cache?.clear?.()
    await loadPluginHooks()
  }
})
```

### 7.5 Post-Sampling Hooks

**文件**: `src/utils/hooks/postSamplingHooks.ts`

这是一个内部 API，允许在 LLM 采样完成后执行钩子：

```typescript
type REPLHookContext = {
  messages: Message[]                   // 完整消息历史
  systemPrompt: SystemPrompt            // 系统提示
  userContext: { [k: string]: string }   // 用户上下文
  systemContext: { [k: string]: string } // 系统上下文
  toolUseContext: ToolUseContext         // 工具使用上下文
  querySource?: QuerySource             // 查询来源
}

// 在 query.ts 的主循环中被调用
await executePostSamplingHooks(messages, systemPrompt, userContext, ...)
```

---

## 8. MCP/LSP 服务器集成

### 8.1 MCP 服务器加载

**文件**: `src/utils/plugins/mcpPluginIntegration.ts`

插件可以通过多种方式声明 MCP 服务器：

```
MCP 服务器来源优先级:
1. .mcp.json 文件 (插件目录根)        → 最低优先级
2. manifest.mcpServers 字段:
   ├── 字符串 → JSON 配置文件路径
   ├── 字符串(.mcpb/.dxt) → MCPB 包文件
   ├── 对象 → 内联 MCP 服务器配置
   └── 数组 → 混合以上格式             → 最高优先级
```

**MCPB (MCP Bundle) 支持**: 支持 `.mcpb` 和 `.dxt` 格式的打包 MCP 服务器，可以是本地文件或远程 URL。

**UserConfig 变量替换**: MCP 配置中支持 `${user_config.KEY}` 语法，运行时从用户配置中替换。

### 8.2 MCP 客户端架构

**文件**: `src/services/mcp/client.ts`

```
传输协议支持:
├── StdioClientTransport      → 子进程 stdio 通信
├── SSEClientTransport        → Server-Sent Events
├── StreamableHTTPClientTransport → HTTP 流式
├── WebSocketTransport        → WebSocket
└── SdkControlClientTransport → SDK 控制模式

特性:
├── OAuth token 自动刷新
├── 会话过期检测 (HTTP 404 + JSON-RPC -32001)
├── 工具结果截断与持久化
├── MCP 资源处理 (List, Read)
└── 错误分类与遥测
```

### 8.3 MCP 服务器去重

当多个插件提供相同命令/URL 的 MCP 服务器时，系统会去重并发出警告：

```typescript
type PluginError = {
  type: 'mcp-server-suppressed-duplicate'
  source: string
  plugin: string
  serverName: string
  duplicateOf: string  // "plugin:other-plugin" 或 "already-configured:name"
}
```

### 8.4 LSP 服务器配置

```typescript
const LspServerConfigSchema = z.strictObject({
  command: z.string(),                    // 可执行命令
  args: z.array(z.string()).optional(),   // 命令参数
  extensionToLanguage: z.record(          // 文件扩展名到语言 ID 映射
    fileExtension(),                      // 如 ".ts"
    nonEmptyString()                      // 如 "typescript"
  ),
  transport: z.enum(['stdio', 'socket']).default('stdio'),
  env: z.record(z.string()).optional(),
  initializationOptions: z.unknown().optional(),
  settings: z.unknown().optional(),
  workspaceFolder: z.string().optional(),
  startupTimeout: z.number().optional(),
  restartOnCrash: z.boolean().optional(),
  maxRestarts: z.number().optional(),
})
```

---

## 9. 权限与安全体系

### 9.1 SkillTool 权限检查

```typescript
async checkPermissions({ skill, args }, context): Promise<PermissionDecision> {
  // 1. 检查 deny 规则
  for (const [ruleContent, rule] of denyRules.entries()) {
    if (ruleMatches(ruleContent)) return { behavior: 'deny' }
  }

  // 2. 远程规范技能——自动授权（仅内部实验）
  if (isRemoteCanonicalSkill) return { behavior: 'allow' }

  // 3. 检查 allow 规则
  for (const [ruleContent, rule] of allowRules.entries()) {
    if (ruleMatches(ruleContent)) return { behavior: 'allow' }
  }

  // 4. 安全属性自动放行
  //    只有 "安全" 属性的技能不需要权限
  if (skillHasOnlySafeProperties(command)) return { behavior: 'allow' }

  // 5. 默认: 询问用户
  return {
    behavior: 'ask',
    message: `Execute skill: ${commandName}`,
    suggestions: [
      { rules: [{ toolName: 'Skill', ruleContent: commandName }], behavior: 'allow' },
      { rules: [{ toolName: 'Skill', ruleContent: `${commandName}:*` }], behavior: 'allow' },
    ],
  }
}
```

**安全属性白名单**: 系统维护一个 `SAFE_SKILL_PROPERTIES` 集合，只包含安全属性的技能自动放行：

```typescript
const SAFE_SKILL_PROPERTIES = new Set([
  'type', 'progressMessage', 'contentLength', 'model', 'effort',
  'source', 'pluginInfo', 'context', 'agent', 'getPromptForCommand',
  'name', 'description', 'isEnabled', 'isHidden', 'aliases',
  'argumentHint', 'whenToUse', 'disableModelInvocation', 'userInvocable',
  // ... 更多
])
```

**设计原则**: 新增的属性默认需要权限——安全默认 (secure by default)。

### 9.2 企业策略控制

```
企业治理层级:
├── managed settings (最高优先级)
│   ├── 强制启用/禁用特定插件
│   ├── --plugin-dir 不能覆盖
│   └── 用户无法更改
├── strictKnownMarketplaces (白名单)
│   ├── 只允许列表中的市场源
│   └── 空列表 = 拒绝所有
├── blockedMarketplaces (黑名单)
│   ├── 明确阻止的市场源
│   └── 空列表 = 语义无操作
└── Fail-Closed 原则:
    ├── 策略已配置 + 无法验证源 = 阻止
    └── 未知源 + 活跃策略 → 阻止（而非静默跳过）
```

### 9.3 路径安全

```typescript
// 路径遍历防护
function validatePathWithinBase(baseDir: string, relativePath: string): string {
  const resolved = resolve(baseDir, relativePath)
  if (!resolved.startsWith(baseDir + sep) && resolved !== baseDir) {
    throw new Error('Path traversal detected')
  }
  return resolved
}

// 文件提取安全 (bundledSkills.ts)
// O_NOFOLLOW: 拒绝符号链接
// O_EXCL: 文件必须是新创建的
// 每进程随机 nonce 防止符号链接攻击
```

---

## 10. UI 呈现与交互

### 10.1 `/plugin` 命令入口

**文件**: `src/commands/plugin/plugin.tsx`

```typescript
export async function call(onDone: LocalJSXCommandOnDone): Promise<React.ReactNode> {
  return <PluginSettings onComplete={onDone} />
}
```

### 10.2 PluginSettings 主界面

**文件**: `src/commands/plugin/PluginSettings.tsx`

采用 Tab 导航的多面板布局：

```
┌──────────────────────────────────────────────┐
│  /plugin                                      │
│                                               │
│  [Discover] [Installed] [Marketplaces] [Errors]│
│  ─────────────────────────────────────────────│
│                                               │
│  当前 Tab 内容区:                              │
│  ├── Discover    → DiscoverPlugins 组件        │
│  ├── Installed   → ManagePlugins 组件          │
│  ├── Marketplaces→ ManageMarketplaces 组件     │
│  └── Errors      → 错误列表 + 修复建议         │
│                                               │
│  [Esc] 返回  [Tab] 切换  [Enter] 选择         │
└──────────────────────────────────────────────┘
```

### 10.3 ManagePlugins 详情视图

**文件**: `src/commands/plugin/ManagePlugins.tsx`

支持多种视图状态的状态机：

```typescript
type ViewState =
  | 'plugin-list'           // 插件列表
  | 'plugin-details'        // 插件详情
  | 'configuring'           // 配置中
  | 'plugin-options'        // 插件选项
  | 'flagged-detail'        // 标记的插件详情
  | 'failed-plugin-details' // 失败插件详情
  | 'mcp-detail'            // MCP 服务器详情
  | 'mcp-tools'             // MCP 工具列表
  | 'mcp-tool-detail'       // MCP 工具详情
```

**功能列表**:
- 按 marketplace 分组展示已安装插件
- 显示插件元数据（描述、版本、来源、作者）
- 展示插件组件：commands、agents、skills、MCP servers
- 启用/禁用/更新/卸载操作
- MCP 服务器连接状态和工具浏览
- 分页支持（大量插件列表）

### 10.4 错误展示系统

**文件**: `src/commands/plugin/PluginErrors.tsx`

```typescript
// 每种错误类型有对应的格式化方法
function formatErrorMessage(error: PluginError): string {
  switch (error.type) {
    case 'git-auth-failed':
      return `Git authentication failed (${error.authType}): ${error.gitUrl}`
    case 'marketplace-blocked-by-policy':
      return error.blockedByBlocklist
        ? `Marketplace '${error.marketplace}' is blocked by enterprise policy`
        : `Marketplace '${error.marketplace}' is not in the allowed list`
    // ...
  }
}

// 错误修复建议
function getErrorGuidance(error: PluginError): string {
  switch (error.type) {
    case 'plugin-cache-miss':
      return 'Run /plugins to refresh'
    case 'dependency-unsatisfied':
      return error.reason === 'not-enabled'
        ? 'Enable the dependency or remove it'
        : 'Plugin not found in any marketplace'
    // ...
  }
}
```

### 10.5 其他 UI 组件

| 组件 | 文件 | 功能 |
|-----|------|------|
| `DiscoverPlugins` | `DiscoverPlugins.tsx` | 市场浏览与搜索 |
| `BrowseMarketplace` | `BrowseMarketplace.tsx` | 单个市场插件列表 |
| `ManageMarketplaces` | `ManageMarketplaces.tsx` | 管理已注册市场源 |
| `AddMarketplace` | `AddMarketplace.tsx` | 添加新市场源 |
| `UnifiedInstalledCell` | `UnifiedInstalledCell.tsx` | 统一的插件列表项 |
| `PluginOptionsDialog` | `PluginOptionsDialog.tsx` | 插件配置对话框 |
| `PluginOptionsFlow` | `PluginOptionsFlow.tsx` | 插件安装配置流程 |
| `PluginTrustWarning` | `PluginTrustWarning.tsx` | 信任警告提示 |
| `ValidatePlugin` | `ValidatePlugin.tsx` | 插件验证工具 |

---

## 11. 插件管理生命周期

### 11.1 安装流程

```
用户在 /plugin UI 中选择安装
           │
           ▼
   PluginInstallationManager
           │
           ├── 1. 解析插件 ID (name@marketplace)
           ├── 2. 查找市场中的插件条目
           ├── 3. 检查企业策略
           ├── 4. 依赖解析 (qualifyDependency → resolveDependencyClosure)
           │
           ├── 5. 下载/缓存插件源:
           │   ├── 本地路径 → copyDir()
           │   ├── GitHub → gitClone() via installFromGitHub()
           │   ├── Git URL → gitClone() via installFromGit()
           │   ├── Git Subdir → installFromGitSubdir() (sparse checkout)
           │   ├── NPM → installFromNpm()
           │   └── MCPB → 下载 + 解压 + 转换
           │
           ├── 6. 拷贝到版本化缓存:
           │   └── copyPluginToVersionedCache()
           │       ~/.claude/plugins/cache/{marketplace}/{plugin}/{version}/
           │
           ├── 7. 更新 settings.json:
           │   └── enabledPlugins[pluginId] = true
           │
           ├── 8. 更新 installed_plugins.json:
           │   └── 记录 scope, installPath, version, gitCommitSha
           │
           ├── 9. 需要用户配置?
           │   └── PluginOptionsFlow → 收集 userConfig 值
           │       ├── 普通值 → settings.json pluginConfigs
           │       └── 敏感值 → macOS keychain / .credentials.json
           │
           └── 10. 刷新运行时:
               ├── clearPluginCache()
               ├── 重新加载 hooks
               └── 连接新的 MCP/LSP 服务器
```

### 11.2 启用/禁用

```typescript
// settings.json 中的表示:
{
  "enabledPlugins": {
    "code-formatter@official": true,    // 已启用
    "debug-tools@custom": false,        // 已禁用
    "editor@builtin": true              // 内置插件已启用
  }
}

// 内置插件的启用状态判定:
// 用户设置 > 插件默认值 > true
const isEnabled = userSetting !== undefined
  ? userSetting === true
  : (definition.defaultEnabled ?? true)
```

### 11.3 更新流程

```
updatePlugin(pluginId)
├── 从市场获取最新条目
├── 计算新版本号
├── 如果版本已缓存 → 跳过下载
├── 下载到临时目录 → 拷贝到新版本缓存
├── 更新 installed_plugins.json
└── clearPluginCache() → 下次加载时使用新版本
```

### 11.4 卸载流程

```
uninstallPlugin(pluginId)
├── 从 settings.json enabledPlugins 中移除
├── 从 installed_plugins.json 中移除
├── 可选: 清理版本化缓存目录
├── 断开关联的 MCP/LSP 服务器
└── clearPluginCache()
```

### 11.5 依赖管理

```typescript
// plugin.json 中声明依赖
{
  "dependencies": [
    "base-tools",              // 裸名——解析为同一 marketplace
    "auth@official-marketplace" // 完全限定名
  ]
}

// 依赖解析流程 (dependencyResolver.ts):
// 1. qualifyDependency(): 裸名 → 完全限定名
// 2. resolveDependencyClosure(): 传递性依赖解析
// 3. verifyAndDemote(): 检查依赖是否满足
//    不满足 → 插件降级为 disabled（不修改 settings）
//    用户可通过 /doctor 修复
```

---

## 12. 数据流全景图

### 12.1 启动时数据流

```
CLI 启动
  │
  ├── initBuiltinPlugins()
  │   └── registerBuiltinPlugin() × N
  │
  ├── loadAllPluginsCacheOnly()  ← 不触发网络
  │   ├── 读取 settings.json → enabledPlugins
  │   ├── 读取 installed_plugins.json → installPath
  │   ├── 并行加载每个插件目录
  │   │   ├── loadPluginManifest()
  │   │   ├── 检测 commands/agents/skills/ 目录
  │   │   └── loadPluginHooks()
  │   ├── mergePluginSources()
  │   ├── verifyAndDemote() (依赖检查)
  │   └── cachePluginSettings()
  │
  ├── loadPluginHooks()
  │   ├── convertPluginHooksToMatchers()
  │   └── registerHookCallbacks()
  │
  ├── 初始化 MCP 客户端
  │   ├── loadPluginMcpServers()
  │   └── MCPConnectionManager.connect()
  │
  ├── 初始化 LSP 客户端
  │   └── loadPluginLspServers()
  │
  └── buildSystemInitMessage()
      └── 注入插件信息到系统初始化消息
```

### 12.2 查询时数据流

```
用户提交查询
  │
  ▼
Query Loop 开始 (query.ts)
  │
  ├── 构建消息（包含 skill listing 的 system-reminder）
  │
  ├── 发送到 Claude API
  │   └── Claude 识别需要使用技能
  │
  ├── tool_use: Skill
  │   │
  │   ├── validateInput()
  │   │   └── 检查技能是否存在于 commands registry
  │   │
  │   ├── checkPermissions()
  │   │   ├── deny rules → 拒绝
  │   │   ├── allow rules → 放行
  │   │   ├── safe properties → 自动放行
  │   │   └── default → 询问用户
  │   │
  │   └── call()
  │       ├── [inline] processPromptSlashCommand()
  │       │   ├── 加载 .md 文件
  │       │   ├── 解析 frontmatter
  │       │   ├── 替换 $ARGUMENTS
  │       │   ├── addInvokedSkill() (压缩恢复)
  │       │   ├── registerSkillHooks() (技能钩子)
  │       │   └── 返回 {newMessages, contextModifier}
  │       │
  │       └── [fork] executeForkedSkill()
  │           ├── prepareForkedCommandContext()
  │           ├── runAgent() → 独立子 Agent
  │           ├── 实时 onProgress 回调
  │           └── extractResultText()
  │
  ├── newMessages 注入到消息历史
  ├── contextModifier 应用到上下文
  │
  ├── 继续 Query Loop...
  │   ├── Claude 使用技能允许的工具
  │   ├── PreToolUse hooks 触发
  │   ├── 工具执行
  │   ├── PostToolUse hooks 触发
  │   └── 生成响应
  │
  ├── Post-Sampling Hooks 执行
  │
  └── 返回结果
```

### 12.3 MCP 工具调用数据流

```
Claude 调用 MCP 工具
  │
  ▼
Tool Execution Pipeline
  │
  ├── 查找 MCP 工具对应的 client
  │
  ├── mcpClient.callTool(toolName, args)
  │   ├── 序列化请求为 JSON-RPC
  │   ├── 通过 transport 发送
  │   │   ├── stdio: 写入子进程 stdin
  │   │   ├── SSE: HTTP POST
  │   │   ├── WebSocket: ws.send()
  │   │   └── HTTP: fetch()
  │   ├── 等待响应
  │   ├── 处理 OAuth token 刷新
  │   └── 处理会话过期重连
  │
  ├── 结果处理:
  │   ├── 截断过大结果
  │   ├── 持久化结果
  │   └── 格式化为 ToolResultBlockParam
  │
  └── 返回给 Query Loop → 发送给 Claude
```

---

## 13. 关键设计模式与技术点

### 13.1 Memoize + Cache Invalidation

```typescript
// 问题: 插件加载是昂贵的 I/O 操作，但需要在多处调用
// 解决: lodash memoize + 精确的缓存失效

export const loadAllPlugins = memoize(async () => { ... })
export const loadAllPluginsCacheOnly = memoize(async () => { ... })

// 失效时机:
export function clearPluginCache(reason?: string): void {
  loadAllPlugins.cache?.clear?.()
  loadAllPluginsCacheOnly.cache?.clear?.()
  if (getPluginSettingsBase() !== undefined) {
    resetSettingsCache()  // 级联失效关联的设置缓存
  }
  clearPluginSettingsBase()
}
```

### 13.2 Lazy Schema (延迟 Schema 构建)

```typescript
// 问题: Zod schema 构建开销大，且启动时未必需要所有 schema
// 解决: lazySchema() 包装器，首次访问时才构建

const PluginManifestSchema = lazySchema(() =>
  z.object({
    ...PluginManifestMetadataSchema().shape,
    ...PluginManifestHooksSchema().partial().shape,
    // ...
  })
)

// 使用时:
const result = PluginManifestSchema().safeParse(data)  // 首次调用时构建
```

### 13.3 并行 I/O 优化

```typescript
// 问题: 多个 pathExists 检查串行执行导致性能问题
// 解决: Promise.all 并行化，保持结果顺序

// 并行检测多个目录
const [commandsDirExists, agentsDirExists, skillsDirExists] =
  await Promise.all([
    pathExists(join(pluginPath, 'commands')),
    pathExists(join(pluginPath, 'agents')),
    pathExists(join(pluginPath, 'skills')),
  ])

// 并行验证路径列表
const checks = await Promise.all(
  relPaths.map(async relPath => {
    const fullPath = join(pluginPath, relPath)
    return { relPath, fullPath, exists: await pathExists(fullPath) }
  })
)
// 结果按原始顺序处理，保持错误/日志顺序确定性
for (const { relPath, fullPath, exists } of checks) { ... }
```

### 13.4 原子性注册 (Atomic Swap)

```typescript
// 问题: 钩子重新加载时，不能有中间的空状态
// 解决: 先清除再注册的原子操作

clearRegisteredPluginHooks()      // 清除所有旧钩子
registerHookCallbacks(allPluginHooks)  // 注册所有新钩子
// 两步之间没有其他操作可以插入
```

### 13.5 Fail-Closed 安全策略

```typescript
// 问题: 企业策略配置损坏时，是默认放行还是拒绝？
// 解决: Fail-Closed——无法验证 = 拒绝

const hasEnterprisePolicy = strictAllowlist !== null || blocklist?.length > 0

if (!marketplaceConfig && hasEnterprisePolicy) {
  // 无法验证来源 + 策略已配置 → 阻止
  errors.push({
    type: 'marketplace-blocked-by-policy',
    blockedByBlocklist: strictAllowlist === null,
    // ...
  })
  return null  // 插件不加载
}
```

### 13.6 Discriminated Union 错误模式

```typescript
// 问题: 字符串匹配错误消息脆弱且不类型安全
// 解决: 24+ 种精确的可辨识联合类型

// 定义:
type PluginError =
  | { type: 'git-auth-failed'; authType: 'ssh' | 'https'; gitUrl: string }
  | { type: 'plugin-cache-miss'; installPath: string }
  // ...

// 使用:
function getPluginErrorMessage(error: PluginError): string {
  switch (error.type) {
    case 'git-auth-failed':
      return `Git authentication failed (${error.authType}): ${error.gitUrl}`
    // 编译器确保所有 case 都被处理
  }
}
```

### 13.7 插件设置白名单

```typescript
// 问题: 插件可能试图修改危险的系统设置
// 解决: 只允许白名单键

const PluginSettingsSchema = lazySchema(() =>
  SettingsSchema()
    .pick({
      agent: true,  // 当前只允许 agent 配置
    })
    .strip()  // 其他键静默移除
)
```

### 13.8 前向兼容的依赖格式

```typescript
// 问题: 未来可能添加版本约束，但旧客户端不能因此拒绝整个插件
// 解决: 接受但忽略未知格式

DependencyRefSchema = z.union([
  z.string()
    .regex(DEP_REF_REGEX)
    .transform(s => s.replace(/@\^[^@]*$/, '')),  // 剥离版本后缀
  z.object({
    name: z.string(),
    marketplace: z.string().optional(),
  })
    .loose()                                        // 允许未知字段
    .transform(o => `${o.name}@${o.marketplace}`),  // 标准化
])
```

### 13.9 遥测数据保护

```typescript
// 问题: 插件名称可能包含敏感信息
// 解决: 区分 PII-tagged 和 redacted 字段

logEvent('tengu_skill_tool_invocation', {
  // 通用仪表板用（已脱敏）
  command_name: isOfficialSkill ? commandName : 'custom',
  plugin_name: isOfficialSkill ? name : 'third-party',

  // 特权列（未脱敏，PII-tagged）
  _PROTO_skill_name: commandName,
  _PROTO_plugin_name: pluginManifest.name,
})
```

---

## 总结

Claude Code 的插件系统是一个精心设计的可扩展架构，其核心特点包括：

1. **声明式设计**: 通过 Markdown + JSON 定义能力，降低插件开发门槛
2. **多层安全**: 路径遍历防护、企业策略控制、权限白名单、Fail-Closed 原则
3. **性能优化**: 双模加载（cache-only vs full）、并行 I/O、延迟 Schema、memoize 缓存
4. **强类型安全**: Zod 运行时验证 + TypeScript 静态类型 + Discriminated Union 错误
5. **前向兼容**: Schema 静默剥离未知字段、依赖格式的向前兼容设计
6. **LLM 深度集成**: SkillTool 桥接、技能列表预算控制、Context Modifier、压缩恢复
7. **完整的生态**: Marketplace 分发、版本化缓存、企业治理、MCP/LSP 协议支持
8. **优秀的 DX**: 渐进式增强、自动目录发现、丰富的错误诊断、热重载支持
