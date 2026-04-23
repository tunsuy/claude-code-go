# Claude Code 工具权限系统深度分析

> 本文档详细分析了 Claude Code 项目中工具权限系统的设计理念、核心架构、技术实现以及在 LLM Loop 中的 Human-in-the-Loop (HIL) 集成方式。

---

## 一、宏观设计理念

### 1.1 核心哲学：安全优先的 Human-in-the-Loop (HIL)

Claude Code 的权限系统建立在一个核心理念之上：**AI 代理拥有执行系统命令和修改文件的能力，但这种能力必须受到严格的、可分层的、用户可控的权限约束**。其设计哲学可以概括为：

- **Fail-closed（默认拒绝）**：任何未明确授权的操作都需要询问用户
- **Defense-in-depth（纵深防御）**：多层安全检查层层递进，即使某层被绕过，后续层仍然有效
- **Progressive trust（渐进信任）**：从严格模式开始，用户可以逐步放宽权限
- **Least privilege（最小权限）**：每个工具只获得完成任务所需的最小权限

### 1.2 架构总览

权限系统是一个**分层决策管道**，从上到下大致分为：

```
┌────────────────────────────────────────────────────┐
│                  Permission Modes                    │  ← 全局策略层
│  (default / plan / acceptEdits / bypassPermissions / │
│   dontAsk / auto / bubble)                           │
├────────────────────────────────────────────────────┤
│              Permission Rules                        │  ← 规则匹配层
│  (allow / deny / ask) × (source: user/project/local/ │
│   policy/cli/command/session)                        │
├────────────────────────────────────────────────────┤
│         Tool-specific Permissions                    │  ← 工具自定义层
│  (BashTool, FileEditTool, MCP tools, etc.)          │
├────────────────────────────────────────────────────┤
│        Classifier / YOLO Classifier                  │  ← AI 辅助决策层
│  (Auto mode: AI 判断命令安全性)                       │
├────────────────────────────────────────────────────┤
│              Hooks System                            │  ← 扩展点层
│  (PreToolUse / PermissionRequest hooks)              │
├────────────────────────────────────────────────────┤
│         UI Permission Prompt                         │  ← 用户交互层
│  (Terminal 交互式确认 / SDK 回调)                     │
└────────────────────────────────────────────────────┘
```

---

## 二、核心类型系统

### 2.1 权限模式（PermissionMode）

```typescript
// src/types/permissions.ts
export const EXTERNAL_PERMISSION_MODES = [
  'acceptEdits',
  'bypassPermissions',
  'default',
  'dontAsk',
  'plan',
] as const

export type ExternalPermissionMode = (typeof EXTERNAL_PERMISSION_MODES)[number]

// 完整模式联合类型，包含内部模式
export type InternalPermissionMode = ExternalPermissionMode | 'auto' | 'bubble'
export type PermissionMode = InternalPermissionMode

// 运行时验证集：用户可寻址的模式
export const INTERNAL_PERMISSION_MODES = [
  ...EXTERNAL_PERMISSION_MODES,
  ...(feature('TRANSCRIPT_CLASSIFIER') ? (['auto'] as const) : ([] as const)),
] as const satisfies readonly PermissionMode[]
```

七种权限模式按信任等级从严到宽排列：

| 模式 | 描述 | 信任等级 |
|------|------|----------|
| **plan** | 只规划不执行，所有写操作被阻止 | 最严格 |
| **default** | 默认模式，每次危险操作都需用户确认 | 严格 |
| **acceptEdits** | 自动批准文件编辑，但 Bash 命令仍需确认 | 中等 |
| **dontAsk** | 不询问用户，自动拒绝需要权限的操作 | 自动拒绝 |
| **auto** | AI 分类器自动判断命令安全性（feature-gated） | 智能宽松 |
| **bypassPermissions** | 绕过所有权限检查 | 最宽松 |
| **bubble** | 子代理"冒泡"到父代理的权限上下文 | 特殊（继承） |

### 2.2 权限行为三元组（PermissionBehavior）

```typescript
// src/types/permissions.ts
export type PermissionBehavior = 'allow' | 'deny' | 'ask'
```

所有权限决策最终归结为三种行为：

- **allow**：直接放行
- **deny**：直接拒绝，向 LLM 返回错误信息
- **ask**：暂停 LLM Loop，向用户展示权限对话框

