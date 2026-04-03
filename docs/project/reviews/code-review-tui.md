# TUI Layer Code Review

> **Reviewer**: Tech Lead
> **Date**: 2026-04-03
> **Subject**: 任务 #17 实现代码（Agent-TUI）— `internal/tui/` + `internal/commands/`
> **Verdict**: APPROVED_WITH_CHANGES

---

## 1. Overall Assessment

TUI 层整体结构清晰，严格遵循 BubbleTea 的 MVU（Model-Update-View）范式，15 个文件职责划分合理。已知的 P0-G（`/model` 切换静默失败）已在 `update.go` 的 `applyCommandResult` 中正确修复：读取 `result.NewModel`、调用 `m.queryEngine.SetModel`、同步更新 `m.statusBar.model` 与 `m.appState`，三处联动完整。

流式事件映射（`engine.Msg` → BubbleTea `tea.Msg`）在 `cmds.go` 中通过 channel-pull 模式实现，每个 `waitForStreamEvent` Cmd 只消费一条事件后立即返还控制权，符合 BubbleTea 单线程事件循环要求。

主要待改进点集中在：**滚动逻辑存在 BUG**、**`/compact` 流程存在语义绕路**、**`glamour` 渲染器每次消息都重建导致性能隐患**、**测试覆盖率为零**，以及若干轻量级问题。

---

## 2. Strengths

1. **MVU 模式干净**：`AppModel` 全量值语义传递，`Update` 返回新 model + Cmd，`View` 纯函数渲染，符合 BubbleTea 设计原则，无副作用注入。

2. **流式事件处理正确**：`streamChanReady` 内部消息类型将 channel 安全地注入 Update 循环；`waitForStreamEvent` 以 per-event Cmd 方式逐条拉取，避免了 goroutine 泄漏与竞态。`isActiveStream` 与 `m.streamCh == nil` 的双重保护防止过期 channel 继续被消费。

3. **`applyCommandResult` 三联更新完整**（已修复 P0-G）：`SetModel` / `statusBar.model` / `appState` 三处同步更新，确保引擎、UI、持久化状态一致。

4. **权限对话框流程正确**：`PermissionRequestMsg.RespFn` 回调设计避免了跨 goroutine channel 阻塞；`Esc` = 拒绝、`Enter` = 按当前选项确认，UI 语义清晰，键盘路由在 `keys.go` 中正确优先于普通输入处理。

5. **主题系统设计合理**：`Theme` 作为值类型在 model 中存储，`BuiltinThemes` map 驱动 `/theme` 切换，`styles.go` 中的 style helper 全部接受 `Theme` 参数而非依赖全局状态，利于测试和主题化扩展。

6. **Vim 模式基础键位完整**：`i/a/A/o/h/j/k/l/0/$/x/dd/yy` 均已实现；`pendingKey` 机制支持双键序列；Esc 切换 Normal 模式符合预期。

7. **Coordinator Panel 独立渲染**：`CoordinatorPanel.View` 为纯函数，与主 model 通过 `AgentStatusMsg` 解耦，扩展友好。

---

## 3. Issues

### P0 — Must Fix Before Merge

*None*（P0-G 已修复，无新增 P0 问题）

---

### P1 — Should Fix Soon

#### P1-A: 滚动逻辑存在方向错误 BUG (`view.go:29-42`)

**文件**: `view.go`, 第 29-41 行

当前实现：
```go
if m.pinnedToBottom || m.scrollOffset == 0 {
    sb.WriteString(msgView)
} else {
    lines := strings.Split(msgView, "\n")
    start := 0
    if m.scrollOffset < len(lines) {
        start = len(lines) - m.scrollOffset
        if start < 0 { start = 0 }
    }
    visible := lines[start:]
    sb.WriteString(strings.Join(visible, "\n"))
}
```

**问题**：`scrollOffset` 越大，`start` 越小（更接近顶部），即"向上滚动"实际上显示的是更靠顶端的内容。但 `keys.go` 中 `KeyPgUp` 增加 `scrollOffset`、`KeyPgDown` 减少，语义是"PgUp 向上看历史"。把上述逻辑代入：PgUp 后 `scrollOffset+10`，`start = len(lines)-scrollOffset` 减小，`visible` 从更早的行开始——方向正确。

然而 `visible := lines[start:]` 会显示从 `start` 到末尾的**所有**行，当 `scrollOffset` 较小时会展示大量内容，没有按 `termHeight` 截断窗口，导致一屏内容随滚动量呈非线性变化。

