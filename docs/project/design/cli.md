# CLI 入口层详细设计

> 负责 Agent：Agent-CLI
> 状态：设计中
> 日期：2026-04-02

---

## 1. 命令树设计

基于原始 TS 实现（`src/entrypoints/cli.tsx` + `src/main.tsx`），使用 cobra 对应如下命令树：

```
claude
├── [root]                    交互式 REPL（默认，无子命令时进入）
│   ├── -p / --print          非交互式单次输出模式
│   ├── -c / --continue       恢复当前目录最近一次会话
│   └── -r / --resume [id]    恢复指定会话 ID，或弹出选择器
│
├── mcp                       配置和管理 MCP 服务器
│   ├── serve                 以 MCP 协议启动 Claude Code 服务器
│   ├── add <name>            添加 MCP 服务器
│   ├── remove <name>         删除 MCP 服务器
│   ├── list                  列出已配置的 MCP 服务器
│   ├── get <name>            查看 MCP 服务器详情
│   ├── add-json <name> <j>   以 JSON 字符串添加服务器
│   ├── add-from-claude-desktop 从 Claude Desktop 导入
│   └── reset-project-choices 重置项目 MCP 服务器选择记录
│
├── auth                      管理身份认证
│   ├── login                 登录 Anthropic 账户
│   ├── logout                登出
│   └── status                显示认证状态
│
├── plugin  (alias: plugins)  管理 Claude Code 插件
│   ├── list                  列出已安装插件
│   ├── install <plugin>      安装插件
│   ├── uninstall <plugin>    卸载插件
│   ├── enable <plugin>       启用插件
│   ├── disable [plugin]      禁用插件
│   ├── update <plugin>       更新插件
│   ├── validate <path>       校验插件/Marketplace manifest
│   └── marketplace           管理插件市场
│       ├── add <source>      添加市场
│       ├── list              列出已配置市场
│       ├── remove <name>     移除市场
│       └── update [name]     更新市场
│
├── agents                    列出已配置的 Agent
├── doctor                    健康检查（自动更新器、MCP 服务器等）
├── update  (alias: upgrade)  检查并安装更新
├── install [target]          安装 Claude Code 本地构建版本
└── [内部/实验性子命令，Go 实现阶段暂不纳入]
    ├── server                启动 Claude Code Session Server（HTTP/Unix）
    ├── ssh <host>            通过 SSH 在远程主机运行（BRIDGE 特性）
    ├── open <cc-url>         连接 Claude Code Server（cc:// URL）
    ├── setup-token           设置长效认证 Token
    ├── completion <shell>    生成 Shell 补全脚本
    ├── ps / logs / attach / kill  后台会话管理（BG_SESSIONS 特性）
    └── remote-control (rc)   远程控制模式（BRIDGE_MODE 特性）
```

> **设计原则**：`-p / --print` 是根命令的 Flag 而非子命令，进入非交互路径后不注册多数交互型子命令（参考原 TS 的 ~65ms 优化：`-p` 模式跳过 52 个子命令注册）。

---

## 2. cmd/claude/main.go

### 职责

`cmd/claude/main.go` 是整个程序的唯一入口，极简，只做三件事：

1. **快速路径拦截**：在加载完整 CLI 之前处理 `--version`、`--dump-system-prompt` 等零依赖快路径，最小化模块加载开销。
2. **启动 Bootstrap**：调用 `internal/bootstrap.Run()` 完成初始化和依赖装配。
3. **执行 Cobra 根命令**：将控制权交给 cobra 的 `rootCmd.Execute()`。

```go
// cmd/claude/main.go
package main

import (
    "os"
    "github.com/your-org/claude-code-go/internal/bootstrap"
)

func main() {
    // 1. 零依赖快速路径（--version 等）
    if handled := bootstrap.HandleFastPath(os.Args); handled {
        return
    }

    // 2. 初始化引导 + 执行 cobra 根命令
    if err := bootstrap.Run(os.Args); err != nil {
        os.Exit(1)
    }
}
```

---

## 3. internal/bootstrap — 启动引导

### 3.1 初始化顺序

对应原 TS 的 `src/entrypoints/init.ts` 中 `init()` 的执行链路，Go 实现保持相同顺序：