### 2.3 权限规则（PermissionRule）

```typescript
// src/types/permissions.ts
export type PermissionRuleValue = {
  toolName: string
  ruleContent?: string
}

export type PermissionRule = {
  source: PermissionRuleSource
  ruleBehavior: PermissionBehavior
  ruleValue: PermissionRuleValue
}
```

权限规则遵循 `Tool(content)` 的格式，例如：

- `Bash` → 匹配所有 Bash 命令
- `Bash(git *)` → 匹配 git 前缀的 Bash 命令
- `FileEdit` → 匹配所有文件编辑
- `mcp__server1__tool1` → 匹配特定 MCP 工具
- `mcp__server1` → 匹配 server1 的所有工具

### 2.4 规则来源的优先级体系（PermissionRuleSource）

```typescript
// src/types/permissions.ts
export type PermissionRuleSource =
  | 'userSettings'      // ~/.claude/settings.json (用户全局)
  | 'projectSettings'   // .claude/settings.json (项目级)
  | 'localSettings'     // .claude/settings.local.json (本地覆盖)
  | 'flagSettings'      // 远程 feature flag
  | 'policySettings'    // 企业策略管理
  | 'cliArg'           // CLI 命令行参数
  | 'command'          // 通过 /permissions 命令临时设置
  | 'session'          // 会话级临时授权
```

这是一个**多层配置合并系统**，类似于 Git 的配置层级，企业可以通过 `policySettings` 强制所有用户遵守安全策略。

### 2.5 权限决策结果（PermissionDecision）

```typescript
// src/types/permissions.ts
export type PermissionDecision<Input> =
  | PermissionAllowDecision<Input>   // 放行，可携带 updatedInput
  | PermissionAskDecision<Input>     // 需要询问用户，携带建议规则
  | PermissionDenyDecision           // 拒绝，携带原因

export type PermissionResult<Input> =
  | PermissionDecision<Input>
  | { behavior: 'passthrough'; ... }  // 透传到下一层检查
```

关键设计点：

- `PermissionAllowDecision` 可以携带 `updatedInput`，允许权限层**修改工具输入**（如路径重写）
- `PermissionAskDecision` 可以携带 `suggestions`（建议的规则更新），让用户在确认时可以选择"始终允许"
- `pendingClassifierCheck` 支持**异步分类器**在用户思考时并行运行

---

## 三、权限检查管道（核心流程）

### 3.1 Tool 接口中的权限钩子

每个 Tool 必须实现以下权限相关方法：

```typescript
// src/Tool.ts
interface Tool {
  // 输入验证 - 在权限检查之前
  validateInput?(input, context): Promise<ValidationResult>

  // 权限检查 - 工具特定的权限逻辑
  checkPermissions(input, context): Promise<PermissionResult>
}
```

`buildTool` 提供了默认实现：

```typescript
// src/Tool.ts
const TOOL_DEFAULTS = {
  // ...
  checkPermissions: (input, _ctx) =>
    Promise.resolve({ behavior: 'allow', updatedInput: input }),
  // ...
}
```

默认 `checkPermissions` 返回 `allow`，意味着简单工具（如 Read）默认放行，安全敏感工具（如 Bash、FileEdit）必须覆盖此方法。

### 3.2 完整的权限检查链

在 `toolExecution.ts` 的 `checkPermissionsAndCallTool` 函数中，完整的检查链如下：

```
1. Zod Schema 验证 (parsedInput = tool.inputSchema.safeParse)
   ↓ 失败则返回 InputValidationError
2. 工具自定义验证 (tool.validateInput)
   ↓ 失败则返回工具特定错误
3. 投机性分类器启动 (startSpeculativeClassifierCheck)
   ↓ 仅 BashTool，并行运行不阻塞
4. PreToolUse Hooks 执行
   ↓ Hooks 可以 allow/deny/修改输入
5. 工具权限检查 (tool.checkPermissions)
   ↓ 返回 allow/deny/ask/passthrough
6. 通用权限系统 (hasPermissionsToUseTool)
   ↓ 模式检查 + 规则匹配 + 分类器
7. canUseTool 回调 (UI层/SDK层)
   ↓ 对 ask 结果展示交互式确认
8. PermissionRequest Hooks
   ↓ 在用户看到对话框前/后运行
9. 用户决策 或 Auto 模式分类器决策
   ↓ 
10. PostToolUse Hooks 执行
```

