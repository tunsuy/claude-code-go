# 基础设施层设计评审

> 评审人：Tech Lead
> 日期：2026-04-02
> 结论：**有条件通过**

---

## 总体评价

基础设施层设计整体思路清晰，层次划分合理，TypeScript → Go 的类型映射策略（branded string、discriminated union → struct + Type 字段、泛型 Store）体现了对 Go 惯用法的良好理解。依赖方向图正确，零依赖的 `pkg/types` 作为基石的设计符合架构文档要求，`RWMutex` 锁外通知订阅者的设计防止了死锁，`AtomicWriteFile` 的 rename 策略符合 POSIX 原子性语义，均是亮点。

但评审过程中发现 **3 个 P0 级缺陷**，会在编码阶段直接导致运行时崩溃、数据竞争或企业安全策略失效，必须在动工前修复。另有若干 P1、P2 问题影响可测试性和代码质量。

---

## 问题清单

| 严重级别 | 位置 | 问题描述 | 建议 |
|---------|------|---------|------|
| **P0** | `pkg/types/ids.go` `NewAgentId()` | 导出函数体为 `panic("see internal/bootstrap")`。`pkg/types` 是公共类型包，任何上层模块调用 `types.NewAgentId()` 时都会直接 panic，且该包无法反向 import `pkg/utils/ids`（会循环依赖），是破坏性 API 契约。 | 从 `pkg/types/ids.go` 中**删除** `NewAgentId()` 函数，仅保留 `AsAgentId()`/`AsSessionId()` 纯类型转换函数；ID 生成逻辑已在 `pkg/utils/ids/ids.go` 正确实现，保留即可。 |
| **P0** | `internal/config/loader.go` `mergeSettings()` 调用顺序 | §3.1 将企业托管配置（`managed-settings.json`）明确定义为"优先级**最高**，用于企业管控"，但代码 `mergeSettings(ls.Policy, ls.User, ls.Project, ls.Local)` 中 Policy 排在最前（被后者覆盖），实际是**最低优先级**。企业安全策略可被用户本地配置任意覆盖，语义完全颠倒，是安全设计缺陷。 | 明确区分 Policy 的"锁定字段"（`disableBypassPermissionsMode`、`allowManagedHooksOnly` 等，不可被覆盖）和"默认字段"（可作为基底被覆盖）。方案一：调整合并顺序为 `mergeSettings(ls.User, ls.Project, ls.Local)` 后再用单独的 `applyPolicyOverrides(ls.Policy)` 强制覆盖锁定字段；方案二：参考 TS 原版 `managedSettingsOverride` 实际行为确定字段分类。无论哪种方案，代码与文档描述必须完全一致。 |
| **P0** | `internal/state/store.go` `SetState()` + `AppState` map/slice 引用语义 | `SetState` 使用 `func(prev T) T` updater，但 `AppState.Tasks`、`AgentNameRegistry` 等字段是 Go map（引用类型）。Updater 返回修改后的结构体值后，旧快照 `prev` 与新状态 `next` 共享同一底层 map，写锁释放后 `notifyListeners(next, prev)` 传出的 `prev` 依然指向已被修改的 map，导致订阅者读到"脏旧状态"。§4.4 示例代码 `prev.Tasks[taskId] = TaskState{...}` 正是触发此竞争的危险用法，设计文档承认问题但将修复推迟至 Phase 2，Phase 1 无任何保护。 | Phase 1 必须提供明确的保护方案（三选一）：(1) `SetState` 内对 map 字段自动执行 `maps.Clone()`（Go 1.21+）再传 updater；(2) 在 §4.4 示例中强制展示 copy-on-write 用法，作为所有 updater 实现的约定；(3) 在 CI 中运行 `go test -race` 并以集成测试覆盖并发写 map 的路径。"Phase 2 再修复"不可接受。 |
| **P1** | `internal/config/loader.go`、`internal/session/store.go`、`internal/session/manager.go` | 三个核心基础设施组件均以具体 struct 暴露，缺乏对应 interface。上层模块（core、tools）无法 mock 配置加载和会话存储，单元测试将依赖真实文件系统，不符合可测试性要求。 | 为每个组件定义 interface：`type ConfigLoader interface { Load() (*LayeredSettings, error) }`、`type SessionStorer interface { AppendEntry(any) error; ReadAll() ([]EntryEnvelope, error); Close() error }`。具体 struct 实现这些 interface，调用方依赖 interface 而非具体类型。 |
| **P1** | `internal/state/store.go` `Subscribe()` 内存泄漏 | 取消订阅通过将切片槽位置 nil 实现，切片**只增不减**。长期运行高频 subscribe/unsubscribe（TUI 重渲染场景）会导致 `listeners` 切片无限增长，每次 `notifyListeners` 遍历全量 nil 槽，是内存和 CPU 双重浪费。 | 改用 `map[uint64]Listener[T]` + 自增 ID 管理订阅者，取消时 `delete(s.listeners, id)`，彻底消除泄漏。需使用独立的 `listenerMu sync.Mutex` 保护此 map。 |
| **P1** | `pkg/types/logs.go` `SerializedMessage` 字段遮蔽 | `SerializedMessage` 嵌入了 `Message`，而 `Message` 已有 `SessionId types.SessionId` 和 `Timestamp time.Time` 字段；`SerializedMessage` 自身又重复声明了同名同类型的 `SessionId` 和 `Timestamp`。Go 嵌入时重名字段发生遮蔽（shadow），JSON 序列化结果不可预测，且编译器不报错，是隐性 bug。 | 从 `SerializedMessage` 删除与 `Message` 重复的字段。若两者语义确有不同（API 层 vs 持久化层），则不应使用嵌入，改为显式组合：`Msg Message \`json:"message"\``。 |
| **P1** | `internal/config/loader.go` `applyLayer()` 字段覆盖不完整 | `applyLayer` 仅手写了约 7 个字段的合并逻辑，注释"其余字段类似处理"。`SettingsJson` 共有 20+ 字段（`AWSCredentialExport`、`GCPAuthRefresh`、`Hooks`、`Worktree`、`EnableAllProjectMCP`、`EnabledMCPServers` 等），缺失字段在当前设计下会被**静默丢弃**，导致配置项完全不生效。 | 明确列出所有 `SettingsJson` 字段的合并语义（覆盖/追加/忽略）；或改用代码生成（`go:generate`）产生合并代码；禁止以"类似处理"注释代替实际实现。 |
| **P1** | `pkg/types/logs.go` `EntryEnvelope.Raw` 无 json tag | `Raw json.RawMessage` 字段缺少 json struct tag，默认序列化 key 为 `"Raw"`，但实际 JSONL 行中不存在此键；同时使用者可能误以为 `json.Unmarshal` 可自动填充 `Raw`，实际上需在 `ReadAll` 中手动赋值，接口语义不清晰。 | 将 `Raw` 标注为 `json:"-"`，并在字段注释中明确说明"由 `ReadAll` 在反序列化后手动填充原始行内容"。 |
| **P1** | `pkg/types/hooks.go` / `plugin.go` 完全缺失 | 文件结构 §1.2 列出了 `hooks.go` 和 `plugin.go`，但文档中无任何类型定义。Hooks 是架构文档明确的核心模块（pre/post-tool/session/sampling hooks），`SettingsJson.Hooks` 字段当前为 `map[string]any`，完全丢失类型安全，下游 Agent-Core 无法基于此实现 hook 调度。 | 补充 `hooks.go` 和 `plugin.go` 的核心类型骨架：`HookType` 枚举（`pre_tool_use`/`post_tool_use`/`session_start` 等）、`HookDefinition`（`command`、`matcher` 字段）、`HookResult`（`block`/`approve`/`modify`）；`SettingsJson.Hooks` 改为 `map[HookType][]HookDefinition`。`LoadedPlugin` 至少给出与 TS 原版对应的字段声明。 |
| **P1** | `pkg/types/command.go` `LocalCommandContext` 缺少 `context.Context` | `Call func(args string, ctx *LocalCommandContext)` 缺少 `context.Context` 参数，命令执行无法支持取消和超时。CLI 中 Ctrl-C 中止、`--max-turns` 超限等场景均需 context 传递。 | `Call` 签名改为 `func(ctx context.Context, args string, cmdCtx *LocalCommandContext) (*LocalCommandResult, error)`，与 Go 惯用法一致（context 作为第一参数）。 |
| **P2** | `pkg/utils/json/json.go` 包名遮蔽标准库 | 包名 `json` 与标准库 `encoding/json` 同名，调用方在同一文件中同时引用时必须为其中一个起别名，增加认知负担，不符合 Go 惯用法。 | 将包声明改为 `package jsonutil`（文件路径不变），消除歧义。 |
| **P2** | `internal/config/loader.go` `os.IsNotExist` 过时 API | `os.IsNotExist(err)` 在 Go 1.13+ 已标记为 legacy，无法正确处理 wrapped error（如 `fmt.Errorf("...: %w", fs.ErrNotExist)`）。 | 替换为 `errors.Is(err, fs.ErrNotExist)`（需 import `io/fs`）。 |
| **P2** | `pkg/utils/ids/ids.go` 忽略 `rand.Read` 错误 | `_, _ = rand.Read(b)` 显式丢弃错误。在受限容器环境下熵源不可用时，会静默产生全零随机字节，导致 ID 碰撞。 | 改为 `if _, err := rand.Read(b); err != nil { panic(fmt.Sprintf("crypto/rand unavailable: %v", err)) }`。此处 panic 是合理的不可恢复错误处理。 |
| **P2** | `pkg/types/permissions.go` `PermissionUpdate.Type` 裸 string | 有效值 `addRules/replaceRules/removeRules/setMode/addDirectories/removeDirectories` 仅以注释说明，缺少枚举常量定义，调用方需手写字符串字面量，易出错。同样的问题存在于 `LocalCommandResult.Type`、`TaskState.Status`。 | 定义 `type PermissionUpdateType string` + 常量；同理补全其他"裸 string 状态值"类型。 |
| **P2** | `pkg/types/permissions.go` `ToolPermissionContext` 缺少 json tag | 同包其他结构体均有 json tags，`ToolPermissionContext` 字段（如 `IsBypassPermissionsModeAvailable`）序列化时使用原始字段名，与 TS 原版 camelCase 格式不一致，跨语言互操作时会产生 key 不匹配。 | 为 `ToolPermissionContext` 所有字段补充 json tags，与 TS 原版字段名对齐。 |
| **P2** | `internal/state/app_state.go` `[]any` MCP 字段 | `MCPClients/MCPTools/MCPCommands/PluginsEnabled/PluginsDisabled` 全部使用 `[]any`，完全丢失类型安全，所有访问都需类型断言，断言失败即 panic。 | 在 `pkg/types` 中定义前向声明接口（`MCPConnection interface { ... }`、`MCPTool interface { ... }`），或将 MCP 运行时字段移出 `AppState`，通过依赖注入传递。 |
| **P2** | `internal/session/store.go` 静默跳过损坏行 | `ReadAll` 中 JSON 解析失败时 `continue` 静默跳过，无日志输出，会话文件损坏时用户无感知，调试极为困难。 | 通过注入的 logger（或 `slog.Default()`）输出 `WARN` 级日志，注明跳过的行索引和错误内容。 |
| **P2** | `pkg/types/logs.go` `EntryType` 枚举不完整 | 仅定义 5 个 EntryType 常量，TS 原版 `src/types/logs.ts` 有 20+ Entry 变体（含 `debug`、`tool_result`、`thinking` 等）。缺失的类型在 `type-switch` 时命中 default 分支，导致数据静默丢失。 | 通读 TS 原版 `src/types/logs.ts`，补全所有 EntryType 枚举值，并为每种变体提供对应的 Go 结构体声明。 |
| **P2** | `internal/bootstrap/bootstrap.go` `homeDir()` 未定义 | `Bootstrap` 内调用 `homeDir()`，但该函数未在文档中定义，实现策略不明确，在容器等 `$HOME` 未设置的环境下可能 panic 或返回空路径。 | 明确定义 `homeDir()` 实现：优先 `os.UserHomeDir()`，失败则 fallback `$HOME` 环境变量，仍失败则返回 error（而非 panic）。 |