```
阶段 0  快速路径检测（--version / --dump-system-prompt / daemon-worker 等）
   │
阶段 1  配置系统初始化
   │    enableConfigs()          → 解析 ~/.claude/settings.json + 项目 .claude/settings.json
   │    applySafeEnvVars()       → 应用安全的环境变量（不涉及 trust 的）
   │    applyExtraCACerts()      → 在第一次 TLS 握手前注入自定义 CA 证书
   │
阶段 2  运行时安全与网络
   │    setupGracefulShutdown()  → 注册退出清理回调
   │    configureGlobalMTLS()    → 配置 mTLS
   │    configureGlobalProxy()   → 配置 HTTP 代理
   │
阶段 3  认证预热（并行）
   │    prefetchKeychainCreds()  → 并行触发 Keychain 读取（macOS）
   │    prefetchMDMSettings()    → 并行触发 MDM 子进程（plutil/reg query）
   │    populateOAuthAccountInfo()
   │
阶段 4  特性/策略加载（并行）
   │    initRemoteManagedSettingsPromise()
   │    initPolicyLimitsPromise()
   │
阶段 5  数据迁移
   │    runMigrations()          → 执行增量 migration（模型别名、配置结构等）
   │
阶段 6  服务层初始化（延迟到 REPL 首次渲染后）
         initUser() / getUserContext() / prefetchSystemContext()
         initGrowthBook() / refreshModelCapabilities()
         settingsChangeDetector / skillChangeDetector
```

### 3.2 依赖注入设计

采用**构造函数注入** + **Wire 风格 Provider** 模式，避免全局单例（与原 TS 的模块级 mutable state 做法相反）：

```
bootstrap.AppContainer
 ├── ConfigStore          → 持有已解析的配置（settings.json 合并结果）
 ├── StateStore           → 会话级可变状态（cwd、sessionId、cost 等）
 ├── APIClient            → Anthropic API HTTP Client（注入 auth + retry）
 ├── MCPClient            → MCP 服务器连接池
 ├── QueryEngine          → 核心推理引擎（依赖 APIClient、Tools）
 ├── ToolRegistry         → 工具注册表（Bash、Edit、Read 等）
 └── TUIProgram           → Bubble Tea TUI 程序（依赖 QueryEngine、StateStore）
```

各模块实例化流程：

```go
// internal/bootstrap/wire.go（或手写 provider）
func BuildContainer(cfg *config.Config) (*AppContainer, error) {
    state    := state.NewStore(cfg)
    auth     := auth.NewProvider(cfg)
    apiClient := api.NewClient(cfg, auth)          // Agent-Services 层
    tools    := tools.NewRegistry(cfg)
    engine   := core.NewQueryEngine(apiClient, tools, state)  // Agent-Core 层
    tui      := tui.NewProgram(engine, state)                 // Agent-TUI 层
    return &AppContainer{...}, nil
}
```

### 3.3 各运行模式的启动路径

| 入口判断条件 | 路径 |
|---|---|
| `args[0]` 是 `mcp serve` | → `mcpServeHandler()`，以 MCP 协议对外提供服务 |
| Flag `-p` / `--print` 存在 | → `runHeadless()`，跳过 TUI 初始化，直接走 QueryEngine |
| Flag `--resume` 或 `-c` | → `loadConversation()` + 标准 REPL |
| 无特殊标志 | → 标准交互式 REPL，启动 Bubble Tea TUI |
| 子命令 `doctor` | → `doctorHandler()`，不初始化 QueryEngine |
| 子命令 `auth *` | → `authHandler()`，最小化初始化 |
| `--version` / `-v` | → 零依赖快路径，直接打印版本退出 |

---

## 4. 运行模式分发

### 4.1 交互式 REPL 模式

**触发条件**：无 `-p`，无 `mcp serve`，无 `doctor` 等独立子命令。

**启动流程**：
```
main() → bootstrap.Run()
  → BuildContainer()        # 构建完整依赖图
  → showSetupScreensIfNeeded()  # 首次运行时显示 trust 对话框
  → initializeTelemetryAfterTrust()  # trust 确认后初始化遥测
  → applyConfigEnvironmentVariables()  # 应用完整环境变量
  → runMigrations()
  → tui.Program.Start()     # 启动 Bubble Tea 主循环
  → startDeferredPrefetches()  # 首次渲染后触发后台预热
```

**关键特性**：
- Cobra 在此模式下注册全部子命令（约 52 个交互型命令）
- 支持 `--resume`（按 session ID 恢复）、`--continue`（恢复最近会话）
- 首次启动展示 workspace trust 对话框，accept 后才执行 git 相关操作

### 4.2 非交互式模式（-p / --print）

**触发条件**：`-p` 或 `--print` flag 存在。

**特点**：
- 跳过全部子命令注册（性能优化，节省 ~65ms）
- 跳过 workspace trust 对话框
- 支持 `--output-format text|json|stream-json`
- 支持 `--max-turns`、`--max-budget-usd` 等限制参数
- 支持管道输入（stdin → prompt）
- SIGINT 注册自己的 handler，中止当前查询后优雅退出

**流程**：
```
main() → bootstrap.RunHeadless(prompt)
  → BuildMinimalContainer()  # 无 TUI 的精简容器
  → engine.Query(prompt)
  → writeOutput(format)      # text / json / stream-json
  → gracefulShutdown()
```