**期望行为**：应裁剪为固定视口高度（`m.termHeight - statusBarLines - inputLines`），即：
```
visible = lines[start : min(start+viewportHeight, len(lines))]
```

**影响**：页面滚动体验混乱，内容渲染超出终端高度。

---

#### P1-B: `/compact` 流程绕过命令语义，存在双重触发路径 (`update.go:204-207`, `keys.go:71-84`)

**文件**: `update.go`, 第 204-207 行；`keys.go`, 第 71-84 行

`cmdCompact` 在 `builtins.go` 中返回 `Result{Text: "compact", Display: DisplayNone}`。`applyCommandResult` 在处理 `DisplayNone` 分支之前**先**用 `name == "compact"` 做了硬编码拦截，跳过了 `DisplayNone` 分支，只打开对话框：
```go
if name == "compact" {
    m.activeDialog = dialogCompact
    return m, nil
}
```
但 `DisplayNone` 分支（第 228-234 行）逻辑是"若有 Text 则作为 user query 发给引擎"，而 `compact` 的 `Text` 是 `"compact"` 字符串，这意味着**如果没有那个 `if name == "compact"` 拦截**，`/compact` 会被当成普通用户消息发送给 LLM，产生严重误操作。

更合理的设计：在 `Result` 上增加 `ShowDialog` 字段（或专用 `Action` enum），由 command 本身声明意图，而非由 TUI 用 `name` 字符串硬编码匹配。当前方式脆弱——若有人将命令重命名或注册同名命令，行为将悄然错误。

**建议**：在 `commands.Result` 添加 `OpenCompactDialog bool` 字段，或引入 `ActionCompact` Action 枚举值，消除 magic string 判断。

---

#### P1-C: `glamour` Renderer 每次消息渲染都重建，性能开销 O(n) 每帧 (`messagelist.go:12-30`)

**文件**: `messagelist.go`, 第 12-30 行

```go
func renderMarkdown(md string, width int, dark bool) string {
    r, err := glamour.NewTermRenderer(
        glamour.WithStylePath(style),
        glamour.WithWordWrap(width),
    )
    ...
}
```

`renderMarkdown` 每次调用都构造一个新 `glamour.TermRenderer`。`View()` 在每帧都会为所有历史消息调用此函数（含流式实时更新），当对话历史增长后，每帧的渲染成本是 `O(n_messages)`，且每条消息都包含 glamour 初始化开销（CSS 解析、样式加载）。在长会话中可能导致明显卡顿。

**建议**：在 `AppModel` 中缓存一个 `glamour.TermRenderer`（或 `MessageListView` 接受预建 renderer），在 `termWidth`、`darkMode` 变化时重建，其余帧复用。

---

#### P1-D: `doAbort` 不清除 `abortFn`、不清零 `streamCh`，可能造成流程状态残留 (`keys.go:206-212`)

**文件**: `keys.go`, 第 206-212 行

```go
func (m AppModel) doAbort() (tea.Model, tea.Cmd) {
    abortCmd := abortQueryCmd(m.queryEngine)
    m.isLoading = false
    m.showSpinner = false
    m.spinner = m.spinner.Reset()
    return m, abortCmd
}
```

`doAbort` 重置了 `isLoading`/`showSpinner`/`spinner`，但**未清零 `m.abortFn`、`m.abortCtx`、`m.streamCh`**。`abortQueryCmd` 调用 `qe.Interrupt`，引擎会中断并关闭 channel，随后 `waitForStreamEvent` 读到 closed channel 会发送 `StreamDoneMsg{}`，触发 `update.go:76-92` 正常清理路径（该路径会清零这些字段）。

**条件性问题**：如果引擎中断后 channel 未正常关闭（e.g. 引擎 BUG），`m.streamCh` 和 `m.abortFn` 将永久残留，导致下次 `startQueryCmd` 调用 `m.abortFn()` 取消一个已经失效的 context，且 `m.streamCh` 指向旧 channel 使后续 `StreamTokenMsg` 检查通过。建议在 `doAbort` 中也主动清零，作为防御性保障：
```go
m.abortFn = nil
m.streamCh = nil
```

---

#### P1-E: `MemdirLoadedMsg` 仅丢弃数据，CLAUDE.md 内容未注入引擎系统提示 (`update.go:29-31`, `init.go:37-43`)

**文件**: `update.go`, 第 29-31 行

