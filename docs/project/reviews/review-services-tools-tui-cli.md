# Services/Tools/TUI/CLI 设计评审

> 评审人：Tech Lead
> 日期：2026-04-02
> 整体结论：**有条件通过**

---

## Services 层评审

### 结论：通过（有轻微缺陷，不阻塞编码）

Services 层设计整体扎实，三个子包职责清晰，接口抽象合理，与架构文档规定的"服务层 → pkg/types，禁止反向依赖核心层"原则一致。

### 问题清单：

#### [SSE 流式处理]

**✅ 正确**：SSE 解析采用 `bufio.Scanner` 逐行读取，`sseReader.Next()` 每次返回**单个 `StreamEvent`**，不做批量缓冲。
调用方（核心层 QueryEngine）通过循环 `StreamReader.Next()` 驱动消费，这与 Anthropic 的 SSE 协议（每行 `data: {...}`，双空行分隔事件）完全一致。

**微小疑点**：
- `[DONE]` 信号的处理方式在 `sseReader.Next()` 注释中提到"遇到 `[DONE]` 返回 `io.EOF`"，但 Anthropic SSE 协议实际使用 `event: message_stop` + `data: {}` 而非 OpenAI 风格的 `[DONE]`。需要确认实现时不误判。
- `Accumulator.Process()` 是辅助型"流式转完整响应"工具，文档说明其使用场景（核心层调用）较模糊，建议在代码注释中明确：它**不是**`StreamReader`的替代，只用于需要完整 `MessageResponse` 的少数场景（如 `/compact`）。

#### [MCP Transport 抽象]

**✅ 足够灵活**：枚举了 `stdio / sse / http / ws` 四种 transport，与 TS 原版 `stdio | sse | http | ws | sdk` 五种对齐（仅缺 `sdk` 类型，可视为 Go 侧不需要保留）。
`Transport` 接口以 `Send/Recv/Close` 三个方法抽象双向 JSON-RPC 通道，简洁且可测试。

**需要关注的点**：

1. **`ws` (WebSocket) transport** 被列在枚举常量中但没有对应的配置结构体（仅有 `StdioTransportConfig`、`SSETransportConfig`、`HTTPTransportConfig`），需要补充 `WSTransportConfig` 或明确在 Go 版本中不支持 WebSocket transport。

2. **SSE vs Streamable HTTP 合并**：`SSETransportConfig` 注释称"也兼容 StreamableHTTP"，但 Streamable HTTP（MCP 2025-03-26 spec）与传统 SSE 协议差异较大（前者使用 `Mcp-Session-Id` header，支持双向单连接；后者是 GET SSE + POST 分离）。建议将二者拆分为独立 transport 类型，或在注释中更详细说明兼容策略。

3. **`NewTransport(t TransportType, cfg any)` 使用空接口**：`cfg any` 类型不安全，在 Go 1.18+ 泛型已稳定的情况下，可考虑使用泛型或为每种 transport 提供专用工厂函数（`NewStdioTransport(cfg StdioTransportConfig)`）。**不阻塞编码，建议改进**。

#### [OAuth Token 存储安全性]

**✅ 整体安全设计良好**：
- 定义了 `TokenStore` 接口解耦存储实现（KeychainStore / FileStore / MemoryStore）
- macOS 优先使用系统 Keychain（`go-keychain`），非 macOS fallback 到加密文件
- PKCE S256 正确使用 `crypto/rand`，State 参数作 CSRF 防护
- 5 分钟 token 过期缓冲设计合理
- `singleflight` 防并发重复刷新，与 TS 的 `refreshPromise` 语义等价

**需要补充/确认的安全细节**：

1. **`FileStore` 加密机制未明确**：文档仅说"使用加密文件存储（非 macOS 平台的 fallback）"，但未说明加密算法（AES-GCM？ChaCha20-Poly1305？）、密钥来源（派生自用户密码？系统 entropy？）。这是**重要安全设计缺口**，必须在编码前明确，否则可能退化为明文存储。

2. **`HandleOAuth401Error` 的竞态保护**：文档中提到"仅当 `failedAccessToken` 与当前存储的 `accessToken` 一致时才触发刷新"，这是正确的防止重复刷新的 compare-and-swap 策略，但需要在实现时用 mutex 保护整个"读取当前 token → 比较 → 刷新"操作，否则仍存在 TOCTOU 竞态。**编码时需特别注意**。

3. **`AuthCodeListener` 端口绑定**：监听随机端口（`port=0`）是正确做法，但需要确保 callback 服务器在成功捕获授权码后**立即关闭**，避免本地端口长时间暴露。

---

## Tools 层评审

### 结论：通过（有条件：需与 Agent-Core #6 接口确认后开始编码）

工具分组清晰，注册机制设计合理，并发安全分类准确。整体设计忠实于 TS 原版语义，同时做了合适的 Go 惯用化改造。