### 4.3 SDK 服务模式（mcp serve）

**触发条件**：子命令为 `mcp serve`。

**特点**：
- 通过 MCP 协议（stdio 或 SSE）将 Claude Code 能力暴露给外部
- 设置 `CLAUDE_CODE_ENTRYPOINT=mcp`
- 不启动 TUI，不进入 REPL
- 对外暴露工具调用接口

**流程**：
```
main() → mcpServeCmd.RunE()
  → bootstrap.RunMCPServer(opts)
  → mcpServer.Listen(stdio|sse)  # 监听 MCP 协议消息
  → dispatch(toolCall) → engine.Execute()
```

### 4.4 诊断模式（doctor）

**触发条件**：子命令为 `doctor`。

**特点**：
- 最小化初始化：不加载 QueryEngine、TUI
- 检查自动更新器状态、MCP 服务器健康、网络连通性
- 注意：跳过 workspace trust 对话框（会直接 spawn stdio MCP server 进行健康检查，需用户在可信目录执行）

**流程**：
```
main() → doctorCmd.RunE()
  → bootstrap.RunDiagnostics()
  → checkAutoUpdater()
  → checkMCPServers()       # 直接 spawn stdio MCP servers
  → checkNetworkConnectivity()
  → printDiagnosticReport()
```

---

## 5. 全局 Flag 设计

以下为根命令的全局 flag，对应 TS 中 `program.option(...)` / `program.addOption(...)` 的定义：

### 核心执行控制

| Flag | 简写 | 类型 | 说明 |
|---|---|---|---|
| `--print` | `-p` | bool | 非交互式输出模式 |
| `--continue` | `-c` | bool | 恢复当前目录最近会话 |
| `--resume [id]` | `-r` | string? | 恢复指定 session，或弹出选择器 |
| `--model <model>` | | string | 覆盖当前会话使用的模型（如 `sonnet`、`opus`） |
| `--output-format <fmt>` | | string | 输出格式：`text`\|`json`\|`stream-json`（仅 -p） |
| `--max-turns <n>` | | int | 最大 agentic 轮次（仅 -p） |
| `--max-budget-usd <n>` | | float | 最大 API 花费上限（仅 -p） |

### 权限控制

| Flag | 说明 |
|---|---|
| `--dangerously-skip-permissions` | 跳过所有权限检查（仅推荐沙箱环境） |
| `--permission-mode <mode>` | 权限模式：`default`\|`auto`\|`bypassPermissions` 等 |
| `--allowed-tools <tools...>` | 允许使用的工具白名单（如 `Bash(git:*) Edit`） |
| `--disallowed-tools <tools...>` | 工具黑名单 |
| `--tools <tools...>` | 指定可用工具集合 |

### 上下文与配置

| Flag | 说明 |
|---|---|
| `--system-prompt <prompt>` | 覆盖系统提示词 |
| `--append-system-prompt <prompt>` | 追加到默认系统提示词 |
| `--add-dir <dirs...>` | 额外允许工具访问的目录 |
| `--mcp-config <configs...>` | 从 JSON 文件或字符串加载 MCP 服务器 |
| `--settings <file-or-json>` | 加载额外 settings（文件路径或 JSON 字符串） |
| `--setting-sources <sources>` | 限制加载的 settings 来源（`user,project,local`） |
| `--agents <json>` | JSON 定义自定义 Agent |

### 会话管理

| Flag | 说明 |
|---|---|
| `--session-id <uuid>` | 指定会话 UUID（必须合法 UUID 格式） |
| `--name <name>` | `-n` 设置会话显示名称 |
| `--no-session-persistence` | 禁用会话持久化（仅 -p） |
| `--fork-session` | Resume 时创建新 session ID |

### 调试与诊断

| Flag | 说明 |
|---|---|
| `--debug` | 启用调试模式 |
| `--debug-to-stderr` | 调试输出到 stderr（隐藏） |
| `--debug-file <path>` | 调试日志写入指定文件 |
| `--verbose` | 覆盖 verbose 配置 |
| `--bare` | 极简模式：跳过 hooks、LSP、插件同步、后台预热等 |

### 模式扩展（交互式）

| Flag | 说明 |
|---|---|
| `--worktree [name]` | `-w` 为会话创建新 git worktree |
| `--tmux` | 为 worktree 创建 tmux session |
| `--effort <level>` | 努力级别：`low`\|`medium`\|`high`\|`max` |
| `--thinking <mode>` | 思维模式：`enabled`\|`adaptive`\|`disabled` |
| `--ide` | 自动连接到 IDE |
| `--fallback-model <model>` | 默认模型过载时的降级模型（仅 -p） |

---

## 6. 依赖所有层的 TODO 占位