---

## 通过条件

以下 **3 个 P0 条件**必须在 Agent-Infra 编码开始前完成（文档修订 + 设计确认），其余 P1/P2 可在编码阶段同步修复并在首个 PR 中一并解决。

### 条件一：消除 `pkg/types/ids.go` 中的 `NewAgentId` panic 存根

`pkg/types/ids.go` 中 `NewAgentId()` 函数必须**删除**。`pkg/types` 只保留零副作用的类型定义和纯转换函数（`AsAgentId`、`AsSessionId`）。ID 生成能力由 `pkg/utils/ids/ids.go` 提供，各调用方直接 import `pkg/utils/ids`。

**验收标准**：`pkg/types` 包内不存在任何会触发 panic 的导出函数体。

---

### 条件二：明确 managed-settings（Policy）的优先级语义并保持代码一致

设计文档 §3.1 描述的 Policy 优先级（"最高，用于企业管控"）与当前 `mergeSettings` 调用顺序存在矛盾。必须在文档中明确：

1. Policy 中哪些字段属于**锁定字段**（不可被任何用户级配置覆盖）；
2. `mergeSettings` 或补充的 `applyPolicyOverrides` 逻辑必须与文档语义完全一致；
3. 参考 TS 原版 `managed-settings.json` 的实际合并行为作为最终依据。