### 问题清单：

#### [工具分组方案]

**✅ 合理**：按功能类别分组（fileops / shell / agent / mcp / web / tasks / interact / misc）比每工具独立目录（TS 风格）更符合 Go 的包粒度惯例。同类工具共享辅助函数（如 `bash_security.go` 被同 package 内多处复用）是正确决策。

**需要关注的点**：

1. **`misc` 包过于宽泛**：目前 `misc/` 包含 `skill.go`、`sleep.go`、`brief.go`、`toolsearch.go`、`syntheticoutput.go`、`lsp.go` 六个性质迥异的工具，这些工具之间几乎没有共享逻辑。建议将来根据实际情况酌情拆分（例如 `lsp.go` 可以独立为 `internal/tools/lsp/`），不阻塞当前编码。

2. **`NotebookEdit` 是否应该在 `fileops` 中**：`NotebookEdit` 操作的是 `.ipynb` 文件，放在 `fileops/` 合理，但其实现复杂度（需要解析 JSON cell 结构）可能值得独立文件，当前设计（`notebookedit.go` 在 `fileops/`）可接受。

#### [工具注册机制与条件注册]

**✅ 支持条件注册**：`RegisterAll(r *Registry, cfg *Config)` 通过 `cfg.WorktreeModeEnabled`、`cfg.TodoV2Enabled`、`cfg.ToolSearchEnabled` 等 feature flag 实现条件注册，设计正确。

**需要补充的点**：

1. **沙箱/权限模式下禁用工具未明确设计**：当前条件注册只覆盖 feature flag 场景，但文档中 3.3 节列出的条件启用工具（如 `REPL` 需要 `USER_TYPE=ant`、`PowerShell` 需要 Windows 平台）的注册条件未在 `RegisterAll` 中体现。需要确认 `cfg.Config` 结构体是否包含这些判断字段，或明确由 `IsEnabled()` 方法在运行时决策。

2. **权限模式（`bypassPermissions` / `plan` 模式）下的工具禁用**：TS 原版在 `plan` 模式下禁止所有写工具执行（不是注销，而是调用时返回拒绝）。设计文档说通过 `EnterPlanMode` 修改 `PermissionContext.Mode`，由权限系统在执行前拒绝。这个路径是正确的，但需要确认 `CheckPermissions()` 接口在此场景的行为（工具接口预期中有 `CheckPermissions` 方法，但在 `IsEnabled()` 层面也需要明确）。

3. **`Register` 重复注册 panic**：MCP 动态工具需要支持断线重连后重新注册，文档 §7.5 提到"Registry 提供 `Deregister` 和 `Replace` 方法"，但 `Registry` 设计代码中未展示这两个方法。**编码前必须补充**。

4. **DefaultRegistry 全局单例**：`var DefaultRegistry = NewRegistry()` 是包级全局变量，与架构文档"依赖注入而非全局单例"的原则略有矛盾。建议通过 `AppContainer` 传递 `*Registry`，`DefaultRegistry` 仅作为测试或 CLI 入口的便捷访问点，并加注释说明。

#### [并发安全分类]

**✅ 分类准确**：只读工具（Read/Glob/Grep/WebFetch/WebSearch 等）标记为 `isConcurrencySafe=true`，写操作工具（Write/Edit/Bash/AgentTool 等）标记为 `false`，逻辑正确。

**一个边界情况**：
- `WebFetch` 标记并发安全，但注释中提到"有本地缓存（缓存写入需 mutex）"。如果多个并发 `WebFetch` 调用同一 URL，缓存的写入需要 mutex 保护，这本身不影响 `isConcurrencySafe=true` 的标记（工具外部行为是并发安全的），但实现时需注意缓存的线程安全。文档已提醒，**实现时留意**。

---

## TUI 层评审

### 结论：通过（有条件：QueryEngine 接口需与 Agent-Core #6 对齐）

TUI 层的 BubbleTea 架构设计正确体现了 Elm Model-Update-View 模式，流式消息渲染机制设计精良，Slash 命令覆盖度高。整体是四份文档中设计最为完整的一份。

### 问题清单：

#### [BubbleTea Elm 架构正确性]

**✅ 正确体现 Elm Model-Update-View**：
- `AppModel` 是不可变值类型（结构体），`Update()` 每次返回新 Model（`(tea.Model, tea.Cmd)` 签名）
- 所有状态变更都通过 `Update()` 分支进行，符合单一事件处理器模式
- 副作用（LLM 查询、磁盘读取）通过 `tea.Cmd` 封装，不在 `Update()` 中直接执行
- `Init()` 返回初始化 `tea.Cmd` 批次（`tea.Batch`），正确