### 3.3 BashTool 权限检查（最复杂的工具权限）

Bash 工具的权限检查是整个系统中最复杂的部分，因为 shell 命令的安全风险最高：

```typescript
// src/tools/BashTool/bashPermissions.ts
import { checkSemantics, parseForSecurityFromAst } from '../../utils/bash/ast.js'
import { getCommandSubcommandPrefix, splitCommand_DEPRECATED } from '../../utils/bash/commands.js'
import { classifyBashCommand } from '../../utils/permissions/bashClassifier.js'
import { matchWildcardPattern, parsePermissionRule } from '../../utils/permissions/shellRuleMatching.js'
```

Bash 权限检查的关键步骤：

1. **AST 解析**：使用 bash parser 将命令解析为 AST，分析命令结构
2. **前缀匹配**：提取命令前缀（如 `git`、`npm`），与规则做通配符匹配
3. **危险模式检测**：检查是否包含 `python *`、`eval`、`sudo` 等危险模式
4. **子命令拆分**：对复合命令（`&&`、`||`、`;`）分别检查每个子命令
5. **沙箱判定**：决定是否在沙箱中运行
6. **分类器辅助**：Auto 模式下使用 AI 分类器判断安全性

---

## 四、ToolPermissionContext —— 权限决策的上下文

### 4.1 结构定义

```typescript
// src/Tool.ts
export type ToolPermissionContext = DeepImmutable<{
  mode: PermissionMode                                    // 当前权限模式
  additionalWorkingDirectories: Map<string, ...>          // 额外允许的工作目录
  alwaysAllowRules: ToolPermissionRulesBySource           // 始终允许的规则集
  alwaysDenyRules: ToolPermissionRulesBySource             // 始终拒绝的规则集
  alwaysAskRules: ToolPermissionRulesBySource              // 始终询问的规则集
  isBypassPermissionsModeAvailable: boolean                // 是否允许 bypass 模式
  strippedDangerousRules?: ToolPermissionRulesBySource     // Auto 模式下被剥离的危险规则
  shouldAvoidPermissionPrompts?: boolean                   // 后台 Agent 不弹窗
  awaitAutomatedChecksBeforeDialog?: boolean               // Coordinator Worker 等待自动检查
  prePlanMode?: PermissionMode                            // Plan 模式前的原始模式
}>
```

关键设计点：

- 使用 `DeepImmutable` 确保权限上下文**不可变**，只能通过 `applyPermissionUpdate` 生成新副本
- `alwaysAllowRules`、`alwaysDenyRules`、`alwaysAskRules` 按来源分组存储，方便追踪每条规则的出处
- `shouldAvoidPermissionPrompts` 用于后台无 UI 的 Agent，自动拒绝而非挂起

### 4.2 权限上下文的初始化

在 `permissionSetup.ts` 中，权限上下文的构建过程：

1. 从所有配置源加载规则（`loadAllPermissionRulesFromDisk`）
2. 根据当前模式过滤规则
3. **Auto 模式下剥离危险规则**（如 `Bash(python:*)`）
4. 合并 CLI 参数和会话规则
5. 冻结为不可变对象

### 4.3 危险规则检测与剥离

```typescript
// src/utils/permissions/permissionSetup.ts
export function isDangerousBashPermission(
  toolName: string,
  ruleContent: string | undefined,
): boolean {
  // Tool-level allow (Bash with no content) - allows ALL commands
  if (ruleContent === undefined || ruleContent === '') return true
  // Standalone wildcard (*) matches everything
  if (content === '*') return true
  // Check for dangerous patterns with prefix syntax
  for (const pattern of DANGEROUS_BASH_PATTERNS) {
    if (content === `${lowerPattern}:*`) return true    // python:*
    if (content === `${lowerPattern}*`) return true     // python*
    if (content === `${lowerPattern} *`) return true    // python *
  }
  return false
}
```

**设计亮点**：Auto 模式在启用时会自动检测并**暂时剥离**那些过于宽泛的 allow 规则（如允许所有 python 命令），这些规则存储在 `strippedDangerousRules` 中以便模式退出时恢复。这防止了用户在 default 模式下配置的宽松规则在 Auto 模式下造成安全风险。

