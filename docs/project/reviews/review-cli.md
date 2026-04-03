# Tech Lead 评审：cli.md

> 评审人：Tech Lead
> 日期：2026-04-02
> 结论：**有条件通过（APPROVED_WITH_CHANGES）**

---

## 总体评估

CLI 设计扎实。6 阶段引导序列、4 种运行模式、Cobra 命令树和标志集均已正确指定，忠实对应 TS 的 `init.ts` / `cli.tsx` 架构。信任边界前/后划分和快速路径优化动机充分。所需修改较小，集中在引导和无头路径的缺失细节。

---

## 优点

1. **6 阶段引导顺序正确且完整** — 阶段 0（快速路径）→ 阶段 1（配置 + CA 证书）→ 阶段 2（优雅关闭 + mTLS + 代理）→ 阶段 3（并行预取认证）→ 阶段 4（并行加载特性/策略）→ 阶段 5（迁移）→ 阶段 6（首次渲染后延迟）。阶段 3 和 4 的并行注释对于启动延迟非常重要（§3.1）。

2. **信任边界位置正确** — Git 操作（`getSystemContext`）和 MCP stdio 服务器启动在工作区信任对话框确认后执行，与 TS 的 `prefetchSystemContextIfSafe()` + `checkHasTrustDialogAccepted()` 安全模型一致（§7.4）。这是关键安全属性。

3. **Cobra 前的快速路径 `os.Args` 拦截** — `bootstrap.HandleFastPath(os.Args)` 在任何模块加载之前处理 `--version`、`--dump-system-prompt`、`daemon-worker`，最小化冷启动延迟（§2、§7.1）。正确且惯用。

4. **`-p` 跳过约 50 个子命令注册** — `buildRootCmd()` 中对 `os.Args` 的预扫描以条件性添加子命令，保留了来自 TS 的约 65ms 优化。正确（§7.2）。

5. **`AppContainer` 依赖注入** — 所有七个字段（`ConfigStore`、`StateStore`、`APIClient`、`MCPClient`、`QueryEngine`、`ToolRegistry`、`TUIProgram`）均已显式声明。使用构造函数注入而非包级全局变量是正确的 Go 设计（§3.2、§7.3）。

6. **4 种运行模式清晰指定** — 交互式 REPL、无头 `-p`、MCP 服务和 doctor 各有独立的引导路径、初始化深度和入口函数。§3.3 中的表格清晰（§3.3、§4）。

7. **`CLAUDE_CODE_ENTRYPOINT` 约定完整保留** — 四个值（`cli`、`sdk-cli`、`mcp`、`claude-code-github-action`）均已枚举，设计正确地将值存储在 `StateStore` 而非 `os.Setenv`（§7.6）。

8. **`cobra` 选型正确** — `spf13/cobra` 是标准 Go CLI 框架，与 `commander-js` 功能对等（§7.5）。

9. **标志集完整** — 所有五个标志组（核心执行、权限控制、上下文/配置、会话管理、调试/诊断、模式扩展）均已枚举，包含 40+ 个标志。与 TS 的 `program.option(...)` 清单一致（§5）。

10. **`doctor` 模式最小初始化** — 正确跳过 `QueryEngine` 和 `TUI` 初始化。关于 doctor 模式跳过信任对话框（用于对 stdio MCP 服务器进行健康检查）的注释是一个重要设计决策，应予以突出记录（§4.4）。

---

## 问题

**【严重】无头模式的 `BuildMinimalContainer()` 未定义**
§4.2 引用 `RunHeadless(prompt)` 调用 `BuildMinimalContainer()`，但 §3.2 仅定义了 `BuildContainer()`。无头容器省略 TUI，但仍需要 `APIClient`、`QueryEngine`、`ToolRegistry` 和 `StateStore`。两种容器的差异必须记录在案。否则，实现者可能在无头构建中意外引入 TUI 依赖。

**【严重】阶段 6 延迟服务没有说明触发机制**
§3.1 指出阶段 6"延迟到 REPL 首次渲染后"，但未说明触发机制。在 BubbleTea 中，"首次渲染后"不是内置事件。请记录实现方式：(a) TUI 在第一次 `View()` 调用后发送 `FirstRenderMsg`，或 (b) 零时延 `tea.Tick` 在第一帧后触发。

**【严重】`mcpServeCmd` 路径未说明工具暴露策略**
§4.3 描述 MCP 服务模式将 `toolCall → engine.Execute()`，但未指定暴露哪些工具。MCP 服务模式是暴露所有已注册工具，还是受限子集？这是功能正确性问题：通过 MCP stdio 暴露 `AskUserQuestion` 将导致死锁。应使用 tools.md 中的 `ALL_AGENT_DISALLOWED_TOOLS` 列表来指导 MCP 服务的工具过滤。