```go
// internal/bootstrap/wire.go

// TODO(dep): 依赖 Agent-Core QueryEngine 接口
// 待 Agent-Core 完成 QueryEngine 接口定义后，在此处完成注入：
//   engine := core.NewQueryEngine(apiClient, toolRegistry, stateStore)
// 接口路径：internal/core/engine.go
var _ core.QueryEngine = (*bootstrap.queryEnginePlaceholder)(nil)

// TODO(dep): 依赖 Agent-TUI Program 接口
// 待 Agent-TUI 完成 Bubble Tea Program 接口定义后，在此处完成注入：
//   program := tui.NewProgram(engine, stateStore)
// 接口路径：internal/tui/program.go
var _ tui.Program = (*bootstrap.tuiProgramPlaceholder)(nil)

// TODO(dep): 依赖 Agent-Services API Client
// 待 Agent-Services 完成 Anthropic API Client 接口后，在此处完成注入：
//   apiClient := services.NewAnthropicClient(cfg.APIKey, cfg.BaseURL)
// 接口路径：internal/services/api/client.go
var _ services.APIClient = (*bootstrap.apiClientPlaceholder)(nil)

// TODO(dep): 依赖 Agent-Services MCP Client
// 待 Agent-Services 完成 MCP Client 后，在此处完成 mcpServe 路径：
//   mcpServer := services.NewMCPServer(toolRegistry)
// 接口路径：internal/services/mcp/server.go
var _ services.MCPServer = (*bootstrap.mcpServerPlaceholder)(nil)
```

---

## 7. 设计决策

### 7.1 快速路径（Fast-path）优先

**决策**：在 `main()` 最顶部，通过原始 `os.Args` 匹配 `--version`、`--dump-system-prompt`、`daemon-worker` 等特殊参数，**不经过 cobra 解析**直接处理后退出。

**原因**：原 TS 实现测量到完整 cobra/commander 初始化约耗时 65ms+，对 `--version` 这类高频轻量操作应零依赖加载。Go 的启动开销虽然远小于 Node.js，但保持同等分层策略有利于后续性能分析和 cold-start benchmark。

### 7.2 非交互模式跳过子命令注册

**决策**：`-p` 模式下，不向 cobra 注册 mcp/auth/plugin/doctor/update 等约 50 个子命令。

**原因**：同原 TS——子命令注册（包括 help text、option 解析元数据）本身有开销，非交互脚本调用只需要根命令的 flag 集合。Go 中实现方式：在 `buildRootCmd()` 时检查 `os.Args` 是否包含 `-p`，按条件分支决定是否 `rootCmd.AddCommand(subCmds...)`。

### 7.3 依赖注入而非全局单例

**决策**：所有跨层依赖（APIClient、QueryEngine、StateStore 等）通过 `AppContainer` 显式传递，不使用 `package` 级别的 `var` 全局变量。

**原因**：原 TS 实现大量使用模块级 mutable state（`bootstrap/state.ts` 中约 60+ 个字段），这在 Go 中会导致测试隔离困难和并发竞争风险。Go 的设计倾向于显式依赖，用 `context.Context` + 构造函数注入替代隐式全局状态。

### 7.4 Bootstrap 阶段分离（安全性考量）

**决策**：将初始化严格分为「trust 前」和「trust 后」两个阶段：
- **trust 前**：只执行无需信任目录的操作（加载 user-level settings、OAuth、CA 证书）
- **trust 后**：执行 git 操作（`getSystemContext`）、完整环境变量应用、MCP stdio server spawn

**原因**：git 配置文件（`core.fsmonitor`、`diff.external` 等）可执行任意代码，在用户确认 trust 对话框前执行会造成安全风险。此逻辑直接移植自原 TS 的 `prefetchSystemContextIfSafe()` + `checkHasTrustDialogAccepted()` 设计。

### 7.5 cobra 而非 commander-js

**决策**：使用 [spf13/cobra](https://github.com/spf13/cobra) 作为 CLI 框架。

**原因**：cobra 是 Go 生态中最成熟的 CLI 框架，与原 TS 的 `@commander-js/extra-typings` 功能对等（子命令、嵌套命令、persistent flags、help 定制），并天然支持 shell completion 生成（对应原 TS 的 `completion <shell>` 子命令）。

### 7.6 配置文件 entrypoint 标识

**决策**：启动时根据模式设置 `CLAUDE_CODE_ENTRYPOINT` 环境变量，值域：`cli`（交互式）、`sdk-cli`（非交互 -p）、`mcp`（MCP serve 模式）、`claude-code-github-action`（CI）。

**原因**：原 TS 中此变量被下游服务（QueryEngine、工具层）用于行为差异化（如是否展示权限对话框）。Go 实现沿用同一约定，通过 `StateStore.SetEntrypoint()` 存储并在需要处读取，避免直接读写 `os.Setenv`。