**验收标准**：文档 §3.1 的优先级说明与代码实现无矛盾；Policy 锁定字段不可被 Local/Project/User 配置覆盖。

---

### 条件三：`SetState` 中 map/slice 字段的并发安全方案明确落地

"Phase 2 再修复"不可接受，Phase 1 必须选定并文档化以下三个方案之一：

- **方案 A**：`SetState` 内对 `AppState` 所有 map 字段自动执行 `maps.Clone()`，再将副本传入 updater；
- **方案 B**：在 §4.4 示例和 API 文档中明确约定 updater 必须 copy-on-write（提供标准模板），并在 code review checklist 中强制检查；
- **方案 C**：禁止在 `AppState` 中存放可变 map，所有集合操作通过 `SetState` 返回全新 map 实现（函数式风格）。

**验收标准**：设计文档 §4.3 不再出现"暂不修复 / Phase 2 待定"，而是给出可落地的 Phase 1 保护方案，且 §4.4 示例代码中没有直接对 map 字段做 mutate 的危险用法。

---

## 评审结论

设计骨架完整，模块边界清晰，依赖方向符合架构规范（`pkg/types` 零依赖，`internal/*` 单向依赖），泛型 Store、RWMutex 锁外通知、原子文件写入等关键设计合理。

三个 P0 问题中，"ids.go panic 存根"会导致直接运行时崩溃，"SetState map 竞争"在有订阅者时必然触发 data race，"Policy 优先级倒置"会使企业安全管控形同虚设——三者均会在编码阶段造成严重后果。

**P0 条件全部满足后，设计文档视为批准，Agent-Infra 可以开始编码实现（任务 #13）。**
