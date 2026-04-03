# Tech Lead 评审：tui.md

> 评审人：Tech Lead
> 日期：2026-04-02
> 结论：**有条件通过（APPROVED_WITH_CHANGES）**

---

## 总体评估

TUI 设计在架构上非常出色。BubbleTea MVU 从 React/Ink 的映射论证充分，流式传输的 Cmd 拉取循环是正确的 BubbleTea 模式，带有 `stubQueryEngine` 的 `QueryEngine` 密封接口干净地解耦了 TUI 开发。所需修改聚焦于几处不完整区域和权限握手中的一处设计空白。

---

## 优点

1. **BubbleTea Elm 架构应用正确** — `AppModel` 作为单一根模型，`Update()` 返回 `(tea.Model, tea.Cmd)`，`View()` 作为纯字符串渲染函数。这是惯用的 BubbleTea 写法，正确对应了 TS React/Ink 的组件状态（§1、§8.1）。

2. **流式传输的 Cmd 拉取循环** — `waitForStreamEvent(stream)` 模式是正确的 BubbleTea 惯用法：每个处理流式事件的 `Update` 返回下一个 `waitForStreamEvent` Cmd，创建具有反压感知的拉取循环，避免协程与渲染循环竞争（§2、§8.2）。相比 `Program.Send()` 推送模式，这对于流式传输更优。

3. **`QueryEngine` 接口作为唯一核心依赖** — 密封的 `isQueryEvent()` 标记方法模式正确防止外部包实现 `Event` 接口，强制了契约边界（§3）。`stubQueryEngine` 使 TUI 可以在没有核心实现的情况下完整开发和测试。

4. **`PermissionRequestEvent.RespondCh chan<- PermissionDecision`** — channel 握手正确建模了权限流：Core 发送事件并在 `RespondCh` 上阻塞；TUI 更新对话框并发送决策。这是暂停-恢复流程控制的正确设计（§3）。

5. **`Init()` 批处理** — 正确地将 `loadMemdirCmd()`、`tea.EnterAltScreen`、`tickCmd()` 和 `listenWindowSize()` 在单个 `tea.Batch` 中批处理，这是惯用的 BubbleTea 初始化模式（§2）。

6. **斜杠命令注册表与 Tab 补全** — `Registry.CompletePrefix(partial)` 用于内联补全，`SlashCommandSuggestions` 在输入框上方以弹窗形式渲染。所有 19 个斜杠命令均被枚举并映射到 TS 源码（§4）。

7. **`memdir` 常量精确** — `MemoryFileName="CLAUDE.md"`、`MaxEntrypointLines=200`、`MaxEntrypointBytes=25_000` 与 TS 一致（§6）。由内向外的遍历在主目录处停止的规则指定正确。

8. **`LoadAndTruncate` 双重截断顺序** — 先按行数截断，再按最后换行处截断字节数。与 TS 的 `truncateEntrypointContent` 语义一致（§6）。

9. **`glamour` 用于 Markdown 渲染** — 选择正确；`charmbracelet/glamour` 是 BubbleTea 生态中惯用的终端 Markdown 渲染器，无需移植 `remark/rehype`（§8.3）。

10. **`lipgloss` 主题系统** — 语义颜色角色（`Primary`、`Secondary`、`Accent`、`Muted`、`Error`、`Warning`、`Success`），内置命名主题（`dark`、`light`、`tokyo-night`），以及 `/theme` 命令集成。设计正确（§2、theme.go）。

11. **Vim 模式状态机** — `Normal | Insert | Visual` 状态在 `input.Model.Update()` 中，通过 `/vim` 或 `--vim` 标志激活。正确描述为嵌套的 `tea.Model`（§8.4）。

12. **协调器面板设计** — `CoordinatorPanel`、`AgentTaskState`、状态显示表和键盘交互（`↑↓`、`Enter`、`x`、`Esc`）均已指定。从 Core 通过 `tea.Program.Send(AgentStatusMsg{})` 的推送模型是正确的（§5）。

---

## 问题

**【阻塞】`waitForStreamEvent` 在 `tui` 包内使用包限定类型引用 `QueryEngine.Event`**
§2 中的 `waitForStreamEvent(stream <-chan QueryEngine.Event)` 将 `QueryEngine` 用作包名，但 `QueryEngine` 是在同一 `tui` 包中定义的接口。channel 类型应为 `<-chan Event`（不是 `QueryEngine.Event`）。这是编译错误。§3 中的代码正确将 `Event` 定义为类型；§2 的 `cmds.go` 伪代码必须更新以保持一致性。

**【严重】权限握手：`RespondCh` 发送无超时**
当 TUI 通过 `sendPermissionResponseCmd` 向 `RespondCh` 发送 `msg.Decision` 时，若 Core 协程已被取消（例如用户在权限对话框打开时按下 Ctrl+C），向 `RespondCh` 的发送将永久阻塞。channel 发送必须使用带 `ctx.Done()` 回退的 `select`。

**【严重】`DiscoverClaudeMd` 遍历存在逻辑 Bug**
§6 中，循环在 `if dir == parent || dir == home` 块内有 `if dir == home { break }`，但同时存在 `dir = home; continue` 路径。若 `startDir` 已在主目录处或其下，`home` 目录本身会被访问两次（一次来自遍历，一次来自 `dir = home` 路径的回退）。算法应显式去重。更简洁的实现：预先将目录收集到切片（`startDir` → 父目录 → 不超过 home），然后遍历该切片。