---

## 五、YOLO/Auto 模式分类器

### 5.1 工作原理

Auto 模式是 Claude Code 最具创新性的权限特性。它使用一个独立的 AI 分类器来判断 LLM 产生的工具调用是否安全：

```typescript
// src/utils/permissions/yoloClassifier.ts
const BASE_PROMPT: string = feature('TRANSCRIPT_CLASSIFIER')
  ? txtRequire(require('./yolo-classifier-prompts/auto_mode_system_prompt.txt'))
  : ''
```

分类器工作流程：

1. 将**当前对话上下文**（精简版）+ **工具调用内容**发送给分类器模型
2. 分类器参考用户自定义的 allow/deny 规则进行判断
3. 返回 `{ shouldBlock: boolean, reason: string }` 决策
4. 决策结果附带 `confidence` 级别（high/medium/low）

### 5.2 两阶段分类器

```typescript
// src/types/permissions.ts
export type YoloClassifierResult = {
  thinking?: string
  shouldBlock: boolean
  reason: string
  stage?: 'fast' | 'thinking'        // 两阶段：快速 + 深度思考
  stage1Usage?: ClassifierUsage       // 第一阶段的 token 使用
  stage2Usage?: ClassifierUsage       // 第二阶段的 token 使用
}
```

两阶段设计：

- **Fast stage**：快速判断明显安全/危险的命令
- **Thinking stage**：对边界情况使用更深入的推理

### 5.3 投机性分类器（Speculative Classifier）

投机性分类器是一个**性能优化设计**：在权限检查管道开始时（步骤3），系统就**预先启动**分类器的判断。当管道走到需要分类器结果时（步骤6-9），分类器可能已经完成了计算。

```
时间线：
  ┌─ 启动投机性分类器 ──────────────────────── 分类器完成 ─┐
  │                                                          │
  └─ Schema验证 → validateInput → Hooks → checkPermissions ──┘
                                                     ↑
                                              此时直接获取结果，无需等待
```

这种设计类似 CPU 的分支预测，大幅减少了 Auto 模式下的感知延迟。

### 5.4 拒绝追踪与降级机制

```typescript
// src/utils/permissions/denialTracking.ts
export const DENIAL_LIMITS = {
  maxConsecutive: 3,   // 连续拒绝 3 次
  maxTotal: 20,        // 总共拒绝 20 次
} as const

export function shouldFallbackToPrompting(state: DenialTrackingState): boolean {
  return (
    state.consecutiveDenials >= DENIAL_LIMITS.maxConsecutive ||
    state.totalDenials >= DENIAL_LIMITS.maxTotal
  )
}
```

**设计亮点**：当分类器连续拒绝 3 次或总计拒绝 20 次时，系统**自动降级回用户交互模式**。这防止了分类器过于保守导致 Agent 完全无法工作。

---

## 六、在 LLM Loop 中的 HIL 集成

### 6.1 Query Loop 架构

```typescript
// src/query.ts
export async function* query(params: QueryParams): AsyncGenerator<...> {
  // query 是一个 AsyncGenerator，每次 yield 一个事件
  // 关键参数：canUseTool（HIL 回调）
  const terminal = yield* queryLoop(params, consumedCommandUuids)
  return terminal
}
```

**LLM Loop 的核心循环**：

```
while (true) {
  1. 构建 System Prompt + Messages → 发送给 LLM API
  2. 流式接收 LLM 响应（yield StreamEvent）
  3. 如果响应包含 tool_use blocks：
     for each toolUse in toolUseBlocks:
       a. 查找工具定义
       b. 调用 runToolUse() → streamedCheckPermissionsAndCallTool()
       c. ★ 权限检查：checkPermissions → canUseTool ★
       d. 如果 ask → 暂停循环，等待用户决策
       e. 如果 allow → 执行工具
       f. 如果 deny → 返回错误信息给 LLM
     将 tool_result 追加到 messages
     continue (让 LLM 看到工具结果继续推理)
  4. 如果响应只有文本 → 结束循环，返回给用户
}
```

### 6.2 canUseTool —— HIL 的桥接点

`canUseTool` 是一个由上层 UI 注入的回调函数，它是 **LLM Loop 与 UI 层之间的桥梁**：

在交互式 REPL 中，`canUseTool` 的实现大致如下：

