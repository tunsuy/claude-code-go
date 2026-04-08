# Infrastructure Layer Code Review

> **Reviewer**: Tech Lead
> **Date**: 2026-04-03
> **Subject**: 任务 #13 实现代码（Agent-Infra）
> **Scope**: pkg/types/\*, pkg/utils/\*, internal/config/loader.go, internal/state/store.go, internal/session/store.go, internal/memdir/\*, pkg/testutil/testutil.go
> **Verdict**: APPROVED_WITH_CHANGES

---

## 1. Overall Assessment

基础设施层整体质量良好，设计意图清晰，Go 惯用法运用得当。`pkg/types` 的 "no side-effectful imports" 约束被严格遵守；`internal/state.Store[T]` 的泛型实现、锁粒度分离方案以及监听器快照策略均属上乘。`pkg/utils/fs.AtomicWriteFile` 包含 `Sync-before-Rename` 保证（力扣 N-11），`internal/session.SessionStore` 的腐坏行跳过策略符合设计文档 §5.8。

发现 **1 个 P0**（正确性/并发安全）、**4 个 P1**（接口契约/测试覆盖/错误处理）、**6 个 P2**（Go 惯用法/轻微设计建议）。P0 须于合并前修复。

---

## 2. Strengths

1. **严格的包约束**：`pkg/types` 全部文件遵守"不引入非标准库依赖"规定，`ids.go` 的注释明确说明生成逻辑由 `pkg/utils/ids` 承担，职责边界清晰。
2. **泛型 Store 并发设计**：`internal/state.Store[T]` 使用双层锁（状态读写用 `RWMutex`，监听器注册用独立 `Mutex`），并在锁外快照后回调，有效避免回调死锁。`atomic.Uint64` 生成监听器 ID 也属正确用法。
3. **AtomicWriteFile 完整性**：`Sync()` → `Close()` → `Chmod()` → `Rename()` 的顺序正确，兼顾了断电安全与权限正确性。
4. **SessionStore 健壮性**：`ReadAll` 使用 `string(scanner.Bytes())` 立即复制（解决扫描缓冲区复用问题），腐坏行 WARN 日志不暴露行内容，符合隐私要求。4 MiB 行缓冲应对大型工具结果是合理折中。
5. **Config 分层合并**：`uniqueAppend` 去重、Policy deny 规则前置、环境变量优先级高于文件但低于 Policy，三层优先级语义清晰且与 TypeScript 参考实现对齐。
6. **测试质量**：`session/store_test.go` 覆盖了空文件、腐坏行跳过、Raw 字节复制、New/Resume 生命周期四个关键路径；`permission/matcher_test.go` 包含大小写敏感、前缀、通配符等边界用例，表驱动写法规范。

---

## 3. Issues

### P0 — Must Fix Before Merge

#### P0-1 · `internal/session.newSessionId()` 存在时间戳碰撞风险且与 `pkg/utils/ids.NewSessionId` 格式不一致

**文件**: `internal/session/store.go`, 第 199–201 行

```go
func newSessionId() types.SessionId {
    return types.AsSessionId(fmt.Sprintf("%d", time.Now().UnixMilli()))
}
```

**问题**：
- 该函数仅使用毫秒时间戳，**没有随机后缀**。在同一毫秒内连续创建两个 Session（例如并发测试、批量脚本），将产生**相同的 SessionId**，导致两个 JSONL 文件互相覆写——数据静默丢失，属严重正确性 Bug。
- 与 `pkg/utils/ids.NewSessionId()`（格式 `<ms>-<16hex>`）不一致，破坏了"格式规范唯一"的不变量。注释中声称"replicated here to avoid a dependency"，但实际上 `internal/session` 已经依赖同模块的 `pkg/utils/fs`，没有任何技术障碍可以直接 `import pkg/utils/ids`。

**修复方案**：删除本地 `newSessionId()` 函数，直接调用 `ids.NewSessionId()`（`import github.com/anthropics/claude-code-go/pkg/utils/ids`）。

---

### P1 — Should Fix Soon

#### P1-1 · `pkg/utils/fs.isNotExist` 手写展开链，stdlib 已有等价函数

**文件**: `pkg/utils/fs/fs.go`, 第 79–97 行

```go
func isNotExist(err error) bool {
    return err != nil && (os.IsNotExist(err) || isErrNotExist(err))
}

func isErrNotExist(err error) bool {
    unwrapped := err
    for unwrapped != nil {
        if unwrapped == fs.ErrNotExist { return true }
        ...
    }
    return false
}
```

**问题**：Go 1.13+ 的 `errors.Is(err, fs.ErrNotExist)` 已内置 unwrap 展开逻辑，与上述手写等价。多出约 20 行代码，且 `unwrapped == fs.ErrNotExist` 使用了`==`值比较而不是 `errors.Is`，在某些包装器下存在等价性语义差异。