**一个设计问题**：
- `AppModel` 中 `abortFn context.CancelFunc` 字段是可变的函数值。在 BubbleTea 的不可变 Model 惯例中，存储可变的 `CancelFunc` 会导致 Update 函数需要特殊处理（旧的 `abortFn` 在返回新 Model 前不能被 GC）。建议将 `CancelFunc` 存放在独立的、通过指针持有的 cancel context 管理器中，或在文档中明确说明其生命周期管理策略。**不阻塞编码，但需要实现者留意**。

#### [流式消息渲染机制]

**✅ 机制合理且符合 BubbleTea 惯用法**：

文档选择了"Cmd 拉取循环"而非"goroutine + `Program.Send()`"，这是正确的权衡：
- 每次 `Update` 处理完 `StreamTokenMsg` 后返回 `waitForStreamEvent(stream)` 作为新 Cmd
- 权限等待时（`PermissionRequestMsg`）不消费下一个 Cmd，自然实现背压暂停
- 与 TS 的 `for await ... setState` 完全等价

**需要补充的细节**：

1. **`stream <-chan QueryEngine.Event` 的生命周期**：`waitForStreamEvent(stream)` 在每次 Update 后以闭包形式持有 channel 引用。当用户触发 `Abort()` 时，channel 可能被关闭（`ok=false` → 返回 `StreamDoneMsg{}`），但 `AppModel` 中仍持有对 stream channel 的引用。需要明确 Abort 流程下 channel 的清理时机，避免 goroutine 泄漏。

2. **`startQueryCmd` 中 `context.Background()` 的使用**：`Submit(context.Background(), ...)` 使用了不可取消的 context。用户 Abort 时通过 `QueryEngine.Abort()` 方法而非 context 取消，这需要 QueryEngine 内部维护独立的取消机制。文档中 `AppModel.abortFn` 字段和 `Abort()` 接口方法的协作关系需要更明确，建议传入由 `abortFn` 派生的 context 而非 `context.Background()`。

3. **`waitForStreamEvent` 函数签名**：文档中写的是 `waitForStreamEvent(stream <-chan QueryEngine.Event) tea.Cmd`，但 `QueryEngine` 是接口名，实际类型应为 `<-chan Event`（同 package 内的 `Event` 类型）。细节性笔误，不影响架构。

#### [Slash 命令系统覆盖度]

**✅ 覆盖了原版主要命令**，对照 TS `src/commands/` 目录检查：

| 覆盖状态 | 命令 |
|---------|------|
| ✅ 覆盖 | `/clear`, `/compact`, `/config`, `/help`, `/exit`, `/memory`, `/model`, `/theme`, `/vim`, `/status`, `/cost`, `/session`, `/mcp`, `/resume`, `/diff`, `/init`, `/review`, `/commit`, `/terminal-setup` |
| ⚠️ 未明确 | `/login` / `/logout`（auth 相关，通常在 CLI 子命令而非 slash 命令，但 TS 版本 TUI 内可用） |
| ⚠️ 未明确 | `/doctors`（健康检查，TS 版本支持在 REPL 内执行） |
| ⚠️ 未明确 | `/pr-comments`（PR review 辅助，TS 版本部分场景支持） |

以上未明确条目不阻塞当前开发，建议在 Slash 命令实现阶段补充排查。

**命令解析细节**：Tab 补全通过 `CompletePrefix(partial)` 实现，设计正确。但文档未说明是否支持**命令参数**的补全（如 `/model ` 后补全可用模型列表）。TS 版本支持动态参数补全，建议在实现 `/model` 等参数型命令时一并设计。

---

## CLI 入口层评审

### 结论：通过（bootstrap 初始化顺序正确，运行模式分发清晰）

CLI 入口层设计全面，命令树覆盖度高，bootstrap 阶段划分合理，依赖注入模式符合 Go 最佳实践。

### 问题清单：

#### [cobra 命令树覆盖度]

**✅ 核心命令树完整**：对照 TS `src/entrypoints/cli.tsx` 的命令结构：
- 根命令及 `-p / -c / -r` flag：✅
- `mcp` 及全部子命令（serve / add / remove / list / get / add-json / add-from-claude-desktop / reset-project-choices）：✅
- `auth` (login / logout / status)：✅
- `plugin`/`plugins`（list / install / uninstall / enable / disable / update / validate / marketplace）：✅
- `agents`, `doctor`, `update`, `install`：✅

**内部/实验性子命令处理**：文档明确注明 `server / ssh / open / setup-token / completion / ps / logs / attach / kill / remote-control` 在 Go 实现阶段暂不纳入，并加了注释说明这是有意为之。这个决策合理，但建议：
1. 在代码中用 `// TODO(feature): <BRIDGE/BG_SESSIONS>` 标注占位，便于后续实现
2. `completion <shell>` 是 cobra 内置功能（`rootCmd.GenBashCompletion()` 等），建议**优先实现**，成本几乎为零