1. 接收工具的 `PermissionResult`
2. 如果 `behavior === 'allow'`，直接返回
3. 如果 `behavior === 'deny'`，直接返回拒绝
4. 如果 `behavior === 'ask'`：
   - 将权限请求推入 **PermissionConfirmQueue**
   - 渲染对应的 **PermissionRequest** 组件（如 BashPermissionRequest、FileEditPermissionRequest）
   - **暂停 LLM Loop**（通过 Promise 阻塞）
   - 等待用户操作（接受/拒绝/修改/始终允许）
   - 用户决策后 resolve Promise，LLM Loop 继续

### 6.3 PermissionContext —— 交互式权限处理

```typescript
// src/hooks/toolPermission/PermissionContext.ts
function createPermissionContext(tool, input, toolUseContext, ...) {
  return Object.freeze({
    // 记录权限决策（用于遥测和调试）
    logDecision(args, opts?) { ... },
    
    // 持久化权限更新（"始终允许"功能）
    async persistPermissions(updates) {
      persistPermissionUpdates(updates)
      setToolPermissionContext(applyPermissionUpdates(..., updates))
    },
    
    // 异步分类器自动批准
    async tryClassifier(pendingCheck, updatedInput) {
      const decision = await awaitClassifierAutoApproval(...)
      return { behavior: 'allow', ... }
    },
    
    // 运行 PermissionRequest hooks
    async runHooks(permissionMode, suggestions, ...) {
      for await (const hookResult of executePermissionRequestHooks(...)) {
        if (hookResult.permissionRequestResult?.behavior === 'allow') {
          return this.handleHookAllow(...)
        }
      }
    },
    
    // 用户确认处理
    async handleUserAllow(updatedInput, permissionUpdates, feedback?) {
      await this.persistPermissions(permissionUpdates)
      this.logDecision({ decision: 'accept', source: { type: 'user' } })
      return { behavior: 'allow', updatedInput, ... }
    },
    
    // 权限队列操作（React state 桥接）
    pushToQueue(item) { queueOps?.push(item) },
    removeFromQueue() { queueOps?.remove(toolUseID) },
  })
}
```

### 6.4 并发权限请求处理

当 LLM 在一次响应中调用多个工具时，多个权限请求会**并发**出现：

```
LLM Response: [tool_use: Bash("git add ."), tool_use: FileEdit("config.json")]
                    ↓                              ↓
           BashPermissionRequest          FileEditPermissionRequest
                    ↓                              ↓
         ┌──────────────────────────────────────────────┐
         │         PermissionConfirmQueue               │
         │  [BashRequest, FileEditRequest]              │
         │         ↓ 逐个展示给用户                      │
         └──────────────────────────────────────────────┘
```

### 6.5 SDK/非交互式模式的 HIL

对于 SDK（编程式调用）和非交互式模式（如 `--print`），HIL 的处理方式不同：

- **SDK 模式**：`canUseTool` 通过 `structuredIO` 或 `remotePermissionBridge` 将权限请求序列化后发送给 SDK 宿主
- **非交互式模式**：`shouldAvoidPermissionPrompts = true`，所有 `ask` 决策自动转为 `deny`
- **Swarm 模式**：worker 的权限请求通过 `permissionSync` 冒泡到 leader 处理

---

## 七、规则匹配引擎

### 7.1 规则解析器

```typescript
// src/utils/permissions/permissionRuleParser.ts
export function escapeRuleContent(content: string): string {
  return content
    .replace(/\\/g, '\\\\')   // 转义反斜杠
    .replace(/\(/g, '\\(')    // 转义左括号
    .replace(/\)/g, '\\)')    // 转义右括号
}
```

规则格式：`Tool(content)`，其中 content 支持通配符 `*` 和前缀匹配 `prefix:*`。

### 7.2 多层规则匹配

```typescript
// src/utils/permissions/permissions.ts
function toolMatchesRule(tool, rule): boolean {
  // 1. 工具级匹配：规则无 content → 匹配整个工具
  if (rule.ruleValue.ruleContent === undefined) {
    return rule.ruleValue.toolName === nameForRuleMatch
  }
  // 2. MCP server 级匹配：mcp__server1 匹配 mcp__server1__*
  const ruleInfo = mcpInfoFromString(rule.ruleValue.toolName)
  const toolInfo = mcpInfoFromString(nameForRuleMatch)
  return ruleInfo?.serverName === toolInfo?.serverName
}
```

