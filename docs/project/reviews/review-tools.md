# Tech Lead 评审：tools.md

> 评审人：Tech Lead
> 日期：2026-04-02
> 结论：**有条件通过（APPROVED_WITH_CHANGES）**

---

## 总体评估

工具层设计详尽、结构清晰，将约 40 个 TypeScript 工具忠实映射到 Go。注册表设计、并发分类和目录结构均合理。阻塞性问题主要集中在并发规则不完整和缺少安全边界文档。所需修改可解决。

---

## 优点

1. **工具枚举完整** — 文档枚举了 23 个核心工具 + 6 个 TaskV2 工具 + 8 个条件工具 = 37 个明确命名的工具。加上动态 `MCPTool` 包装器，舒适覆盖"约 40 个工具"的设计目标（§3.1–3.3）。

2. **`Registry` 使用 `sync.RWMutex`** — 正确：`Register/Get` 使用写锁，`All/Filter` 使用读锁。`order []string` 字段保留插入顺序，对于确定性的 API `tools` 数组输出非常重要（§5.1）。

3. **`isConcurrencySafe` 分类** — 12 个并发安全工具和 12+ 个串行工具均附有推理说明（§6.1–6.2）。这是查询引擎并行执行策略的正确抽象。

4. **特性开关条件注册** — `RegisterAll(r *Registry, cfg *Config)` 模式配合 `cfg.WorktreeModeEnabled`、`cfg.TodoV2Enabled` 和 `cfg.ToolSearchEnabled` 守卫，正确对应 TS 的特性开关门控（§5.2）。

5. **`BLOCKED_DEVICE_PATHS` 完整枚举** — 所有 12 个设备路径（`/dev/zero`、`/dev/random`、`/dev/urandom`、`/dev/full`、`/dev/stdin`、`/dev/tty`、`/dev/console`、`/dev/stdout`、`/dev/stderr`、`/dev/fd/0`、`/dev/fd/1`、`/dev/fd/2`）均已列出（§4.1.2）。

6. **Bash 三层安全检测** — 第一层（用户 excludedCommands）、第二层（安全规则：环境变量劫持、跨管道 cd+git、包装器剥离）、第三层（权限系统），忠实移植了 TS 的 `bashSecurity.ts` 架构（§4.2.2）。

7. **`BashTool` 超时常量** — `DefaultBashTimeoutMs=120_000`（2 分钟）和 `MaxBashTimeoutMs=600_000`（10 分钟）与 TS 一致（§4.2.4）。

8. **AgentTool 禁止工具列表** — `AskUserQuestion`、`EnterPlanMode`、`ExitPlanMode`、`EnterWorktree`、`ExitWorktree` 正确对应 `ALL_AGENT_DISALLOWED_TOOLS`（§4.3.1）。

9. **`FileEdit` 唯一性验证** — 读取 → 计数 → 检查（0=未找到，>1=模糊，1=替换）→ 写入的流程正确防止了静默过匹配。TOCTOU 注释（§7.7）准确且重要。

10. **工具单例模式** — 包级 `var FooTool = &fooTool{}` 单例配合无状态 `Call` 方法，与 TS 的 `buildTool({...})` 导出模式一致（§7.2）。

11. **目录结构合理性** — 按类别分组（而非按工具）的决策论证充分：共享辅助文件（`bash_security.go`、`bash_sandbox.go`、`bash_timeout.go`）、受控导入链和合理的包数量（§2、§7.1）。

---

## 问题

**【阻塞】§6.3 并发规则不完整且可能有误**
"批次中任何写工具 → 整个批次串行"的规则与每工具 `isConcurrencySafe` 方法相冲突。若 LLM 在一轮中请求 `[Read, Bash, Grep]`，当前规则会因 `Bash` 为串行而将全部三个工具串行化。正确方案（与 TS 行为一致）是：先并行运行所有并发安全工具，再逐一运行串行工具。按现有写法的批次全局串行化规则将在任何混合轮次中完全消除并行性。此问题必须在查询引擎实现前修正。

**【严重】`MCPTool` 动态注册缺少 Registry 的 `Deregister`/`Replace` API**
§7.5 指出 MCP 重连需要 `Deregister` 和 `Replace` 方法，但 §5.1 仅定义了 `Register`、`Get`、`All` 和 `Filter`。Registry 结构体和接口必须在 MCP 工具安全处理断连/重连周期之前扩展这些方法。