```go
case MemdirLoadedMsg:
    // Paths are available here; no action needed for display.
    return m, nil
```

`loadMemdirCmd` 发现了 CLAUDE.md 文件路径并打包到 `MemdirLoadedMsg.Paths`，但 `Update` 对此消息**完全不做处理**。CLAUDE.md 内容永远不会被读取并注入引擎系统提示。这是功能性遗漏——该特性在任务文档和 TS 原版中均为核心特性（项目级指令注入）。

**建议**：在 `MemdirLoadedMsg` 处理中读取文件内容，调用 `m.queryEngine.SetSystemPrompt(content)` 或等效接口。

---

### P2 — Minor / Suggestions

#### P2-A: `newSystemMessage` 使用 `RoleAssistant` 混淆角色语义 (`model.go:173-179`)

```go
func newSystemMessage(text string) types.Message {
    return types.Message{
        Role: types.RoleAssistant, // displayed as assistant but muted
        ...
    }
}
```

系统消息（错误提示、`/help` 输出等）使用 `RoleAssistant` 会混入对话历史，在调用 `m.queryEngine.SetMessages` 同步时可能将 TUI-only 的系统提示传给 LLM。建议添加 `types.RoleSystem` 或在发送给引擎前过滤掉非 `RoleUser`/`RoleAssistant` 消息。

#### P2-B: Tab 补全只取第一个匹配项，不支持循环补全 (`keys.go:187-202`)

```go
// Autocomplete to the first match.
m.input = m.input.SetValue("/" + matches[0].Name + " ")
```

连续按 Tab 不会在候选项间循环，只会补全到 `matches[0]`。`model.go` 中已定义了 `slashSuggestions` 结构体（含 `selected` 字段），但未连接到 Tab 处理逻辑。应利用已有结构实现循环补全。

#### P2-C: `stripANSI` 实现不完整，对非 `m` 终结符的 ANSI 序列（如 `[2J`、`[H`）不处理 (`coordinator.go:141-158`)

```go
func stripANSI(s string) string {
    ...
    if inEsc {
        if r == 'm' {
            inEsc = false  // 只识别颜色序列 ESC[...m
        }
        continue
    }
    ...
}
```

ANSI 序列以字母结尾（不仅是 `m`），此实现会将 `[2J` 中的 `2` 和 `J` 当作非 ANSI 内容写入输出。在含有光标控制序列时 `lipglossWidth` 计算将偏差，导致 `CoordinatorPanel` 行对齐错位。可使用标准正则 `\x1b\[[0-9;]*[A-Za-z]` 替代。

#### P2-D: `slashSuggestions` 类型定义在 `model.go` 中但未使用

`model.go` 第 88-98 行定义了 `slashSuggestions` 和 `commandDisplayEntry`，但 `AppModel` 中没有这两个字段，也没有任何地方引用它们。要么补充到 model 中实现 Tab 补全 UI，要么删除以避免混淆。

#### P2-E: Vim Normal 模式下 `G` / `gg` 跳转未实现 (`input.go`)

Vim 标准的 `G`（跳到末尾）、`gg`（跳到开头）、`w`/`b`（按词移动）均未实现。当前已有 `pendingKey` 机制可扩展支持，但 `yy` 等现有双键序列处理后直接 `return m, nil`，未给出任何用户反馈。建议在 Normal 模式中显示一个临时的"未实现"提示或直接补充常用命令。

#### P2-F: `handleTabCompletion` 在非 slash 前缀时将 Tab 转发给 textarea，但 textarea 默认将 Tab 插入为制表符

`keys.go` 第 192 行：
```go
m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyTab})
```
对于普通输入（非 `/` 开头），Tab 被传递给 `textarea`，结果是在文本中插入 `\t`。对于聊天输入框来说，这通常不是期望行为。建议直接 `return m, nil` 或在此处实现缩进逻辑。

#### P2-G: `StatusBar` 的 `tokenUsage` 和 `cost` 字段初始化后从未更新

`statusBar.go` 中定义了 `TokenUsage` 和 `cost float64`，但 `AppModel` 中没有任何 `Update` 路径在收到 `MsgTypeTurnComplete`（含 `InputTokens`、`OutputTokens` 等字段）时更新这些字段。状态栏的 token 计数永远显示 `0 tok · $0.0000`。

**建议**：在 `update.go` 中为 `StreamDoneMsg` 或新增的 `TurnCompleteMsg` 处理器中更新 `m.statusBar.tokenUsage`。