**修复方案**：将两个私有函数合并为 `errors.Is(err, fs.ErrNotExist)`，并在 `SafeReadFile` 中直接使用。

---

#### P1-2 · `internal/config/loader.go` `applyLayer` 未处理 `Hooks` 的顺序覆盖语义

**文件**: `internal/config/loader.go`, 第 275–283 行

```go
for ht, defs := range src.Hooks {
    dst.Hooks[ht] = append(dst.Hooks[ht], defs...)
}
```

**问题**：对 Hooks 采用 `append` 合并，而 `applyPolicyOverrides` 对 Policy Hooks 采用完全替换（第 371–373 行）。设计文档（`docs/project/design/infra.md`）并未明确约定普通层 Hooks 是 "append" 还是 "override" 语义；但 TypeScript 参考实现在 project/local 层也是追加（正确）。主要风险是**当 Local 层需要覆盖 User 层 Hook 时无法做到**，可能影响测试/CI 场景。

**修复方案**：在 `SettingsJson` 或文档中明确 Hooks 的合并语义，如有 "替换" 需求，应提供 `hooks_overrides` 字段而非修改 `append` 行为。当前语义至少要在注释中写清楚。

---

#### P1-3 · `internal/config/loader.go` `applyEnvOverrides` 不可注入，妨碍单元测试

**文件**: `internal/config/loader.go`, 第 380–393 行

```go
func applyEnvOverrides(s *SettingsJson) {
    if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
```

**问题**：函数直接调用 `os.Getenv`，无法在测试中注入自定义环境（除非 `t.Setenv`，但这在并发测试中有全局副作用）。`Loader` 没有 `environ` 注入点。

**修复方案**：在 `Loader` 结构体中增加 `environ func(string) string` 字段（默认为 `os.Getenv`），在测试中可替换为自定义函数。这是标准的依赖注入模式，对调用者零破坏（`NewLoader` 不需要改签名，提供 `NewLoaderWithEnv` 变体即可）。

---

#### P1-4 · `pkg/testutil/testutil.go` 功能极度匮乏，实用价值不足

**文件**: `pkg/testutil/testutil.go`

**问题**：`testutil` 包仅有 `AssertNoError` / `AssertError` 两个函数，而整个基础设施层的测试都依赖它作为共享工具库。实际上 `store_test.go` 甚至没有使用它（测试文件直接用 `t.Fatalf`）。缺少：
- `MustTempDir(t)` — 包装 `t.TempDir()`，避免每个测试文件重复 boilerplate；
- `AssertEqual[T comparable](t, got, want T)` — 基本值比较断言；
- `MustWriteFile(t, path, content)` — 原子写临时文件帮助函数。

**修复方案**：扩充 `testutil` 以涵盖基础层测试的常见模式，减少各包测试文件中的重复代码。这不影响任何已有接口。

---

### P2 — Minor / Suggestions

#### P2-1 · `pkg/types/hooks.go` `HookDefinition` 缺少 `Callback` 字段

**文件**: `pkg/types/hooks.go`, 第 31–38 行

```go
type HookDefinition struct {
    Command   string `json:"command"`
    Matcher   string `json:"matcher,omitempty"`
    TimeoutMs int    `json:"timeout_ms,omitempty"`
}
```

文件注释第 60 行定义了 `HookCallback`，并注明 "Either Command (shell) or Callback (Go func) is set per HookDefinition"，但 `HookDefinition` 结构体中没有 `Callback HookCallback` 字段。Go 原生 Hook 机制目前无法通过 `HookDefinition` 注册，只能在 `LoadedPlugin.Hooks` 层绕过。建议将 `Callback HookCallback \`json:"-"\`` 添加到 `HookDefinition`，使两种模式使用同一结构。

---

#### P2-2 · `internal/state/store.go` `AppState.MCPTools / MCPCommands` 类型为 `[]any`

**文件**: `internal/state/store.go`, 第 150–151 行

```go
MCPTools    []any `json:"-"` // TODO(dep): typed once Agent-MCP defines the Tool interface
MCPCommands []any `json:"-"`
```

`[]any` 会导致取出后需要类型断言，且编译器无法提供任何静态安全保证。TODO 标注已有意识到这个问题，建议在 `pkg/types` 中预先定义 `MCPTool` 和 `MCPCommand` 接口（即便是 minimal 的 `ID() string` + `Name() string`），然后将这两个字段改为 `[]types.MCPTool` 和 `[]types.MCPCommand`，以便 Agent-Services 实现时直接满足接口，无需回头修改 AppState。

---

