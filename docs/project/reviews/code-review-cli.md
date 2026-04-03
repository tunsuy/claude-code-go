# CLI Layer Code Review

> **Reviewer**: Tech Lead
> **Date**: 2026-04-03
> **Subject**: 任务 #18 实现代码（Agent-CLI）
> **Verdict**: APPROVED_WITH_CHANGES

---

## 1. Overall Assessment

CLI 入口层整体质量良好，主干结构与设计文档高度一致：快速路径拦截、cobra 命令树、headless/interactive 双路径分发、MCP serve 循环均已到位。P0-C/D/E/F 四项历史问题已按约定修复，`os.Exit`/`os.Setenv` 的可测试性隐患也基本得到清除。

但仍存在若干值得关注的问题：MCP serve 实现是一个**功能性占位**而非生产就绪实现；bootstrap 的包级全局变量 `rootCmd` 尚存在并发/重入风险；`isPrintMode` 与实际 cobra 解析之间的逻辑二义性在特定参数排列下会产生误判；测试覆盖率对关键路径（interactive run、headless run、MCP serve）仍不足。这些问题中部分属于 P1 级别，需在下一个迭代中跟进。

---

## 2. Design vs Implementation Delta

| 设计文档约定 | 实现状态 | 差异说明 |
|---|---|---|
| `main.go` 仅做快速路径 + `bootstrap.Run()` | ✅ 完全符合 | — |
| `HandleFastPath` 处理 `--version`/`-v` | ✅ 符合 | 设计提到 `--dump-system-prompt` 等额外快路径，实现暂未涵盖（可接受） |
| 6 阶段初始化顺序 | ⚠️ 部分符合 | 阶段 2（proxy/mTLS）、阶段 4（feature flags）、阶段 5（migrations）均为空实现，`wire.go` 已覆盖阶段 1/3/6 |
| `AppContainer` 字段结构 | ✅ 符合 | 字段命名略有差异（`AppStateStore` vs 设计的 `StateStore`），属可接受重构 |
| `TODO(dep)` 占位（`wire.go` 接口断言） | ❌ 未实现 | 设计文档 §6 要求的四条 `var _ Interface = (...)` 编译期接口断言完全缺失，失去早期发现接口漂移的保障 |
| `plugin` 命令别名 `plugins` | ❌ 缺失 | 设计 §1 命令树要求 `plugin (alias: plugins)`，实现未注册 alias |
| `update (alias: upgrade)` | ❌ 缺失 | 同上，`upgrade` 别名未注册 |
| `mcp serve` 设置 `CLAUDE_CODE_ENTRYPOINT=mcp` | ❌ 缺失 | 设计 §4.3 明确要求；实现的 `runMCPServe()` 未设置该标识 |
| headless 模式 `CLAUDE_CODE_ENTRYPOINT=sdk-cli` | ❌ 缺失 | 设计 §7.6 约定通过 `StateStore.SetEntrypoint()` 设置，未实现 |
| `-p` 模式跳过子命令注册 | ✅ 符合 | `buildRootCmd(headless)` 分支正确 |
| `--resume`/`--continue` 对话恢复逻辑 | ⚠️ 部分符合 | flag 已注册，但 `runInteractiveOrHeadless` 未消费这两个 flag（未传入 TUI） |
| `doctor` 最小化初始化（不加载 Engine） | ⚠️ 部分符合 | `runDoctor()` 实现极简，不加载 Engine，但未做网络/MCP 检查（仅打印 Go runtime 信息） |
| `wire.go` 设计名称对应 `BuildContainer` provider 风格 | ✅ 符合 | — |

---

## 3. Strengths

1. **main.go 极简设计落地干净**：职责单一，仅 23 行，无业务逻辑渗漏，完全符合设计意图。

2. **快速路径实现正确**：`HandleFastPath` 扫描原始 `args[1:]`，正确处理 `--` 终止符，在 cobra 初始化前截断，零依赖开销。

3. **headless 子命令注册跳过**：`isPrintMode` 早于 cobra 解析扫描 `os.Args`，`buildRootCmd(headless)` 按条件跳过全部交互型子命令注册，完整还原设计 §7.2 的性能优化意图。

4. **rootFlags 完整性高**：`root.go` 中 `rootFlags` 结构体涵盖设计 §5 全局 flag 表中的绝大多数条目，字段命名清晰，分组注释规范。

5. **P0-E 修复到位**：`runAuthStatus` 改为 `return fmt.Errorf("not authenticated")` 而非 `os.Exit(1)`，使函数可被单元测试直接调用、验证返回值，无副作用。