#### P2-H: `abortQueryCmd` 使用 `context.Background()` 而非应有的引擎内部 abort 机制

```go
func abortQueryCmd(qe engine.QueryEngine) tea.Cmd {
    return func() tea.Msg {
        qe.Interrupt(context.Background())
        return nil
    }
}
```

`Interrupt` 接受一个 context 参数但实现上用的是 `context.Background()`，语义上 Interrupt 操作本身不应被取消，这是 OK 的。但返回 `nil` 意味着 BubbleTea 收到 `nil` 消息——这是合法的（BubbleTea 忽略 nil msg），但不够明确。建议返回一个专用的 `AbortedMsg{}` 以便将来扩展（如显示"Interrupted"提示）。

#### P2-I: `coordinatorPanel.Tasks` map 只增不减，没有 eviction 逻辑

`AgentTaskState` 有 `EvictAfter *time.Time` 字段，但 `TickMsg` 处理器（`update.go:20-27`）只更新 spinner，从不遍历 `Tasks` map 驱逐过期任务。对于长时间运行的 coordinator 会话，已完成任务会持续占用内存和 UI 空间。

#### P2-J: `renderConfirmDialog` 中 `width` 参数接收但被 `_ = width` 丢弃 (`view.go:86-95`)

```go
func renderConfirmDialog(title, body string, width int, theme Theme) string {
    ...
    _ = width
    ...
}
```

对话框内容未使用终端宽度，在窄终端上可能溢出。应利用 `width` 参数添加 `lipgloss` 样式约束（`MaxWidth(width - 4)`）。

---

## 4. Summary

| 优先级 | 编号 | 标题 | 文件 |
|--------|------|------|------|
| P1 | P1-A | 滚动视口未按 termHeight 裁剪，内容渲染超出屏幕 | `view.go` |
| P1 | P1-B | `/compact` 通过 magic string 硬编码拦截，设计脆弱 | `update.go`, `builtins.go` |
| P1 | P1-C | `glamour.NewTermRenderer` 每帧 O(n) 重建，长对话卡顿 | `messagelist.go` |
| P1 | P1-D | `doAbort` 不清零 `abortFn`/`streamCh`，状态残留风险 | `keys.go` |
| P1 | P1-E | `MemdirLoadedMsg` 数据被完全丢弃，CLAUDE.md 未注入引擎 | `update.go`, `init.go` |
| P2 | P2-A | `newSystemMessage` 使用 `RoleAssistant` 混淆角色，可能污染 LLM 上下文 | `model.go` |
| P2 | P2-B | Tab 补全不循环，`slashSuggestions` 结构体定义但未连接 | `keys.go`, `model.go` |
| P2 | P2-C | `stripANSI` 只处理 `m` 终止符，宽度计算可能偏差 | `coordinator.go` |
| P2 | P2-D | `slashSuggestions`/`commandDisplayEntry` 定义但未使用 | `model.go` |
| P2 | P2-E | Vim Normal 模式缺少 `G/gg/w/b` 等常用导航 | `input.go` |
| P2 | P2-F | 非 slash 输入时 Tab 插入制表符，聊天场景不合适 | `keys.go` |
| P2 | P2-G | `statusBar.tokenUsage`/`cost` 从不更新，始终显示 0 | `statusbar.go`, `update.go` |
| P2 | P2-H | `abortQueryCmd` 返回 nil，建议返回专用 `AbortedMsg` | `cmds.go` |
| P2 | P2-I | Coordinator `Tasks` map 无 eviction，任务永不清除 | `coordinator.go`, `update.go` |
| P2 | P2-J | `renderConfirmDialog` 的 `width` 参数被忽略 | `view.go` |

**测试覆盖率**：`internal/tui/` 目录下当前**零测试文件**。需要为以下关键路径补充单元测试：
- `applyCommandResult`（特别是 `/model`、`/theme`、`/clear`、`/vim` 的状态变更）
- `dispatchEngineMsg`（全部 engine.MsgType 到 tea.Msg 的映射）
- `handlePermissionKey`（Esc 拒绝、Enter 确认、RespFn 回调调用）
- `handleCompactKey` + `handleExitKey`（y/n/Esc 分支）
- `vimNormalKey` 的双键序列（`dd`、`yy`）
- `handleSubmit` 的正常查询路径与 slash 命令分支

建议在合并前至少补充 `applyCommandResult`、`dispatchEngineMsg` 和权限对话框三组测试，其余可在后续迭代中补齐。