匹配优先级：**deny > ask > allow > mode default**

### 7.3 Shell 规则匹配（通配符引擎）

对于 Bash/PowerShell 工具，规则匹配使用专门的 `shellRuleMatching.ts`：

- `git *` → 匹配以 `git ` 开头的所有命令
- `npm test` → 精确匹配 `npm test`
- `docker compose *` → 匹配 docker compose 子命令
- 支持 `prefix:*` 风格和 glob 通配符

---

## 八、文件系统权限

```typescript
// src/utils/permissions/filesystem.ts
export const DANGEROUS_FILES = [
  '.gitconfig', '.gitmodules',
  '.bashrc', '.bash_profile', '.zshrc', '.zprofile', '.profile',
  '.ripgreprc', '.mcp.json', '.claude.json',
] as const

export const DANGEROUS_DIRECTORIES = [
  '.git', '.vscode', '.idea', '.claude',
] as const
```

文件系统权限检查：

1. **路径验证**：确保路径在允许的工作目录范围内
2. **危险文件保护**：`.gitconfig`、`.bashrc` 等配置文件在 Auto 模式下需要额外确认
3. **路径遍历防护**：检测 `../` 攻击
4. **额外目录管理**：用户可以通过 `/add-dir` 命令添加工作目录

---

## 九、Hooks 扩展机制

权限系统在多个关键点提供了 Hook 扩展：

1. **PreToolUse Hook**：工具执行前触发，可以 allow/deny/修改输入
2. **PermissionRequest Hook**：权限弹窗显示前/后触发，可以自动批准
3. **PostToolUse Hook**：工具执行后触发，用于日志/审计
4. **PermissionDenied Hook**：权限被拒绝时触发

Hook 配置在 `.claude/settings.json` 中：

```json
{
  "hooks": {
    "PreToolUse": [
      { "if": "Bash(git *)", "command": "echo 'Git command detected'" }
    ]
  }
}
```

### 9.1 Hook 在权限管道中的位置

```
                    ┌─────────────────────┐
                    │   PreToolUse Hook   │ ← 可提前 allow/deny
                    └──────────┬──────────┘
                               ↓
                    ┌─────────────────────┐
                    │  tool.checkPermissions │
                    └──────────┬──────────┘
                               ↓
                    ┌─────────────────────┐
                    │ hasPermissionsToUseTool │ ← 规则+模式检查
                    └──────────┬──────────┘
                               ↓ (如果 ask)
                    ┌─────────────────────┐
                    │ PermissionRequest Hook │ ← 可自动批准
                    └──────────┬──────────┘
                               ↓ (如果仍需询问)
                    ┌─────────────────────┐
                    │   用户交互确认      │
                    └──────────┬──────────┘
                               ↓
                    ┌─────────────────────┐
                    │  PostToolUse Hook   │ ← 执行后审计
                    └─────────────────────┘
```

---

## 十、权限持久化与同步

### 10.1 权限更新操作

```typescript
// src/types/permissions.ts
export type PermissionUpdate =
  | { type: 'addRules'; destination; rules; behavior }     // 添加规则
  | { type: 'replaceRules'; destination; rules; behavior }  // 替换规则
  | { type: 'removeRules'; destination; rules; behavior }   // 移除规则
  | { type: 'setMode'; destination; mode }                  // 设置模式
  | { type: 'addDirectories'; destination; directories }    // 添加目录
  | { type: 'removeDirectories'; destination; directories } // 移除目录
```

### 10.2 持久化目标

| 目标 | 文件位置 | 说明 |
|------|----------|------|
| **session** | 内存中 | 仅当前会话有效，重启后消失 |
| **localSettings** | `.claude/settings.local.json` | 不进版本控制 |
| **projectSettings** | `.claude/settings.json` | 可进版本控制 |
| **userSettings** | `~/.claude/settings.json` | 全局生效 |

### 10.3 "始终允许" 工作流

当用户在权限对话框中选择"Always allow"时：