#### [bootstrap 初始化顺序]

**✅ 顺序正确**，六个阶段的排布逻辑清晰：

```
阶段 0  快速路径（零依赖）
阶段 1  配置（CA 证书需在首次 TLS 握手前就绪）✅
阶段 2  网络安全（mTLS/Proxy 在任何 HTTP 调用前就绪）✅
阶段 3  认证预热（并行，不阻塞主流程）✅
阶段 4  特性/策略（并行）✅
阶段 5  数据迁移（在服务初始化前执行，避免旧格式数据被新逻辑误读）✅
阶段 6  延迟服务初始化（首次渲染后，不阻塞冷启动）✅
```

**两个需要确认的细节**：

1. **Trust 对话框时序**：文档 §4.1 提到"首次运行展示 trust 对话框，accept 后才执行 git 相关操作"，但 bootstrap 阶段列表（§3.1）中未明确 trust 检查的位置。对照 §7.4 中"trust 前/后两阶段"设计，trust 检查应该在**阶段 2（运行时安全）之后、阶段 6 之前**，且明确在 `tui.Program.Start()` 序列中触发。建议在代码实现中添加注释标明 `showSetupScreensIfNeeded()` 的调用位置。

2. **`applyExtraCACerts()` 的时机**：阶段 1 中"在第一次 TLS 握手前注入自定义 CA 证书"——Go 的 `http.DefaultTransport` 是全局共享的，注入 CA 证书需要在任何 HTTP 客户端初始化之前完成。当前阶段 1 位于"配置系统初始化"，早于服务层初始化，时序正确，但需确保实现时使用自定义 `tls.Config` 注入到所有 HTTP client（包括 API Client、OAuth client、MCP HTTP transport），而非依赖 `http.DefaultTransport` 的修改（后者在并发场景不安全）。

#### [4 种运行模式分发逻辑]

**✅ 四种模式分发清晰**：

| 模式 | 触发条件 | 初始化路径 | 设计评价 |
|------|---------|-----------|---------|
| 交互式 REPL | 无特殊 flag | `BuildContainer()` → `tui.Program.Start()` | ✅ |
| 非交互式（`-p`） | `--print` flag | `BuildMinimalContainer()` → `engine.Query()` | ✅ |
| MCP 服务模式 | `mcp serve` 子命令 | `mcpServer.Listen()` | ✅ |
| 诊断模式 | `doctor` 子命令 | `bootstrap.RunDiagnostics()` | ✅ |

**一个细节问题**：
- 文档 §7.2 提到"-p 模式下不向 cobra 注册约 50 个子命令"，实现方式是"在 `buildRootCmd()` 时检查 `os.Args` 是否包含 `-p`"。这种**在 cobra 初始化前直接检查 `os.Args`** 的做法存在边缘情况：如果用户使用 `claude --model claude-3-7-sonnet -p "..."` 或 `-p` 出现在其他位置，`strings.Contains(os.Args, "-p")` 可能误判（如目录名包含 `-p`）。建议使用更精确的 arg 解析（cobra 的 `FParseErrWhitelist` 或预扫描 `--` 边界），或接受极少数边缘情况的轻微开销，先完整注册再根据 `--print` flag 决定后续路径。

**全局 Flag 设计**：所有 flag 设计全面，与 TS 原版完全对应，包括 `--dangerously-skip-permissions`、`--effort`、`--thinking` 等高级 flag，覆盖度满分。

---

## 总结

四份文档整体质量高，设计决策充分考虑了 Go 惯用法与 TS 原版语义的平衡。共同特点：
- 接口优先、依赖倒置设计一致
- 对 TS 原版的行为映射清晰（TS→Go 映射表设计良好）
- 并发安全意识贯穿各层
- 各层通过 `TODO(dep)` 占位标注跨 Agent 依赖，依赖关系清晰

**唯一阻塞性问题**：`oauth.FileStore` 加密机制必须在 Agent-Services 编码前明确（AES-GCM 推荐，密钥派生使用 PBKDF2 或平台 entropy）。

**各 Agent 可开始编码**（依赖关系满足后）：

- **Agent-Services 可开始 #14**（待 #13 Infra 实现完成；`FileStore` 加密方案需先明确）
- **Agent-Tools 可开始 #16**（待 #11 Tool 接口由 Agent-Core 确认；`Registry.Deregister/Replace` 方法需在编码时补充）
- **Agent-TUI 可开始 #17**（待 #15 Core 实现完成；可用 `stubQueryEngine` 先开发全部 TUI 功能；`Abort()` 与 context 传递方式需编码时明确）
- **Agent-CLI 可开始 #18**（待 #17 TUI 实现完成；`-p` 模式的 `os.Args` 预扫描逻辑建议用更健壮的实现替代）
