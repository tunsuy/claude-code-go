# 基础设施层设计评审

> 评审人：Tech Lead
> 日期：2026-04-02
> 结论：**有条件通过**

---

## 总体评价

基础设施层设计整体思路清晰，TS → Go 的类型映射合理，三级配置加载、并发安全 Store、JSONL 会话持久化的核心骨架已具备可实施性。文档具备较高的完成度，可作为编码实现的起点。

但评审过程中发现若干**设计缺陷与遗漏**，部分属于会在编码阶段引发 bug 或编译失败的严重问题，需要在开始实现前明确解决方案。整体结论为**有条件通过**，修复下列 P0/P1 问题后可启动编码。

---

## 问题清单

| 严重级别 | 位置 | 问题描述 | 建议 |
|---------|------|---------|------|
| **P0** | `pkg/types/ids.go` `NewAgentId()` | 函数体为 `panic("see internal/bootstrap")`，但同时在 `pkg/utils/ids/ids.go` 中又有完整的 `NewAgentId` 实现。`pkg/types` 是零依赖类型包，不应承担生成逻辑；当前写法会在任何调用者调用 `types.NewAgentId()` 时直接 panic，且无法 import `pkg/utils/ids`（循环依赖）。 | 删除 `pkg/types/ids.go` 中的 `NewAgentId()` 函数，仅保留 `AsAgentId()`/`AsSessionId()` 两个转换函数；生成逻辑完全收归 `pkg/utils/ids`。 |
| **P0** | `internal/state/store.go` `SetState` + 4.3 AppState | `SetState` 使用 `func(prev T) T` 的值语义 updater，但 `AppState` 中含有 `map`（Tasks、AgentNameRegistry 等）和 `slice`（MCPClients 等）字段，这些是引用类型——"返回修改后的结构体值" 后旧值与新值共享同一 map/slice 底层数组，`s.mu.Unlock()` 之后 `onChange/notifyListeners` 传出的 `prev` 依然指向相同的 map，导致**读到已变更的旧状态**，竞争条件在有订阅者时必现。 | Phase 1 至少对 map 字段做浅拷贝（`maps.Clone`），或在 `SetState` 文档中明确禁止 updater 直接 mutate map；4.4 节的"示例"代码中直接 `prev.Tasks[taskId] = ...` 正是危险用法，需加警告并给出正确示例（先 `maps.Clone`，再赋值）。 |
| **P0** | `internal/config/loader.go` `Load()` | `for src, path := range paths` 遍历 `map` 时迭代顺序不确定，但 `mergeSettings` 依赖固定的层级顺序（Policy < User < Project < Local）。若迭代顺序随机，读取文件的顺序虽不影响合并（合并在 `mergeSettings` 中按参数顺序），但 `ptrs` 赋值本身无顺序问题——**真正的 P0 是**：`os.IsNotExist(err)` 在 Go 1.13+ 已废弃，应改用 `errors.Is(err, os.ErrNotExist)`；旧 API 对 wrapped error 无法正确检测，会导致配置文件缺失时返回错误而非静默跳过。 | 将 `os.IsNotExist(err)` 替换为 `errors.Is(err, os.ErrNotExist)`（全文一致）。 |
| **P1** | `internal/state/store.go` `Subscribe()` | 取消订阅通过"将切片对应位置置 nil"实现，但切片只增不减，长期运行后存在内存泄漏（nil 槽永远不会被回收）。且 `idx` 在闭包中捕获，当切片扩容（`append` 触发底层数组重分配）后 `idx` 依然有效，但若两个 goroutine 同时 `Subscribe`，存在 `append` 与 `idx` 读取的竞争。 | 改用 `map[uint64]Listener[T]` 加自增 ID 管理订阅者；取消时从 map 中 delete，彻底消除泄漏与竞争。 |
| **P1** | `internal/config/loader.go` `applyLayer()` | `applyLayer` 仅手写了少量字段（`Model`、`APIKeyHelper`、`DefaultShell`、`RespectGitignore`、`CleanupPeriodDays`、`Env`、`Permissions`），注释"其余字段类似处理"——`SettingsJson` 共有 20+ 字段，大量字段（`AWSCredentialExport`、`GCPAuthRefresh`、`Hooks`、`Worktree`、`EnableAllProjectMCP`、`EnabledMCPServers` 等）缺乏合并逻辑，在当前设计下会被静默丢弃。 | 明确列出所有字段的合并语义；或改用反射/code-gen 生成合并代码；至少在文档中补全所有字段的处理策略（覆盖/追加/忽略）。 |
| **P1** | `pkg/types/logs.go` `EntryEnvelope` | `EntryEnvelope.Raw` 字段缺少 `json:"..."` tag，在 JSON 反序列化时 `Raw` 不会被自动填充——当前 `ReadAll` 中是手动赋值 `env.Raw = json.RawMessage(line)`，但 `EntryEnvelope` 作为公开类型，使用者会误以为可直接通过标准 `json.Unmarshal` 填充 `Raw`。 | 为 `Raw` 添加 `json:"-"` 明确标记为非 JSON 字段，并在文档注释中说明需由调用方手动赋值；或改名为 `RawLine` 并加注释说明。 |
| **P1** | `pkg/types/command.go` `LocalCommandContext` | `AppStateReader` 接口定义在 `pkg/types` 包内，但其方法 `GetPermissionContext() ToolPermissionContext` 返回的是同包类型，这本身没问题；**问题在于** `LocalCommandContext.AppState` 字段名与 `internal/state.AppStateStore` 存在概念混淆——命令层只需要只读快照，但接口未限制写操作（缺少 `context.Context` 参数），导致命令执行时无法支持取消/超时。 | `LocalCommandContext` 增加 `Ctx context.Context` 字段；`Call` 函数签名应包含 `context.Context` 参数以支持取消。 |
| **P1** | `internal/session/store.go` `SessionStore` | `AppendEntry` 在加锁前先调用 `json.Marshal`（CPU 密集），这是正确的；但 `file.Write` 不保证原子性（write syscall 可能被拆分），对于 JSONL 格式，一行 JSON 若写入不完整将导致文件损坏，且无 `Sync`/`Flush` 调用，进程崩溃时最后若干行会丢失。 | 对于追加写入，Linux/macOS 的 `O_APPEND` + 单次 `write` 对于小于 PIPE_BUF（4096 字节）的数据是原子的，但消息可能超过此限制；建议改用 `bufio.Writer` + 定期 `Flush`，或在 `Close` 前调用 `file.Sync()`；同时在文档中说明崩溃恢复策略（`ReadAll` 中跳过损坏行已处理）。 |
| **P2** | `pkg/types/permissions.go` `PermissionUpdate.Type` | `Type` 字段为裸 `string`，文档注释标注了有效值（`addRules|replaceRules|...`），但没有对应的枚举常量定义，调用方必须手写字符串字面量，易出错且无法 IDE 补全。 | 新增 `PermissionUpdateType string` 类型及对应常量（`PermissionUpdateAddRules` 等）。 |
| **P2** | `internal/state/app_state.go` `AppState` | `MCPClients/MCPTools/MCPCommands/PluginsEnabled/PluginsDisabled` 全部用 `[]any` 并打 `json:"-"`，丢失了类型安全。当 `internal/state` 包需要操作这些字段时，调用者必须进行类型断言，极易引入运行时 panic。 | 定义前向声明接口（如 `MCPConnection interface{ ... }`）放在 `pkg/types` 或 `internal/state` 中，或至少使用具体指针类型（即便以注释说明"未来替换"）；避免 `[]any`。 |
| **P2** | `pkg/utils/json/json.go` 包名冲突 | 包名为 `json`，与标准库 `encoding/json` 同名，虽然内部已用 `stdjson` 别名规避，但**调用方**在同一文件中同时 import `encoding/json` 和本包时，必须给其中一个起别名，降低可读性。 | 将包名改为 `jsonutil` 或 `jsonlines`，文件保留在 `pkg/utils/json/` 目录但 `package jsonutil`。 |
| **P2** | `internal/config/loader.go` `LayeredSettings.Merged` 注释 | 注释写"合并后的有效配置（Local > Project > User > Policy）"，但 `mergeSettings` 的调用顺序是 `mergeSettings(ls.Policy, ls.User, ls.Project, ls.Local)`，即 Policy 优先级最低（最先被覆盖）；而架构文档（§4.4）和设计文档（§3.1）描述的企业策略（Policy）优先级应**最高**。注释与代码逻辑互相矛盾，至少有一处是错的。 | 核实 TS 原版 `managed-settings.json` 的优先级语义，统一注释与代码；若 Policy 确为最高优先级，则调用顺序应改为 `mergeSettings(ls.User, ls.Project, ls.Local, ls.Policy)`（Policy 最后 = 最高覆盖）。 |
| **P2** | `internal/bootstrap/bootstrap.go` `homeDir()` | `Bootstrap` 中调用了 `homeDir()`，但该函数未在设计文档中定义；不清楚是 `os.UserHomeDir()` 封装还是读环境变量，存在平台差异（容器环境中 `$HOME` 可能未设置）。 | 在 `bootstrap.go` 中明确定义 `homeDir()` 的实现策略，优先 `os.UserHomeDir()`，失败时 fallback 到 `$HOME`，仍失败则返回错误（而非 panic）。 |
| **P2** | `pkg/types/logs.go` Entry 类型不完整 | `EntryType` 常量仅列出 5 个（`transcript`/`summary`/`tag`/`pr_link`/`worktree_state`），注释"其余类型"，但 TS 原版 `src/types/logs.ts` 中有 20+ Entry 变体（含 `debug`、`tool_result`、`thinking`、`reasoning` 等），未定义的类型在 `ReadAll` 时会被 `EntryEnvelope` 解析为未知类型枚举值，调用方无法正确 type-switch。 | 补全所有 TS 原版中出现的 `EntryType` 枚举值，并为每个变体提供对应的具体结构体（至少在 logs.go 中列出声明）。 |
| **P2** | `pkg/types/hooks.go` / `plugin.go` | 文档仅列出文件名，无任何代码实现或类型定义，属于完全空白的模块。Hooks 在架构文档中是独立的核心层模块，Plugin 类型也有跨层引用需求。 | 补充 `hooks.go` 和 `plugin.go` 的类型设计，至少给出与 TS 原版对应的核心类型骨架（`HookEvent`、`HookCallback`、`HookResult`、`LoadedPlugin` 等）。 |