6. **P0-D 修复到位**：`root.go` 无 `os.Setenv` 调用，entrypoint 标识的设置按设计意图延后至 `StateStore`（虽然该调用尚未实现，但至少不再有裸 `os.Setenv` 污染测试环境）。

7. **依赖注入架构**：`AppContainer` + `ContainerOptions` 的构造函数注入模式符合设计 §7.3，没有 package 级别的 mutable 全局状态（除 `rootCmd` 外，见 P1-A）。

8. **MCP 命令树结构完整**：`mcp.go` 注册了设计要求的全部 8 个子命令，未实现的以明确 `not yet implemented` 错误返回，便于后续 Agent-Services 接手。

9. **错误处理风格一致**：所有 `RunE` 通过 `fmt.Errorf("%w", err)` 包装上游错误，cobra 的 `SilenceErrors: true` + `SilenceUsage: true` 组合使顶层错误格式由调用方控制，符合惯例。

10. **测试文件存在且有基础覆盖**：`bootstrap_test.go` 覆盖了 `runAuthStatus`、`BuildContainer`、`resolveModel`、`collectHeadlessPrompt` 四个函数，均为纯函数或可隔离的逻辑；`main_test.go` 覆盖 `HandleFastPath` 和 `--help` 不 panic。

---

## 4. Issues

### P0 — Must Fix Before Merge

> 无新增 P0 问题（原 P0-C/D/E/F 已修复）。

---

### P1 — Should Fix Soon

#### P1-A：`bootstrap.go` 中 `rootCmd` 包级全局变量破坏测试隔离

**位置**：`internal/bootstrap/bootstrap.go:28`

```go
// 现有代码
var rootCmd *cobra.Command

func Run(args []string) error {
    rootCmd = buildRootCmd(headless)
    ...
}
```

**问题**：`rootCmd` 是包级 `var`，`Run()` 每次调用都覆盖它。在并行测试（`t.Parallel()`）或同一进程内连续调用 `Run()` 的场景下会产生竞争。更重要的是，这破坏了设计 §7.3 "避免全局单例" 的原则。

**建议**：将 `rootCmd` 改为 `Run()` 的本地变量；`Execute()` 兼容函数可直接改为调用 `Run(os.Args)`（已是如此）并去掉全局持有。

---

#### P1-B：`isPrintMode` 与 cobra flag 解析存在二义性

**位置**：`internal/bootstrap/bootstrap.go:69-79`

```go
func isPrintMode(args []string) bool {
    for _, a := range args[1:] {
        if a == "-p" || a == "--print" {
            return true
        }
        if a == "--" { break }
    }
    return false
}
```

**问题**：该扫描不感知其他 flag 的参数消耗。例如 `claude --model -p` 会误判为 print 模式（`-p` 实际上是 `--model` 的值）；而 `claude somesubcmd -p` 在非 headless 场景下注册了子命令，但 `isPrintMode` 已导致子命令被跳过。

这与原 TS 实现一致（TS 同样用字符串扫描），但 Go 的强类型 cobra 环境提供了更安全的替代方案：在注册完整命令树后，先 `Parse()` flags 再决定是否需要精简路径——或在根命令的 `PersistentPreRunE` 中检查已解析的 `f.print`，然后动态跳过子命令（实际上子命令注册时机已过，此时仅影响 help text）。

**短期修复建议**：在 `isPrintMode` 函数添加注释，说明已知的二义性边界，并添加针对 `claude --model -p` 场景的单元测试，明确文档化期望行为（即使是"允许误判"）。

---

#### P1-C：`--resume` / `--continue` flag 未传入 TUI

**位置**：`internal/bootstrap/root.go:168-192`（`runInteractiveOrHeadless`）

```go
// runInteractive 调用处
return runInteractive(f, opts)
```

**问题**：`rootFlags` 中的 `continueSession`（`-c`）和 `resume`（`-r`）已解析，但 `runInteractive` 函数签名仅接受 `f *rootFlags` 和 `opts ContainerOptions`——而构建 TUI 时：

```go
m := tui.New(container.QueryEngine, container.AppStateStore, false, true)
```

`f.continueSession` 和 `f.resume` 完全未被传递给 TUI 或 `AppStateStore`。这意味着 `-c` / `-r` flag 被用户传入后静默丢弃，实现的对话恢复能力为零。

**建议**：在 `runInteractive` 中将 `f.resume`/`f.continueSession` 写入 `AppStateStore`（或传递给 `tui.New`），或至少添加 `TODO` 注释标记此处为已知缺口。

---

#### P1-D：`mcp serve` 的 `CLAUDE_CODE_ENTRYPOINT` 缺失

**位置**：`internal/bootstrap/mcp.go:45`（`runMCPServe`）