```typescript
// src/hooks/toolPermission/PermissionContext.ts
async handleUserAllow(updatedInput, permissionUpdates, feedback?) {
  // 1. 持久化规则到磁盘
  const acceptedPermanentUpdates = await this.persistPermissions(permissionUpdates)
  // 2. 更新内存中的权限上下文
  setToolPermissionContext(applyPermissionUpdates(..., updates))
  // 3. 记录遥测
  this.logDecision({
    decision: 'accept',
    source: { type: 'user', permanent: acceptedPermanentUpdates },
  })
}
```

---

## 十一、安全防护机制汇总

### 11.1 多层防御

| 防护层 | 机制 | 防护目标 |
|--------|------|----------|
| 输入验证 | Zod Schema + validateInput | 防止畸形输入 |
| 危险模式检测 | DANGEROUS_BASH_PATTERNS | 防止任意代码执行 |
| 路径验证 | pathValidation + filesystem | 防止路径遍历 |
| 沙箱隔离 | SandboxManager | 限制文件系统访问 |
| 分类器评估 | YOLO Classifier | AI 辅助安全判断 |
| 用户确认 | Permission Prompt | 人工决策兜底 |
| 降级保护 | denialTracking | 防止分类器故障卡死 |
| 企业策略 | policySettings | 组织级安全合规 |
| Killswitch | bypassPermissionsKillswitch | 远程紧急关闭 bypass 模式 |

### 11.2 特殊安全措施

1. **_simulatedSedEdit 防护**：防止 LLM 注入内部字段绕过权限
2. **Legacy 工具名映射**：旧名称（如 `Task` → `Agent`）自动映射，防止通过旧名称绕过规则
3. **规则遮蔽检测**：`shadowedRuleDetection.ts` 检测冗余/冲突的规则
4. **MCP 工具命名空间隔离**：MCP 工具使用 `mcp__server__tool` 前缀，防止与内置工具冲突

---

## 十二、核心文件索引

| 文件路径 | 功能描述 |
|----------|----------|
| `src/types/permissions.ts` | 权限类型系统定义 |
| `src/Tool.ts` | 工具接口与权限钩子 |
| `src/utils/permissions/permissions.ts` | 核心权限检查逻辑 |
| `src/utils/permissions/permissionSetup.ts` | 权限上下文初始化 |
| `src/utils/permissions/yoloClassifier.ts` | Auto 模式 AI 分类器 |
| `src/utils/permissions/dangerousPatterns.ts` | 危险命令模式定义 |
| `src/utils/permissions/denialTracking.ts` | 拒绝追踪与降级 |
| `src/utils/permissions/filesystem.ts` | 文件系统权限检查 |
| `src/utils/permissions/shellRuleMatching.ts` | Shell 命令规则匹配 |
| `src/utils/permissions/permissionRuleParser.ts` | 规则解析器 |
| `src/utils/permissions/shadowedRuleDetection.ts` | 规则冲突检测 |
| `src/utils/permissions/PermissionUpdate.ts` | 权限更新操作 |
| `src/services/tools/toolExecution.ts` | 工具执行与权限管道 |
| `src/hooks/toolPermission/PermissionContext.ts` | 交互式权限上下文 |
| `src/tools/BashTool/bashPermissions.ts` | Bash 工具权限 |
| `src/query.ts` | LLM Loop 主循环 |

---

## 十三、总结

Claude Code 的工具权限系统是一个**企业级的、可观测的、可扩展的安全框架**。其核心技术亮点包括：

1. **声明式规则引擎**：通过 `Tool(content)` 格式的规则，用户可以精细控制每个工具的每种操作
2. **AI-in-the-Loop 安全**：Auto 模式用 AI 分类器替代人工确认，在效率和安全之间取得平衡
3. **不可变状态管理**：权限上下文使用 `DeepImmutable` + 函数式更新，避免状态竞争
4. **渐进信任模型**：从严格到宽松的多级模式，配合"始终允许"的渐进授权
5. **优雅降级**：分类器故障时自动降级回人工确认模式
6. **可扩展的 Hook 系统**：企业可以通过 Hooks 注入自定义安全检查
7. **异步非阻塞设计**：分类器与 Hook 并行运行，不影响用户响应速度
8. **投机性分类器**：类似 CPU 分支预测的优化，提前启动分类器减少延迟

这套系统实现了 AI Agent 领域一个关键问题的优秀解法：**如何让 AI 高效地执行系统操作，同时确保人类始终保持控制权**。