**【严重】`AgentTool` 实现是无具体设计的存根**
§4.3.1 中含有 `return ToolResult{Content: "TODO: sub-agent result"}, nil`。与其他明确命名依赖的 TODO 不同（`TODO(dep): Agent-Core #6`），AgentTool 本身除输入 schema 外没有任何设计。子 agent fork 策略（克隆 vs 新引擎）、`errgroup` 使用和级联取消（§7.8）仅以文字描述，没有代码结构。这在设计文档中可接受，但应明确标注为 `TODO(dep): Agent-Core #6`。

**【次要】`WebFetch` HTML 转 Markdown 库未指定**
§4.5.1 提到"goquery 或 html2text"但未确定选择哪个。这是一个真实的依赖决策，不同库输出质量有差异。建议选定 `github.com/JohannesKaufmann/html-to-markdown`（维护良好，还原度高）并记录在文档中。

**【次要】`FileRead` 的 Token 预算估算使用未经校准的近似倍数**
§7.6 中"代码 0.3 token/byte，文本 0.25 token/byte"的启发值没有校准数据支撑。应记录这些数值的来源及更新方式。此外，"估算 > 限制/4 时调用 Token Count API"的逻辑会产生额外 API 往返，需明确此操作是阻塞还是异步。

**【次要】条件工具表中的 `PowerShell` 缺少实现注释**
§3.3 将 `PowerShell` 列为 Windows 平台限定，但 §4 中没有对应的实现章节。至少应说明它使用 `exec.Command("powershell.exe", "-Command", cmd)`，并与 `BashTool` 共享超时/安全逻辑。

**【次要】`DefaultRegistry` 全局变量违反 DI 原则**
`var DefaultRegistry = NewRegistry()` 是包级全局变量，与架构"无全局单例"原则相悖（cli.md §7.3）。应注释说明 `DefaultRegistry` 仅用于测试/CLI 入口便利；`AppContainer` 应始终显式传递 `*Registry`。

---

## 必须修改项

1. **重写 §6.3 并发规则。** 将"任何写工具 → 全部串行"规则替换为：(a) 按 `isConcurrencySafe` 分组工具；(b) 通过 `errgroup` 并行运行所有安全工具；(c) 并行组完成后串行运行不安全工具；(d) 为并行组内的文件类工具添加路径冲突检测。

2. **向 §5.1 的 `Registry` 结构体添加 `Deregister(name string) error` 和 `Replace(t Tool)` 方法**，并说明线程安全语义。注明 `Deregister` 必须在移除工具条目前排空所有进行中的调用。

3. **明确将 `AgentTool` 标记为 `TODO(dep): Agent-Core #6`**，并提供最小接口规范：fork 子引擎需要哪些 `QueryEngine` 方法（`Fork(opts ForkOptions) QueryEngine`），以及 `ForkOptions` 包含哪些字段。

4. **为 `WebFetch` 确定 HTML 转 Markdown 库**，并将其添加到依赖项章节。

5. **在 §4 中补充 `PowerShell` 工具实现注释**，并引用共享沙箱/超时基础设施。

---

## 实现注意事项

- `Register` 重复名称 panic（§5.1）对于启动时注册是正确的，但在 MCP 重连时若再次调用会 panic。按必须修改项 2 添加的 `Replace` 方法应作为 MCP 重注册的正确路径。
- `FileReadTool.readText` 调用 `validateContentTokens`，可能需要真实的 token 计数器。考虑在回退到 Token Count API 之前使用 `github.com/tiktoken-go/tokenizer` 进行 cl100k_base 估算，避免在常见情况下产生额外 API 调用。
- `BashTool.ShouldUseSandbox` 正确地对 `SandboxManager.IsEnabled()` 进行门控——确保 `SandboxManager` 在任何 `BashTool.Call` 调用之前完成初始化。§7.4 关于 Phase 1 使用普通 `exec.Command` 的说明对于 MVP 可接受，但必须立即定义 `BashExecutor` 接口，以便 Phase 2 可以无需修改调用方地换入沙箱实现。
- `TaskCreate` 使用 `context.WithCancel(parentCtx)` 派生协程是级联取消的正确方式（§7.8）。确保所有协程在 `TaskRegistry` 中被追踪，且关闭时等待所有运行中的任务（使用 `sync.WaitGroup`）。
- `CallOptions.Context` 类型（`ToolUseContext`）被引用但从未在文档中定义。必须在 `pkg/types` 中定义，至少包含：`Permissions`、`FileReadingLimits`、`Engine`（供 AgentTool 使用）和 `RequestPrompt`（供 AskUserQuestion 使用）。

---

*评审版本：v1.0 · 2026-04-02 · Tech Lead*