**问题**：设计 §4.3 和 §7.6 明确要求：进入 `mcp serve` 路径时须设置 `CLAUDE_CODE_ENTRYPOINT=mcp`。当前实现完全缺失。下游的 QueryEngine / 工具层依赖此变量做行为差异化（如权限对话框显示逻辑），缺失会导致 MCP 模式下工具行为与设计预期不符。

**建议**：在 `runMCPServe()` 函数入口处通过 `container.AppStateStore.SetEntrypoint("mcp")` 设置（或在 `newMCPServeCmd` 的 `PersistentPreRunE` 中处理）。

---

#### P1-E：MCP serve 实现为占位骨架，缺乏生产所需的关键处理

**位置**：`internal/bootstrap/mcp.go:45-78`

当前实现仅处理 `initialize` 一个 JSON-RPC 方法，对其他所有方法返回 `-32601 Method not found`。具体缺失：

| 缺失点 | 影响 |
|---|---|
| `tools/list` 方法未实现 | MCP 客户端无法发现可用工具 |
| `tools/call` 方法未实现 | 工具调用完全无法转发 |
| `capabilities.tools` 为空对象 | 客户端无法判断服务端工具能力 |
| `enc.Encode` 错误被 `//nolint:errcheck` 静默 | 写入 stdout 失败时服务端无感知，客户端挂起 |
| 无 `initialized` 通知处理 | 部分 MCP 客户端要求服务端响应此通知 |
| `serverInfo.version` 硬编码 "0.1.0" | 未读取 `appVersion` 变量 |

**建议**：如果 P0-F 的交付标准是"能通过 MCP 协议测试套件基本握手"，当前实现勉强合格。但若预期"能被真实 MCP 客户端使用工具"，须升级为至少实现 `tools/list` + `tools/call` 的完整分发循环，并修复 encode 错误静默问题。

---

#### P1-F：`wire.go` 缺少设计要求的编译期接口断言

**位置**：`internal/bootstrap/wire.go`（整个文件）

**问题**：设计文档 §6 明确列出四条 `var _ Interface = (...)` 编译期断言，用于在各 Agent 模块接口尚未稳定期间捕捉接口漂移。实现中完全缺失，失去了设计意图中最重要的"接口契约早期发现"保障。

**建议**：在 `wire.go` 中添加注释形式的占位断言（即使对应包尚未提供完整接口，也应以 `TODO` 形式声明意图），或在对应包接口稳定后立即补充。

---

### P2 — Minor / Suggestions

#### P2-A：`plugin` 命令缺少 `plugins` 别名，`update` 缺少 `upgrade` 别名

**位置**：`internal/bootstrap/plugin.go:10`，`internal/bootstrap/misc.go:42`

设计 §1 命令树注明 `plugin (alias: plugins)` 和 `update (alias: upgrade)`。Cobra 通过 `Aliases: []string{"plugins"}` 字段支持别名，一行修复。

---

#### P2-B：`runInteractive` 中 SIGINT goroutine 逻辑空洞

**位置**：`internal/bootstrap/root.go:204-211`

```go
go func() {
    <-sigCh
    // Let BubbleTea handle its own cleanup; the engine interrupt will
    // propagate via context cancellation.
}()
```

这个 goroutine 接收到 SIGINT 后什么也不做（注释说 BubbleTea 自行处理），意味着该 goroutine 仅在收到信号后退出，没有任何实际作用。如果 BubbleTea 确实自行处理 SIGINT，则 `signal.Notify(sigCh, ...)` 本身会拦截操作系统的默认行为（立即终止），反而可能延迟响应。

**建议**：要么删除此代码段（让 BubbleTea 完全管理信号），要么实现实际的清理逻辑（调用 `p.Quit()` 或 cancel context）。

---

#### P2-C：`openBrowser` 的 `//nolint:errcheck` 注释宜改为显式忽略

**位置**：`internal/bootstrap/auth.go:119`

```go
openBrowser(authURL) //nolint:errcheck
```

`openBrowser` 失败（如用户在无显示器服务器运行）时，用户只能看到 URL 输出到 stdout，但不会有任何提示"浏览器打开失败，请手动访问以上 URL"。建议显式处理错误并打印提示，而不是静默丢弃。

---

#### P2-D：`BuildHeadlessContainer` 是 `BuildContainer` 的直接 passthrough

**位置**：`internal/bootstrap/wire.go:101-107`

```go
func BuildHeadlessContainer(opts ContainerOptions) (*AppContainer, error) {
    return BuildContainer(opts)
}
```