**【严重】`AppModel.abortFn context.CancelFunc` 在 `StreamDoneMsg` 后未重置**
处理 `StreamDoneMsg` 的 `Update` 分支设置 `m.isLoading = false`，但未将 `m.abortFn` 置为 `nil`。若用户在完成后按下 Esc，后续的 `Abort()` 调用将在已完成的上下文上调用过期的取消函数（无害但令人困惑）。更关键的是，`startQueryCmd` 必须在创建新上下文前检查 `m.abortFn != nil` 并调用它，否则上一次查询的协程可能逃逸。

**【严重】`tui` 包中定义的 `QueryEngine` 接口可能导致循环导入**
§4 中的 `CommandContext` 含有 `QueryEngine QueryEngine` 字段，意味着 `internal/commands` 导入 `internal/tui`。若某个命令需要触发查询，`internal/tui` 也会导入 `internal/commands`（用于 `Registry`）。这是循环导入。`QueryEngine` 和 `Event` 接口必须在实现开始前提取到共享包（`pkg/types` 或 `internal/core`）。

**【次要】`TickMsg` 和 `AgentStatusMsg` 处理缺少过期条目清理规范**
§1（`AgentStatusMsg` 的 `Update`）简单执行 `m.agentTasks[msg.TaskID] = msg.Status` 而不进行驱逐。§5 提到 `AgentTaskState` 中的 `EvictAfter *time.Time`，但 `handleTick` 检查过期任务的逻辑未说明。请记录驱逐逻辑（在每次 tick 时检查 `EvictAfter`，移除过期条目）。

**【次要】`StreamDoneMsg.FinalMessage` 与进行中流式 token 的区分方式未说明**
`StreamTokenMsg` 的 `Update` 将增量追加到最后一条消息，但 `StreamDoneMsg` 也追加 `msg.FinalMessage`。若两者都到达并被追加，最终消息将被重复。设计应明确：(a) `StreamDoneMsg` 替换进行中的消息，或 (b) `StreamDoneMsg` 仅携带使用数据（不含内容），内容已从 `StreamTokenMsg` 增量中积累。

---

## 必须修改项

1. **修复 `waitForStreamEvent` 类型引用** — 将 §2 的 `cmds.go` 伪代码中的 `<-chan QueryEngine.Event` 改为 `<-chan Event`。验证 `cmds.go` 中所有事件类型引用使用正确的非限定名称。

2. **为 `RespondCh` 发送添加超时** — 在 `sendPermissionResponseCmd` 中使用 `select { case respondCh <- decision: case <-ctx.Done(): }` 防止流已取消时的协程泄漏。

3. **修复 `DiscoverClaudeMd` 遍历** — 重写循环，预先将父目录收集到切片（包含 home），然后遍历，对已访问路径去重。`~/.claude/CLAUDE.md` 全局回退应在去重检查后追加。

4. **在 `StreamDoneMsg` 和 `StreamErrorMsg` 处理中将 `abortFn` 重置为 `nil`** — 在两种情况下添加 `m.abortFn = nil`。在 `startQueryCmd` 中添加守卫：`if m.abortFn != nil { m.abortFn() }`，然后再创建新上下文。

5. **将 `QueryEngine` 接口移到共享包** — 在 `internal/core` 或 `pkg/types` 中定义 `QueryEngine` 和 `Event` 类型，打破 `tui → commands → tui` 循环导入。更新两个包中的所有引用。

6. **说明 `StreamDoneMsg` 语义** — 在 §1（messages.go）中明确 `StreamDoneMsg.FinalMessage` 是追加还是替换正在积累的流式消息。

---

## 实现注意事项

- `AppModel` 在 `Update(msg tea.Msg) (tea.Model, tea.Cmd)` 中是值传递。随着 `messages []types.Message` 增长到数百条，切片头部的复制会带来显著的 GC 压力。考虑将 `messages` 存储为 `*[]types.Message`（指向切片的指针），或作为独立的不可变日志结构。也可以先做基准测试，若 BubbleTea 自身的复制语义使此问题不可避免，则接受现状。
- `AppModel` 中持有指针的 `permReq *PermissionRequest` 字段在值复制的 Model 中是正确的：指针在复制间保持稳定，因此 `respondCh` 保持有效。请在文档中明确说明，避免未来重构时天真地深拷贝模型。
- `memdir.LoadMemoryPrompt` 在每次 `Submit` 时调用。对于含有多个 `CLAUDE.md` 文件的仓库，这是重复的文件系统遍历。考虑使用文件监视失效（`fsnotify`）来缓存结果，而非每次提交都重新遍历。
- `glamour` 渲染器配置应根据当前终端背景色（通过 `$COLORFGBG` 或 `termenv` 检测明/暗）选择合适的 glamour 样式，而非仅依赖用户的 `/theme` 选择。
- 明确说明 `PromptInput` 嵌套模型委托模式：`AppModel.Update()` 应调用 `m.input, inputCmd = m.input.Update(msg)`，并将 `inputCmd` 与 AppModel 自身的 Cmd 一起批处理。请在 §2（input.go）中明确记录，防止重新实现为普通 `View()` 函数（这会丢失 Vim 模式状态）。
- `startQueryCmd` 当前将 `context.Background()` 传递给 `Submit`。这意味着中止完全依赖 `QueryEngine.Abort()` 而非上下文取消。请明确记录此选择，并确保 `QueryEngine.Abort()` 保证关闭返回的 channel；否则拉取循环将无限期挂起。

---

*评审版本：v1.0 · 2026-04-02 · Tech Lead*