---

## 通过条件

在开始 Agent-Infra 编码实现（任务 #13）之前，须满足以下条件：

### 必须修复（阻塞编码启动）

1. **[P0-1]** 删除 `pkg/types/ids.go` 中的 `NewAgentId()` panic 存根，避免调用者运行时崩溃。
2. **[P0-2]** 在 `SetState` 文档和 4.4 节示例中，明确说明 map/slice 字段必须先浅拷贝再修改，并提供正确示例代码（`maps.Clone`）；或在 `SetState` 实现中自动对 map 字段做防御性拷贝。
3. **[P0-3]** 将 `os.IsNotExist` 替换为 `errors.Is(err, os.ErrNotExist)`。

### 应当修复（编码前明确，不阻塞但需在 PR 前完成）

4. **[P1-1]** 将 `Subscribe` 的订阅者管理从"nil 槽切片"改为"map + 自增 ID"，消除内存泄漏。
5. **[P1-2]** 在 `applyLayer` 中补全所有 `SettingsJson` 字段的合并逻辑，或在文档中明确列出哪些字段暂缓实现及原因。
6. **[P1-3]** 为 `LocalCommandContext.Call` 增加 `context.Context` 参数。
7. **[P1-4]** 澄清 `LayeredSettings.Merged` 注释与代码中 Policy 优先级的矛盾，保持一致。

### 建议改进（可在后续迭代中处理）

8. **[P2]** 将 `pkg/utils/json` 包名改为 `jsonutil`，避免与标准库冲突。
9. **[P2]** `PermissionUpdate.Type` 增加枚举常量。
10. **[P2]** `AppState` 中的 `[]any` 字段改用前向声明接口。
11. **[P2]** 补全 `EntryType` 枚举，补充 `hooks.go` 和 `plugin.go` 骨架。

---

## 评审结论

设计文档骨架完整，核心模块划分符合架构规范，依赖方向总体正确（`pkg/types` 零依赖、`internal/*` 单向依赖 `pkg/types`），Go 惯用法运用基本合理（泛型 Store、RWMutex 保护、原子写文件等）。

**P0 问题有 3 个**，其中"ids.go panic 存根"和"map 引用语义竞争条件"如果不修复，会在编码阶段直接导致运行时崩溃或 data race，必须在动笔前解决。

**条件满足后**，设计文档视为批准，Agent-Infra 可以开始编码实现（**任务 #13**）。