当前完全等价于 `BuildContainer`，注释说"MCP 初始化跳过等优化可在未来迭代添加"，但这意味着 headless 模式仍然建立了 `mcp.Pool` 和 TUI 无关的依赖。若 `BuildContainer` 未来引入 TUI 相关的初始化（如 terminal capability 探测），headless 路径会被误带入。**建议**：至少在函数体内添加 `// TODO: skip MCP pool init in headless mode` 等具体 TODO，防止遗忘。

---

#### P2-E：`bootstrap_test.go` 的 `TestMain` 使用 `os.Exit(m.Run())` 而非返回

**位置**：`internal/bootstrap/bootstrap_test.go:27`

```go
os.Exit(m.Run())
```

这是 Go 测试 `TestMain` 的标准写法，本身无误。但此处 `defer os.RemoveAll(tmpHome)` 会因 `os.Exit` 而**不被执行**，造成临时目录泄漏。标准修法：

```go
code := m.Run()
os.RemoveAll(tmpHome) // 显式清理，因 os.Exit 绕过 defer
os.Exit(code)
```

---

#### P2-F：`consumeText` 对 `MsgTypeTurnComplete` 每次都追加换行

**位置**：`internal/bootstrap/run.go:117-119`

```go
case engine.MsgTypeTurnComplete:
    fmt.Fprintln(w) //nolint:errcheck
```

如果 LLM 响应本身以 `\n` 结尾（非常常见），此处会额外追加一个空行，导致 `-p` 模式 pipeline 中出现冗余换行。建议追踪最后写入的字节，仅在响应非 `\n` 结尾时补充。

---

#### P2-G：`resolveAPIKey` 每次调用均构造完整 OAuth client，有轻微性能开销

**位置**：`internal/bootstrap/wire.go:119-143`

`resolveAPIKey` 在每次 `BuildContainer`/`BuildHeadlessContainer` 调用时都创建 `TokenStore`、`OAuthConfig`、`OAuthClient`、`TokenManager` 四个对象，然后调用网络 I/O（`CheckAndRefreshIfNeeded`）。对于热路径（如批量脚本调用 `-p` 模式）这是可接受的，但应在函数注释中说明此调用可能有网络延迟，并考虑 timeout 控制（当前使用 `context.Background()`，无超时）。

---

#### P2-H：测试覆盖率缺口

当前测试文件覆盖：

| 函数 | 覆盖状态 |
|---|---|
| `HandleFastPath` | ✅ `main_test.go` |
| `bootstrap.Run` (--help 不 panic) | ✅ `main_test.go` |
| `runAuthStatus` (unauthenticated) | ✅ `bootstrap_test.go` |
| `BuildContainer` (missing config) | ✅ `bootstrap_test.go` |
| `resolveModel` | ✅ `bootstrap_test.go` |
| `collectHeadlessPrompt` (from args) | ✅ `bootstrap_test.go` |
| `collectHeadlessPrompt` (from stdin) | ❌ 缺失 |
| `runMCPServe` (initialize handshake) | ❌ 缺失 |
| `runMCPServe` (unknown method → error) | ❌ 缺失 |
| `isPrintMode` (various arg patterns) | ❌ 缺失 |
| `applyPermissionFlags` | ❌ 缺失 |
| `runAuthLogin` (--api-key fast path) | ❌ 缺失 |
| `buildRootCmd` (headless=true vs false 子命令数量) | ❌ 缺失 |

建议优先补充 `runMCPServe` 和 `isPrintMode` 的测试，前者是 P0-F 交付物，后者是影响整个启动路径分发的关键判断函数。

---

## 5. Summary

| 严重级别 | 数量 | 条目 |
|---|---|---|
| P0（必须合并前修复） | 0 | — |
| P1（应在下个迭代修复） | 6 | P1-A 全局 rootCmd、P1-B isPrintMode 二义性、P1-C resume/continue 未传递、P1-D MCP entrypoint 缺失、P1-E MCP serve 骨架不完整、P1-F 接口断言缺失 |
| P2（建议/轻微） | 8 | P2-A 别名缺失、P2-B 空洞 SIGINT goroutine、P2-C openBrowser 错误静默、P2-D BuildHeadlessContainer passthrough、P2-E defer+os.Exit、P2-F 冗余换行、P2-G OAuth 无超时、P2-H 测试覆盖缺口 |

**裁定理由**：无新增 P0，`main.go` 框架结构和 cobra 命令树结构质量可接受，可以合并。但 P1-C（`-c`/`-r` flag 静默丢弃）和 P1-E（MCP serve 缺少 `tools/list`+`tools/call`）是明显的功能性空洞，需在下一个 sprint 内补齐，建议以跟踪 issue 形式记录并 assign 给 Agent-CLI 负责人。