**【次要】`runMigrations()` 在阶段 5 和 REPL 启动流程中重复出现**
§3.1 将 `runMigrations()` 列在阶段 5，但 §4.1 的 REPL 启动流程也调用了 `runMigrations()`。请澄清这是同一次调用还是第二次调用，若相同则从 §4.1 的流程中移除，明确说明已由阶段 5 覆盖。

**【次要】`-p` 模式的 `os.Args` 预扫描存在边界情况风险**
§7.2 描述在 Cobra 解析前检查 `os.Args` 中的 `-p`。字面的 `strings.Contains` 检查存在误判风险（例如包含 `-p` 作为路径组件的目录名，或嵌入在更长标志中的 `-p`）。应使用最小参数扫描器，在去掉二进制名称后仅在单词边界匹配 `-p` 或 `--print`。

**【次要】`--session-id <uuid>` 验证有注释但未指定**
§5 指出 UUID 必须"合法 UUID 格式"，但未指定接受哪个 UUID 版本或使用什么验证函数。请记录：接受任何有效 UUID（v1–v7），使用 `github.com/google/uuid` 解析器验证。

**【次要】`doctor` 信任绕过的安全理由缺乏可见度**
§4.4 指出 `doctor` 跳过工作区信任对话框但会启动 MCP stdio 服务器。这是潜在的安全问题：若当前目录包含恶意 `mcp.json` 配置，doctor 将在无信任确认的情况下启动这些服务器。请记录缓解措施（例如 doctor 仅从用户级配置启动服务器，而非项目级配置）。

---

## 必须修改项

1. **在 §3.2 中定义 `BuildMinimalContainer()`** — 记录其字段集（除 `TUIProgram` 外的所有字段）以及它运行哪些引导阶段（阶段 1–4，跳过阶段 6）。

2. **说明阶段 6 触发机制** — 添加子章节解释"首次渲染后"的实现方式。推荐方案：`Init()` 返回一个 `tea.Cmd`，在零时延 tick 后发送 `DeferredInitMsg`，`Update` 处理 `DeferredInitMsg` 时调用 `startDeferredPrefetches()`。

3. **指定 MCP 服务工具过滤器** — 记录 `mcp serve` 模式使用受限工具集：所有已注册工具减去 `ALL_AGENT_DISALLOWED_TOOLS` 再减去 `AskUserQuestion`（需要 TUI）。在 §4.3 中添加 `BuildMCPServerToolset(registry *tools.Registry) []tools.Tool` 函数。

4. **去除重复的 `runMigrations()` 引用** — 确认 §4.1 的调用与阶段 5 是否相同。若相同，从 §4.1 流程中移除，说明已由阶段 5 覆盖。

5. **补充 `--session-id` 的 UUID 验证规范** — 引用 `github.com/google/uuid` 并指定接受的版本。

6. **记录 `doctor` 信任范围** — 添加安全注释，明确说明 `doctor` 在无信任确认时启动 MCP 服务器，仅读取用户范围的 MCP 配置（`~/.claude/settings.json`），而非项目范围的 `.claude/settings.json`。

---

## 实现注意事项

- `AppContainer.TUIProgram` 类型为 `*tea.Program`（来自 `charmbracelet/bubbletea`），不是自定义接口。这意味着引导层直接依赖 BubbleTea。若无头模式需要避免此依赖（用于减小二进制体积），可引入一个薄包装接口 `TUIProgram interface{ Start() error; Send(msg tea.Msg) }`。
- `applyExtraCACerts()` 在阶段 1 中必须在任何 `net/http` 客户端创建之前运行（包括 `APIClient`）。验证 `BuildContainer()` 中的顺序是严格的：证书 → HTTP 客户端 → `api.NewClient`。不要依赖 `http.DefaultTransport` 的修改；显式地将自定义 `*tls.Config` 注入每个 HTTP 客户端。
- `doctor` 模式启动 stdio MCP 服务器会创建子进程。确保即使在最小初始化模式下也调用 `RegisterCleanup`（来自服务层 stdio 传输），以便 MCP 子进程在退出时被回收。
- `setupGracefulShutdown()` 在阶段 2 中应同时注册 `SIGINT` 和 `SIGTERM` 处理器。在 `-p`（无头）模式中，文档称 SIGINT 是单独注册的（§4.2）——确保没有双重注册冲突。
- 对于 `--agents <json>` 标志：在标志解析时（cobra `PreRunE`）解析和验证 JSON，而非在查询时，以便错误能立即以使用帮助的形式呈现，而非在执行中途出现。
- `--mcp-config <configs...>` 标志可以接受文件路径和原始 JSON 字符串。记录检测逻辑（尝试 `os.Stat`；若文件不存在，尝试 `json.Valid`；若两者都失败，返回错误），避免歧义。
- `shell completion`（`rootCmd.GenBashCompletion()` 等）是 cobra 的零成本内置功能。在初始提交中实现；没有理由推迟。

---

*评审版本：v1.0 · 2026-04-02 · Tech Lead*