#### P2-3 · `internal/memdir/loader.go` `LoadAndTruncate` 截断位置可能切断 UTF-8 多字节字符

**文件**: `internal/memdir/loader.go`, 第 38–43 行

```go
cutAt := maxBytes - len(notice)
return full[:cutAt] + notice
```

`full[:cutAt]` 是字节切片，若 `cutAt` 落在 UTF-8 多字节字符中间，`strings.Builder` 写入的结果将包含非法 UTF-8 序列，可能导致下游 JSON 序列化出现 `\uFFFD` 替换字符或编码错误。

**修复方案**：使用 `utf8.ValidString(full[:cutAt])` 检查后回退，或使用 `strings.ToValidUTF8` 处理截断点。

---

#### P2-4 · `pkg/utils/permission/matcher.go` `MatchPathRule` 前缀匹配过于宽泛

**文件**: `pkg/utils/permission/matcher.go`, 第 40–42 行

```go
// Prefix match
if strings.HasPrefix(path, pathRule) {
    return true
}
```

示例：规则 `/tmp/foo`，路径 `/tmp/foobar/baz`，因为 `strings.HasPrefix` 成立，`/tmp/foobar/baz` 会被错误允许，但直觉上该规则应仅覆盖 `/tmp/foo` 目录树。

**修复方案**：前缀匹配应确保分隔符边界：

```go
if path == pathRule || strings.HasPrefix(path, pathRule+"/") {
    return true
}
```

注意：当前测试用例恰好没有覆盖这个边界情形（`/tmp/foo` vs `/tmp/foobar`），需同步补充测试。

---

#### P2-5 · `internal/config/settings.go` 为空文件

**文件**: `internal/config/settings.go`

```go
// All types and constants for the config package are defined in loader.go.
// This file intentionally left with only the package declaration.
package config
```

空文件在 Git 历史中制造噪音，且与注释矛盾（文件名暗示此处应有 settings 相关内容）。建议删除该文件，或将 `SettingsJson` 及相关类型从 `loader.go` 迁移到此，保持"loader 专注于加载逻辑，settings 专注于数据结构"的分层意图。

---

#### P2-6 · `pkg/utils/ids/ids.go` `NewAgentId` prefix 未校验非法字符

**文件**: `pkg/utils/ids/ids.go`, 第 28–38 行

```go
func NewAgentId(prefix string) types.AgentId {
    ...
    return types.AgentId(fmt.Sprintf("a%s-%s", prefix, suffix))
}
```

若调用方传入包含 `-` 的 prefix（如 `"foo-bar"`），生成结果为 `afoo-bar-<16hex>`，格式为 `a(?:.+-)?[0-9a-f]{16}` 仍然匹配（贪婪），但可读性较差。若传入包含 `[^0-9a-f\-a-zA-Z]` 的字符，生成的 AgentId 将通不过 `types.AsAgentId` 校验。

**修复方案**：在 `NewAgentId` 中对 prefix 做简单校验（`regexp` 或 `strings.ContainsAny`），并在 prefix 非法时 `panic` 或返回 `error`，使问题在调用侧而非验证侧暴露。

---

## 4. Summary

| Priority | Count | Items |
|----------|-------|-------|
| P0 | 1 | Session ID 碰撞 + 格式不一致 |
| P1 | 4 | `isNotExist` 冗余、Hooks 合并语义未文档化、`applyEnvOverrides` 不可测试注入、`testutil` 匮乏 |
| P2 | 6 | `HookDefinition` 缺 Callback 字段、`[]any` MCP 字段、UTF-8 截断、路径前缀过宽、空 settings.go、AgentId prefix 未校验 |

**P0 是阻塞项**：`newSessionId()` 的毫秒精度无随机性，在高频调用下必然产生碰撞，导致会话数据静默覆盖。修复成本极低（直接 import `pkg/utils/ids`），请在合并前处理。

P1 中的 **P1-3（环境变量注入）** 和 **P1-4（testutil 扩充）** 对后续其他层的测试质量有连带影响，建议在本迭代内同步跟进。

P2 条目均不阻断功能，可纳入下一个 sprint 处理，但 **P2-4（路径前缀宽泛）** 属权限安全相关，建议提前处理以避免误授权。

整体而言，该层实现已达到可用质量，修复上述问题后可正式合并进主干。

## 修复跟踪记录

| 问题编号 | 级别 | 描述摘要 | 状态 | 复核时间 | 备注 |
|---------|------|---------|------|---------|------|
| P0-CR-9 | P0 | `newSessionId()` 毫秒时间戳碰撞风险 | ✅ 已修复 | 2026-04-03 | 改调 `ids.NewSessionId()`，内部使用 `crypto/rand` 后缀 |

> **本层评审通过，通知 PM。**
