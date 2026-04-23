# 终端恢复系统差异分析报告

> **文档状态**：功能规划  
> **创建日期**：2026-04-23  
> **跟踪 Issue**：#TBD（待创建）  
> **相关文档**：[`terminal-resume-system-design.md`](./origin/terminal-resume-system-design.md)

---

## 概述

本文档基于对 Claude Code TypeScript 原版终端恢复系统的深入分析（见 `origin/terminal-resume-system-design.md`），
与当前 Go 版本实现进行逐项对比，识别差距并规划改进路径。

原版终端恢复系统包含 **6 大核心子系统**（会话存储、中断处理、优雅关闭、会话恢复、文件历史、工具结果存储）和 **完整的跨平台基础设施**，涉及约 **25 个核心文件、~1.8 万行代码**。  
当前 Go 版本仅实现了基础 JSONL 存储层、CLI 标志 `--resume`/`--continue` 和简单 SIGINT 捕获，**约 15% 的功能覆盖率**。高可靠性基础设施（优雅关闭管道、中断检测、文件历史、工具结果卸载）几乎全部缺失。

---

## 目录

- [一、已实现功能](#一已实现功能)
- [二、未实现/不完整功能](#二未实现不完整功能)
- [三、架构差异总结](#三架构差异总结)
- [四、优先改进计划](#四优先改进计划)
- [五、实现建议](#五实现建议)
- [六、代码质量问题](#六代码质量问题)

---

## 一、已实现功能

### 1.1 JSONL 会话存储基础层 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| JSONL 追加写入 (`AppendEntry`) | ✅ 完成 | `internal/session/store.go` |
| JSONL 全量读取 (`ReadAll`) | ✅ 完成 | `internal/session/store.go` |
| 损坏行跳过（WARN 日志） | ✅ 完成 | `internal/session/store.go:98` |
| 会话文件路径 (`sessionPath`) | ✅ 完成 | `internal/session/store.go:186` |
| 项目哈希路径隔离 | ✅ 完成 | `pkg/utils/fs/ProjectHash` |
| `SessionStorer` 接口 | ✅ 完成 | `internal/session/store.go:27` |
| `SessionManager` 生命周期管理 | ✅ 完成 | `internal/session/store.go:122` |

**路径格式与 TS 原版一致**：`<projectDir>/.claude/projects/<hash>/<sessionId>.jsonl`

```go
// internal/session/store.go
func (s *SessionStore) AppendEntry(entry any) error {
    line, err := json.Marshal(entry)
    // ...
    _, err = s.file.Write(append(line, '\n'))
    return err
}
```

### 1.2 CLI 会话标志 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| `--resume <id>` 按 ID 恢复 | ✅ 完成 | `internal/bootstrap/root.go:90` |
| `--continue` / `-c` 恢复最近会话 | ✅ 完成 | `internal/bootstrap/root.go:89` |
| `--fork-session` 分支会话 | ✅ 完成 | `internal/bootstrap/root.go:135` |
| `--session-id` 指定会话 UUID | ✅ 完成 | `internal/bootstrap/root.go:129` |
| `--no-session-persistence` 禁用持久化 | ✅ 完成 | `internal/bootstrap/root.go:133` |

```go
// internal/bootstrap/root.go
cmd.Flags().BoolVarP(&f.continueSession, "continue", "c", false,
    "Resume the most recent session in the current directory")
cmd.Flags().StringVarP(&f.resume, "resume", "r", "",
    "Resume a specific session ID (omit value to show picker)")
```

### 1.3 会话恢复基础逻辑 ⚠️（部分完成）

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 按 ID 加载会话消息 | ✅ 完成 | `internal/bootstrap/session.go:31` |
| 最近会话发现（mtime 排序） | ✅ 完成 | `internal/bootstrap/session.go:48` |
| 消息注入到 `QueryEngine` | ✅ 完成 | `internal/bootstrap/root.go:208` |
| `/resume` 命令骨架 | ⚠️ 存根 | `internal/commands/builtins.go:367` |
| 交互式会话选择器 | ❌ 缺失 | — |

```go
// internal/bootstrap/session.go
func continueMostRecentSession(cwd string) ([]types.Message, error) {
    // 按 mtime 降序排列，取第一个
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].modTime > candidates[j].modTime
    })
    return resumeSessionByID(candidates[0].sessionID, cwd)
}
```

### 1.4 JSONL 类型系统 ⚠️（部分完成）

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| 基础 Entry 类型常量 | ✅ 完成 | `pkg/types/logs.go` |
| `EntryEnvelope` 信封解析 | ✅ 完成 | `pkg/types/logs.go:44` |
| `SerializedMessage` | ✅ 完成 | `pkg/types/logs.go:52` |
| `TranscriptMessage`（含 `parentUuid`） | ✅ 完成 | `pkg/types/logs.go:64` |
| `SummaryEntry` | ✅ 完成 | `pkg/types/logs.go:84` |
| `LogOption`（会话列表项） | ✅ 完成 | `pkg/types/logs.go:72` |
| 14+ 扩展 Entry 类型（文件历史、内容替换等） | ❌ 缺失 | — |

### 1.5 基础中断处理 ⚠️（部分完成）

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| SIGINT 捕获 | ✅ 完成 | `internal/bootstrap/root.go:214` |
| Ctrl+C TUI 中断（加载中） | ✅ 完成 | `internal/tui/keys.go:11` |
| Escape TUI 中断（加载中） | ✅ 完成 | `internal/tui/keys.go:104` |
| `QueryEngine.Interrupt()` 接口 | ✅ 完成 | `internal/engine/engine.go:24` |
| SIGTERM 捕获 | ⚠️ 仅捕获，无处理 | `internal/bootstrap/root.go:214` |
| 中断状态检测与恢复注入 | ❌ 缺失 | — |
| Ctrl+X Ctrl+K（终止所有 Agent） | ❌ 缺失 | — |

### 1.6 Hooks 系统基础层 ✅

| 功能 | 状态 | 文件位置 |
|------|------|----------|
| `PreToolUse` / `PostToolUse` hooks | ✅ 完成 | `internal/hooks/hooks.go` |
| Shell 命令 hook 执行 | ✅ 完成 | `internal/hooks/hooks.go:74` |
| 超时控制 | ✅ 完成 | `defaultTimeoutMs = 10_000` |
| `Stop` hook 类型定义 | ⚠️ 类型已定义，无调用 | `pkg/types` |

---

## 二、未实现/不完整功能

### 2.1 ❌ 优雅关闭管道（Graceful Shutdown Pipeline）

**优先级**：🔴 P0  **难度**：高

**TS 原版**（`gracefulShutdown.ts`，~530 行）：
```typescript
// 6 步关闭优先级序列（有序超时）
async function gracefulShutdown(signal: string) {
    // 1. 同步恢复终端原始模式
    process.stdout.write('\x1b[?1049l'); // 退出 alt-screen
    // 2. 同步打印恢复提示
    printResumeHint(sessionInfo);
    // 3. 等待数据落盘（2s 超时）
    await sessionStorage.flush({ timeout: 2000 });
    // 4. 执行 SessionEnd hooks（可配置超时）
    await runHooks('Stop', { timeout: hooksTimeout });
    // 5. 发送 analytics（500ms 超时）
    await sendAnalytics({ timeout: 500 });
    // 6. 兜底强制退出（200ms）
    setTimeout(() => process.exit(signal), 200);
}
```

**Go 当前实现**（`internal/bootstrap/root.go:212`）：
```go
// 仅捕获信号，goroutine 里什么都没做
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
go func() {
    <-sigCh
    // Let BubbleTea handle its own cleanup; ...
}()
```

**差距分析**：
- 无有序关闭步骤：数据可能未落盘就退出
- 无关闭超时管道：SessionEnd hooks 无限等待
- 无 `cleanupRegistry` 发布订阅：关闭逻辑分散
- 无恢复提示打印
- 无 BubbleTea 程序级强制退出协调

---

### 2.2 ❌ 中断检测与会话恢复注入（Interrupt Detection）

**优先级**：🔴 P0  **难度**：中

**TS 原版**（`conversationRecovery.ts`，~598 行）：
```typescript
// 三状态中断检测
function deserializeMessagesWithInterruptDetection(entries) {
    // none: 正常完成
    // interrupted_prompt: 用户发出但 AI 未开始
    // interrupted_turn: AI 回复中途被截断（最后一条是 tool_result）
    if (lastEntry.type === 'tool_result' && !hasAssistantAfter) {
        return { messages, interruptState: 'interrupted_turn' };
    }
}

// 中断时自动注入"继续"消息
if (interruptState === 'interrupted_turn') {
    messages.push({
        role: 'user',
        content: 'Continue from where you left off.'
    });
}
```

**Go 当前实现**（`internal/bootstrap/session.go:98`）：
```go
// 仅过滤 user/assistant 条目，无任何中断状态检测
func extractMessages(mgr *session.SessionManager) ([]types.Message, error) {
    for _, env := range entries {
        if env.Type != types.EntryTypeUser && env.Type != types.EntryTypeAssistant {
            continue
        }
        // 直接解析，无中断检测
        msgs = append(msgs, sm.Message)
    }
    return msgs, nil
}
```

**差距分析**：
- 无三状态中断检测（none/interrupted_prompt/interrupted_turn）
- 恢复中断会话时不会注入"Continue from where you left off."
- 无 `IsSidechain` 支持（分支链路过滤）
- 无 `parentUuid` DAG 遍历（`buildConversationChain`）

---

### 2.3 ❌ parentUuid DAG 会话链（Conversation Chain）

**优先级**：🔴 P0  **难度**：高

**TS 原版**：
```typescript
// 支持 4 种拓扑结构
type ConversationTopology = 
    | 'linear'        // 标准线性链
    | 'branched'      // 分支链（worktree）
    | 'compacted'     // 压缩后的链
    | 'snipped'       // 修剪后的链

function buildConversationChain(entries: Entry[]): Entry[] {
    // 从叶节点向根节点遍历 parentUuid
    const leafEntry = findLeaf(entries);
    return traverseToRoot(leafEntry, entryMap);
}
```

**Go 当前实现**：
```go
// pkg/types/logs.go - 字段已定义但无 DAG 逻辑
type TranscriptMessage struct {
    ParentUUID  string `json:"parentUuid,omitempty"` // 仅有字段
    IsSidechain bool   `json:"isSidechain,omitempty"`
}

// bootstrap/session.go - 按顺序线性读取，无 DAG 遍历
for _, env := range entries {
    // 线性追加，忽略 parentUuid
}
```

**差距分析**：
- `parentUuid` 字段已定义但无 DAG 遍历实现
- 无叶节点检测（leaf node detection）
- 无 sidechain 过滤（`IsSidechain` 字段已存在但无使用）
- 压缩/修剪后的会话恢复会失序

---

### 2.4 ❌ 交互式 /resume 命令（Session Picker）

**优先级**：🔴 P0  **难度**：中

**TS 原版**（`sessionRestore.ts`，~552 行）：
```typescript
// 交互式 fuzzy 选择器
async function interactiveResume(options: LogOption[]) {
    const selected = await renderSessionPicker(options, {
        searchable: true,
        showPreview: true,
        formatEntry: (opt) => `${opt.title} [${opt.date}] ${opt.firstPrompt}`,
    });
    return selected.sessionId;
}

// Lite 读取（仅 head+tail 64KB，快速加载会话列表）
function readSessionLite(path: string): LogOption {
    const headBytes = readHead(path, 64 * 1024);
    const tailBytes = readTail(path, 64 * 1024);
    return extractMetadata([...parseHead(headBytes), ...parseTail(tailBytes)]);
}
```

**Go 当前实现**（`internal/commands/builtins.go:367`）：
```go
func cmdResume() *Command {
    return &Command{
        Name: "resume",
        Execute: func(ctx CommandContext, args string) Result {
            return Result{
                Text:    "Session resume not yet implemented.", // 存根
                Display: DisplayMessage,
            }
        },
    }
}
```

**差距分析**：
- `/resume` 命令仅为存根，无任何实现
- 无 Lite 读取优化（当前全量读取，大文件慢）
- 无交互式 fuzzy 选择器
- 无会话预览（title/date/firstPrompt）
- 无会话元数据尾部重写（`reAppendSessionMetadata`）

---

### 2.5 ❌ 文件历史备份系统（File History）

**优先级**：🟡 P1  **难度**：高

**TS 原版**（`fileHistory.ts`，~900 行）：
```typescript
// 编辑前备份
async function backupFileForEdit(sessionId, path, content) {
    const hash = sha256(path);
    const version = getNextVersion(sessionId, hash);
    const backupPath = `~/.claude/file-history/${sessionId}/${hash}@v${version}`;
    await writeFile(backupPath, content);
    
    // 记录 file-history-snapshot 条目到 JSONL
    await session.appendEntry({
        type: 'file-history-snapshot',
        files: [{ path, hash, version }],
        turn: currentTurn,
    });
}

// /undo 命令支持
async function undoLastEdit(sessionId) {
    const snapshots = getSnapshotsForLastTurn(sessionId);
    for (const snapshot of snapshots) {
        const backup = readBackup(snapshot.hash, snapshot.version);
        await writeFile(snapshot.path, backup);
    }
}
```

**Go 当前实现**：
```go
// 无文件历史系统
// internal/tools/fileops/ 中无 backup/history 相关代码
// EntryTypeToolResult 已定义，但无 file-history-snapshot 类型
```

**差距分析**：
- 无编辑前文件备份
- 无 `~/.claude/file-history/` 目录结构
- 无版本号管理（`@v<n>` 命名）
- 无 `/undo` 命令实现
- `file-history-snapshot` Entry 类型未定义
- 每个 turn 最多 100 个快照的配额管理未实现

---

### 2.6 ❌ 工具结果存储卸载（Tool Result Storage）

**优先级**：🟡 P1  **难度**：中

**TS 原版**（`toolResultStorage.ts`，~1000 行）：
```typescript
const TOOL_RESULT_THRESHOLD = 50_000; // 50,000 字符

// 大型工具输出卸载到文件
async function storeToolResult(sessionId, toolUseId, content) {
    if (content.length > TOOL_RESULT_THRESHOLD) {
        const ext = detectFormat(content); // 'txt' | 'json'
        const path = `<projectDir>/${sessionId}/tool-results/${toolUseId}.${ext}`;
        await writeFile(path, content);
        // 在 JSONL 中存储占位符
        return '<persisted-output>';
    }
    return content; // 小输出直接内联
}

// 恢复时从磁盘加载
async function restoreToolResults(messages, sessionId) {
    const contentReplacements = new Map<string, string>();
    for (const [id, placeholder] of messages) {
        if (placeholder === '<persisted-output>') {
            contentReplacements.set(id, await readFile(toolResultPath(sessionId, id)));
        }
    }
    return applyReplacements(messages, contentReplacements);
}
```

**Go 当前实现**：
```go
// 无工具结果卸载，所有内容直接内联在 JSONL 中
// internal/session/store.go AppendEntry 不做大小检测
// 无 <persisted-output> 占位符机制
// 无 contentReplacements 恢复逻辑
```

**差距分析**：
- 无 50,000 字符阈值检测
- 大型工具结果直接写入 JSONL（文件体积膨胀）
- 无 `<projectDir>/<sessionId>/tool-results/` 目录结构
- 无 `contentReplacements` Map 恢复机制
- 无并行工具结果恢复（TS 用 `Promise.all`）

---

### 2.7 ❌ Cleanup Registry（清理注册表）

**优先级**：🟡 P1  **难度**：低

**TS 原版**（`cleanupRegistry.ts`，26 行 + `cleanup.ts`，~603 行）：
```typescript
// 发布-订阅式清理注册
const cleanupFns: Array<() => void> = [];

export function registerCleanup(fn: () => void) {
    cleanupFns.push(fn);
}

export async function runCleanupFunctions() {
    await Promise.all(cleanupFns.map(fn => fn()));
}

// 各子系统注册各自的清理逻辑
registerCleanup(() => sessionStorage.close());
registerCleanup(() => mcpClients.closeAll());
registerCleanup(() => fileHistory.flush());
```

**Go 当前实现**：
```go
// 无 cleanup registry
// 各子系统无统一清理入口
// session.Close() 需手动调用（见 bootstrap/session.go:37 的 defer mgr.Close()）
```

**差距分析**：
- 无集中化清理注册机制
- 各子系统（MCP、session、file history）清理逻辑分散
- `SessionManager.Close()` 仅在 `bootstrap/session.go` 的 `defer` 中调用，不在关闭管道中

---

### 2.8 ❌ 孤儿进程检测（Orphan Process Detection）

**优先级**：🟡 P1  **难度**：中

**TS 原版**（macOS 专用）：
```typescript
// macOS 不发送 SIGHUP，而是撤销 TTY FD
// 每 30s 检测 stdout.writable
function startOrphanDetection() {
    setInterval(() => {
        if (!process.stdout.writable) {
            console.error('Parent process died. Exiting...');
            gracefulShutdown(129); // SIGHUP 等效
        }
    }, 30_000);
}
```

**Go 当前实现**：
```go
// 无孤儿进程检测
// internal/bootstrap/root.go 中未实现此逻辑
```

**差距分析**：
- macOS 上父进程死亡后子进程不会收到 SIGHUP
- 无 30s TTY FD 可写性检测
- 孤儿进程会继续运行并消耗 API 配额
- 需要跨平台抽象（macOS 专有逻辑需平台隔离）

---

### 2.9 ❌ 会话元数据尾部重写（Metadata Tail Rewrite）

**优先级**：🟡 P1  **难度**：中

**TS 原版**（`sessionStorage.ts`）：
```typescript
// 退出时将元数据追加到文件末尾
// 确保 Lite 读取（tail 64KB）总能获取到元数据
async function reAppendSessionMetadata(sessionId: string) {
    const metadata = {
        type: 'custom-title',
        title: getSessionTitle(),
        timestamp: Date.now(),
    };
    await appendEntry(metadata); // 写到文件尾部
}

// Lite 读取（只读 head+tail 64KB，不全量读）
function readSessionLite(path: string): LogOption {
    const tail = readTail(path, 64 * 1024); // 尾部 64KB
    const title = extractTitle(tail);       // 从尾部获取最新 title
}
```

**Go 当前实现**：
```go
// 无元数据尾部重写
// 无 Lite 读取优化
// continueMostRecentSession 通过 mtime 排序，不读取会话内容摘要
// 无会话 title/summary 写入机制
```

**差距分析**：
- 会话列表无法显示 title、summary 等元数据
- 全量读取所有历史会话文件（大项目性能差）
- 无 Lite/Full 两级读取策略
- 会话选择器无法预览第一条消息或标题

---

### 2.10 ❌ Ctrl+X Ctrl+K 终止所有 Agent

**优先级**：🟡 P1  **难度**：中

**TS 原版**：
```typescript
// 双键序列 + 3s 窗口确认
let pendingKillAgents = false;
onKeypress('ctrl+x', () => {
    pendingKillAgents = true;
    showHint('Press Ctrl+K within 3s to kill all background agents');
    setTimeout(() => { pendingKillAgents = false; }, 3000);
});
onKeypress('ctrl+k', () => {
    if (pendingKillAgents) {
        chat.emit('chat:killAgents');
        pendingKillAgents = false;
    }
});
```

**Go 当前实现**（`internal/tui/keys.go`）：
```go
// 无 Ctrl+X 键处理
// 无双键序列逻辑
// 无 killAgents 消息
// AgentCoordinator.StopAgent() 接口已存在但无 TUI 触发路径
```

**差距分析**：
- 无法批量终止所有后台 Agent
- 无双键序列状态机（pending + 3s 窗口）
- `AgentCoordinator` 有 `StopAgent` 接口但 TUI 无绑定
- 危险操作无二次确认机制

---

### 2.11 ❌ 会话后台化（Session Backgrounding）

**优先级**：🟢 P2  **难度**：高

**TS 原版**：
```typescript
// Ctrl+B 将前台查询转为后台任务
onKeypress('ctrl+b', async () => {
    const taskId = await backgroundCurrentQuery();
    printMessage(`Query running in background. Task ID: ${taskId}`);
    printMessage(`Resume with: claude --resume ${sessionId}`);
    process.exit(0); // 前台退出，后台继续
});
```

**Go 当前实现**：
```go
// 无 Ctrl+B 后台化
// 无前台转后台机制
// AgentCoordinator 支持后台 Agent，但前台查询无法转移
```

**差距分析**：
- 无 Ctrl+B 键绑定
- 无前台查询转后台的机制
- 无恢复提示打印（`claude --resume <id>`）

---

### 2.12 ❌ 子 Agent 会话恢复（Sub-Agent Resume）

**优先级**：🟢 P2  **难度**：高

**TS 原版**（`resumeAgent.ts`）：
```typescript
// 子 Agent 独立会话文件
// <projectDir>/subagents/agent-<id>.jsonl + .meta.json
async function resumeSubAgent(agentId: string) {
    const session = loadFromFile(`subagents/agent-${agentId}.jsonl`);
    const meta = loadMeta(`subagents/agent-${agentId}.meta.json`);
    
    // 重建 contentReplacements
    await restoreToolResults(session.messages, agentId);
    // 恢复 worktree 状态
    await restoreWorktree(meta.worktree);
    
    return { session, meta };
}
```

**Go 当前实现**：
```go
// 子 Agent 无独立会话文件
// internal/coordinator/coordinator.go 管理 Agent 状态但不持久化
// 无 subagents/ 目录结构
// 无 .meta.json 元数据文件
```

**差距分析**：
- 子 Agent 状态仅在内存中，进程重启后丢失
- 无 `subagents/` 目录结构
- 无 `.meta.json` 元数据序列化
- 无工具结果 `contentReplacements` 重建
- 无 worktree 状态恢复

---

### 2.13 ❌ 跨项目会话恢复（Cross-Project Resume）

**优先级**：🟢 P2  **难度**：中

**TS 原版**：
```typescript
// 检测 worktree vs 不同 repo
function buildCrossProjectResumeCmd(sessionInfo) {
    if (isSameRepo(sessionInfo.projectDir)) {
        return `cd ${sessionInfo.projectDir} && claude --resume ${sessionInfo.id}`;
    } else {
        return `claude --resume ${sessionInfo.id} --project ${sessionInfo.projectDir}`;
    }
}
```

**Go 当前实现**：
```go
// 仅支持当前项目内的会话恢复
// resumeSessionByID 要求 cwd 与原始会话项目匹配
// 无跨目录会话检测
```

**差距分析**：
- 无跨项目会话发现
- 无恢复命令生成（`cd ... && claude --resume <id>`）
- `sessionPath` 强绑定 `projectDir`，无全局会话注册表

---

### 2.14 ❌ 100ms 写入刷新计时器（Flush Timer）

**优先级**：🔴 P0  **难度**：低

**TS 原版**（`sessionStorage.ts`）：
```typescript
// 批量写入，100ms 刷新间隔
class SessionStorage {
    private flushTimer: NodeJS.Timeout | null = null;
    private pendingWrites: Entry[] = [];
    
    appendEntry(entry: Entry) {
        this.pendingWrites.push(entry);
        if (!this.flushTimer) {
            this.flushTimer = setTimeout(() => this.flush(), 100);
        }
    }
    
    async flush(opts?: { timeout?: number }) {
        // 将 pendingWrites 批量写入文件
        await writeBatch(this.pendingWrites);
        this.pendingWrites = [];
        this.flushTimer = null;
    }
}
```

**Go 当前实现**（`internal/session/store.go:57`）：
```go
// 每次追加都直接同步写入，无批量缓冲
func (s *SessionStore) AppendEntry(entry any) error {
    line, err := json.Marshal(entry)
    // ...
    _, err = s.file.Write(append(line, '\n'))  // 同步写入
    return err
}
```

**差距分析**：
- 同步写入无性能问题（Go 的 `os.File.Write` 系统调用开销可接受）
- 但无法支持关闭时 `flush(timeout: 2000)` 的批量刷新语义
- 正常运行时实际无差异，但在优雅关闭管道中需要一致的 flush 接口
- **注**：此差距在不实现 P0.1 优雅关闭管道的情况下影响有限

---

### 2.15 ❌ Tombstone 快速路径（消息删除优化）

**优先级**：🟢 P2  **难度**：中

**TS 原版**：
```typescript
// O(1) 尾部搜索 + ftruncate 消息删除
async function deleteMessage(sessionId, messageId) {
    const fileSize = await stat(path).size;
    // 从末尾 64KB 开始搜索（O(1) 近似）
    const tailBytes = await readTail(path, 64 * 1024);
    const offset = findMessageOffset(tailBytes, messageId);
    if (offset !== -1) {
        // ftruncate 删除（O(1)）
        await truncate(path, fileSize - tailBytes.length + offset);
    } else {
        // 回退到全量重写
        await rewriteWithout(path, messageId);
    }
}
```

**Go 当前实现**：
```go
// 无消息删除功能
// SessionStore 仅支持追加（append-only）
// 无 ftruncate 操作
```

**差距分析**：
- 无消息删除 API
- 无 tombstone 追加策略
- 无 `ftruncate` 快速路径
- 对于需要撤回消息的场景（如 `/undo`），需全量重写

---

### 2.16 ❌ Teleport/Remote 会话恢复

**优先级**：🟢 P2  **难度**：高

**TS 原版**：
```typescript
// 从远端 Session Ingress API 拉取会话
async function hydrateSessionFromRemote(sessionId: string) {
    const response = await fetch(SESSION_INGRESS_API + sessionId);
    const { entries } = await response.json();
    return reconstructSession(entries);
}
```

**Go 当前实现**：
```go
// 无远端会话 API
// 无 Teleport 基础设施
```

**差距分析**：
- 无 Remote Session Ingress API 客户端
- 无会话序列化/反序列化传输格式
- 无跨机器会话同步

---

### 2.17 ❌ Bun 信号兼容性修复

**优先级**：N/A（Go 不适用）

**TS 原版**：
```typescript
// 修复 Bun 的 removeListener bug：
// 注册一个永不取消的 onExit 防止信号处理器被静默删除
import { onExit } from 'signal-exit';
onExit(() => {}); // 永不取消
```

**Go 当前实现**：
```go
// Go 标准库 os/signal 无此 bug
// signal.Notify 的语义正确，无需修复
```

**差距分析**：此条目不适用于 Go 实现，可忽略。

---

## 三、架构差异总结

| 维度 | TS 原版 | Go 当前实现 | 完成度 |
|------|---------|------------|--------|
| **JSONL 写入** | 100ms 批量刷新 + crash-safe 追加 | 同步 `os.File.Write` 追加 | 70% |
| **JSONL 读取** | Lite（head+tail 64KB）+ Full 两级 | 全量 `bufio.Scanner` 读取 | 40% |
| **Entry 类型数量** | 14+ 类型（含 file-history、content-replacement 等） | 22 类型（结构不完整） | 50% |
| **parentUuid DAG** | 叶→根遍历，4 种拓扑支持 | 字段已定义，无遍历逻辑 | 10% |
| **中断检测** | 三状态（none/interrupted_prompt/interrupted_turn） | 无检测 | 0% |
| **优雅关闭** | 6 步有序管道，含超时 | 仅捕获信号，空处理 | 5% |
| **Cleanup Registry** | 发布-订阅，26 行核心 | 无集中机制 | 0% |
| **会话元数据** | 退出时尾部重写，Lite 读取可见 | 无元数据管理 | 0% |
| **CLI --resume/--continue** | fuzzy picker + 预览 + cross-project | 基础文件查找 + 注入 | 40% |
| **/resume 命令** | 交互式选择器，完整实现 | 存根（"not yet implemented"）| 5% |
| **文件历史备份** | ~/.claude/file-history/ + /undo | 无 | 0% |
| **工具结果卸载** | 50K 阈值 + 磁盘外存 + 恢复 | 无，全内联 | 0% |
| **孤儿进程检测** | macOS 30s TTY 检测 | 无 | 0% |
| **子 Agent 恢复** | 独立 JSONL + .meta.json | 无持久化 | 0% |
| **后台化（Ctrl+B）** | 前台→后台转移 | 无 | 0% |
| **Ctrl+X Ctrl+K** | 双键序列终止所有 Agent | 无 | 0% |
| **跨项目恢复** | worktree/repo 检测 + 命令生成 | 无 | 0% |
| **Teleport 恢复** | 远端 API 水合 | 无 | 0% |
| **Tombstone 删除** | O(1) ftruncate 快速路径 | 无删除 API | 0% |
| **整体覆盖率** | — | — | **≈ 15%** |

---

## 四、优先改进计划

### P0 — 核心可靠性（建议 5 人天）

| 编号 | 任务 | 文件 | 工作量 |
|------|------|------|--------|
| P0-1 | 实现优雅关闭管道（6 步有序序列） | `internal/bootstrap/shutdown.go`（新建） | 2d |
| P0-2 | 实现中断状态检测与恢复注入 | `internal/bootstrap/session.go` | 1d |
| P0-3 | 实现 parentUuid DAG 遍历 + 叶节点检测 | `internal/session/chain.go`（新建） | 1.5d |
| P0-4 | 实现 `SessionStore.Flush(timeout)` 接口 | `internal/session/store.go` | 0.5d |

### P1 — 重要用户体验（建议 7 人天）

| 编号 | 任务 | 文件 | 工作量 |
|------|------|------|--------|
| P1-1 | 实现 Cleanup Registry（发布-订阅清理） | `internal/lifecycle/cleanup.go`（新建） | 0.5d |
| P1-2 | 实现文件历史备份系统（含 /undo） | `internal/filehistory/`（新建包） | 2.5d |
| P1-3 | 实现工具结果卸载（50K 阈值） | `internal/session/toolresult.go`（新建） | 1.5d |
| P1-4 | 实现会话元数据尾部重写 + Lite 读取 | `internal/session/store.go` | 1d |
| P1-5 | 实现孤儿进程检测（macOS 专用） | `internal/bootstrap/orphan_darwin.go`（新建） | 0.5d |
| P1-6 | 实现 Ctrl+X Ctrl+K 双键序列 + TUI 绑定 | `internal/tui/keys.go` | 1d |

### P2 — 完整性提升（建议 8 人天）

| 编号 | 任务 | 文件 | 工作量 |
|------|------|------|--------|
| P2-1 | 实现 /resume 交互式选择器（TUI 内嵌）| `internal/tui/session_picker.go`（新建） | 2d |
| P2-2 | 实现子 Agent 会话持久化（JSONL + .meta.json）| `internal/coordinator/persistence.go`（新建） | 2d |
| P2-3 | 实现会话后台化（Ctrl+B） | `internal/tui/keys.go` + `engine/` | 1.5d |
| P2-4 | 实现跨项目会话发现与命令生成 | `internal/session/crossproject.go`（新建） | 1.5d |
| P2-5 | 实现 Tombstone 快速路径消息删除 | `internal/session/store.go` | 1d |

### P3 — 可选增强（建议 4 人天）

| 编号 | 任务 | 文件 | 工作量 |
|------|------|------|--------|
| P3-1 | 实现 Teleport/Remote 会话恢复 | `internal/session/remote.go`（新建） | 2d |
| P3-2 | 会话文件 100MB 分块策略 | `internal/session/store.go` | 1d |
| P3-3 | `EntryType` 完整枚举（file-history-snapshot 等） | `pkg/types/logs.go` | 0.5d |
| P3-4 | 会话恢复 Analytics 集成 | `internal/bootstrap/shutdown.go` | 0.5d |

---

## 五、实现建议

### 5.1 优雅关闭管道（P0-1）

```go
// internal/bootstrap/shutdown.go
package bootstrap

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/tunsuy/claude-code-go/internal/lifecycle"
    "github.com/tunsuy/claude-code-go/internal/session"
)

// GracefulShutdown executes the 6-step ordered shutdown pipeline.
// It mirrors TS gracefulShutdown.ts step-by-step.
func GracefulShutdown(container *AppContainer, exitCode int) {
    // Step 1: Restore terminal raw mode (synchronous)
    fmt.Fprint(os.Stdout, "\x1b[?1049l") // exit alt-screen

    // Step 2: Print resume hint (synchronous)
    if container.SessionManager != nil {
        sid := container.SessionManager.SessionId
        fmt.Fprintf(os.Stderr, "\n\nTo resume: claude --resume %s\n", sid)
    }

    // Step 3: Flush session data (2s timeout)
    flushCtx, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer flushCancel()
    if err := container.SessionManager.Flush(flushCtx); err != nil {
        fmt.Fprintf(os.Stderr, "warn: session flush: %v\n", err)
    }

    // Step 4: Run Stop hooks (configurable timeout)
    hooksCtx, hooksCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer hooksCancel()
    _ = container.HookDispatcher.Run(hooksCtx, "Stop", nil)

    // Step 5: Run all registered cleanup functions
    lifecycle.RunCleanupFunctions()

    // Step 6: Force exit
    os.Exit(exitCode)
}
```

### 5.2 中断状态检测（P0-2）

```go
// internal/session/recovery.go
package session

import "github.com/tunsuy/claude-code-go/pkg/types"

// InterruptState describes how a prior session ended.
type InterruptState int

const (
    InterruptStateNone             InterruptState = iota // completed normally
    InterruptStateInterruptedPrompt                      // user sent, AI not started
    InterruptStateInterruptedTurn                        // AI reply cut mid-tool-result
)

// DetectInterruptState inspects the tail of messages to determine how the
// session ended. Mirrors TS deserializeMessagesWithInterruptDetection.
func DetectInterruptState(msgs []types.Message) InterruptState {
    if len(msgs) == 0 {
        return InterruptStateNone
    }
    last := msgs[len(msgs)-1]
    if last.Role == "user" {
        // User sent a message but we have no assistant reply.
        return InterruptStateInterruptedPrompt
    }
    // Check if last assistant message ends with tool_result (interrupted mid-turn)
    for i := len(msgs) - 1; i >= 0; i-- {
        if msgs[i].Role == "assistant" {
            break
        }
        if msgs[i].Role == "user" {
            for _, blk := range msgs[i].Content {
                if blk.Type == "tool_result" {
                    return InterruptStateInterruptedTurn
                }
            }
        }
    }
    return InterruptStateNone
}

// InjectContinueMessage appends a "continue" user message if the session was
// interrupted mid-turn.
func InjectContinueMessage(msgs []types.Message, state InterruptState) []types.Message {
    if state != InterruptStateInterruptedTurn {
        return msgs
    }
    return append(msgs, types.Message{
        Role:    "user",
        Content: []types.ContentBlock{{Type: "text", Text: "Continue from where you left off."}},
    })
}
```

### 5.3 Cleanup Registry（P1-1）

```go
// internal/lifecycle/cleanup.go
package lifecycle

import "sync"

var (
    mu         sync.Mutex
    cleanupFns []func()
)

// RegisterCleanup registers fn to be called during graceful shutdown.
// Mirrors TS cleanupRegistry.ts.
func RegisterCleanup(fn func()) {
    mu.Lock()
    defer mu.Unlock()
    cleanupFns = append(cleanupFns, fn)
}

// RunCleanupFunctions calls all registered cleanup functions in reverse
// registration order (LIFO).
func RunCleanupFunctions() {
    mu.Lock()
    fns := make([]func(), len(cleanupFns))
    copy(fns, cleanupFns)
    mu.Unlock()

    // LIFO order: last registered, first cleaned up.
    for i := len(fns) - 1; i >= 0; i-- {
        fns[i]()
    }
}
```

### 5.4 parentUuid DAG 遍历（P0-3）

```go
// internal/session/chain.go
package session

import "github.com/tunsuy/claude-code-go/pkg/types"

// BuildConversationChain traverses the parentUuid DAG from the leaf entry
// to the root, returning entries in root→leaf order.
// Mirrors TS buildConversationChain.
func BuildConversationChain(entries []types.TranscriptMessage) []types.TranscriptMessage {
    // Build a UUID → entry map.
    byUUID := make(map[string]types.TranscriptMessage, len(entries))
    for _, e := range entries {
        uuid := e.SerializedMessage.Message.ID
        byUUID[uuid] = e
    }

    // Find leaf: the entry that no other entry points to as parent.
    parentUUIDs := make(map[string]bool)
    for _, e := range entries {
        if e.ParentUUID != "" {
            parentUUIDs[e.ParentUUID] = true
        }
    }
    var leaf *types.TranscriptMessage
    for i := range entries {
        uuid := entries[i].SerializedMessage.Message.ID
        if !parentUUIDs[uuid] && !entries[i].IsSidechain {
            leaf = &entries[i]
            break
        }
    }
    if leaf == nil {
        return nil
    }

    // Traverse from leaf to root.
    var chain []types.TranscriptMessage
    current := leaf
    visited := make(map[string]bool)
    for current != nil {
        uuid := current.SerializedMessage.Message.ID
        if visited[uuid] {
            break // cycle guard
        }
        visited[uuid] = true
        chain = append(chain, *current)
        if current.ParentUUID == "" {
            break
        }
        parent, ok := byUUID[current.ParentUUID]
        if !ok {
            break
        }
        current = &parent
    }

    // Reverse to root→leaf order.
    for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
        chain[i], chain[j] = chain[j], chain[i]
    }
    return chain
}
```

### 5.5 孤儿进程检测（P1-5，macOS 专用）

```go
// internal/bootstrap/orphan_darwin.go
//go:build darwin

package bootstrap

import (
    "os"
    "time"
)

// StartOrphanDetection starts a background goroutine that checks every 30s
// whether stdout is still writable. On macOS, the kernel revokes the TTY FD
// instead of sending SIGHUP when the parent terminal closes.
func StartOrphanDetection(container *AppContainer) {
    go func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        for range ticker.C {
            // Attempt a zero-byte write to stdout to detect TTY revocation.
            _, err := os.Stdout.Write(nil)
            if err != nil {
                // stdout is no longer writable — parent process died.
                GracefulShutdown(container, 129) // SIGHUP equivalent
                return
            }
        }
    }()
}
```

---

## 六、代码质量问题

### 6.1 /resume 命令存根未标记 TODO

**位置**：`internal/commands/builtins.go:371`

```go
// 当前：静默返回"not yet implemented"，无 TODO 标记
return Result{
    Text:    "Session resume not yet implemented.",
    Display: DisplayMessage,
}

// 建议：添加明确的 TODO 并关联 issue
// TODO(P0-4): Implement interactive session picker. See issue #XXX.
```

### 6.2 信号处理 goroutine 无任何动作

**位置**：`internal/bootstrap/root.go:215`

```go
// 当前：捕获信号后 goroutine 空跑
go func() {
    <-sigCh
    // Let BubbleTea handle its own cleanup; ...
}()

// 问题：BubbleTea 不知道需要处理 SIGTERM；仅靠注释无法保证行为
// 建议：信号到达时调用 GracefulShutdown 或 p.Quit()
go func() {
    sig := <-sigCh
    exitCode := 0
    if sig == syscall.SIGTERM {
        exitCode = 15 // SIGTERM 退出码约定
    }
    GracefulShutdown(container, exitCode)
}()
```

### 6.3 extractMessages 忽略 TranscriptMessage 包装层

**位置**：`internal/bootstrap/session.go:104`

```go
// 当前：直接解析为 SerializedMessage，丢失 parentUuid 等字段
var sm types.SerializedMessage
if err := json.Unmarshal(env.Raw, &sm); err != nil {
    continue
}
msgs = append(msgs, sm.Message)

// 建议：先尝试解析为 TranscriptMessage，再降级为 SerializedMessage
var tm types.TranscriptMessage
if err := json.Unmarshal(env.Raw, &tm); err == nil && tm.ParentUUID != "" {
    // Use TranscriptMessage for DAG-aware chain building
    transcriptMsgs = append(transcriptMsgs, tm)
} else {
    var sm types.SerializedMessage
    if err := json.Unmarshal(env.Raw, &sm); err == nil {
        msgs = append(msgs, sm.Message)
    }
}
```

### 6.4 SessionStore 无 Flush 接口

**位置**：`internal/session/store.go`

```go
// 当前：Close() 关闭文件，无 Flush 语义
func (s *SessionStore) Close() error {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.file.Close()
}

// 建议：添加带超时的 Flush 方法（用于优雅关闭管道 Step 3）
func (s *SessionStore) Flush(ctx context.Context) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    return s.file.Sync() // fdatasync 确保落盘
}
```

### 6.5 SessionStorer 接口过于精简

**位置**：`internal/session/store.go:27`

```go
// 当前接口
type SessionStorer interface {
    AppendEntry(entry any) error
    ReadAll() ([]types.EntryEnvelope, error)
    Close() error
}

// 建议：扩展接口以支持关闭管道需求
type SessionStorer interface {
    AppendEntry(entry any) error
    ReadAll() ([]types.EntryEnvelope, error)
    ReadLite() (*types.LogOption, error)      // Lite 读取，只读 head+tail
    Flush(ctx context.Context) error           // 带超时刷盘
    Close() error
}
```

### 6.6 bootstrap/session.go 与 session 包职责重叠

**位置**：`internal/bootstrap/session.go`

`continueMostRecentSession` 和 `resumeSessionByID` 的发现逻辑应属于 `session` 包，而非 `bootstrap`。当前在 `bootstrap` 层实现会导致：
- `session` 包无法被其他入口（如 headless 模式、测试）复用
- 会话发现与 cobra 标志耦合

**建议**：将 `continueMostRecentSession` 移入 `internal/session/manager.go`，暴露 `session.FindMostRecent(projectDir) (SessionID, error)` 接口。

---

## 依赖关系图

```
terminal-resume-system-gap-analysis
├── P0（核心可靠性）
│   ├── P0-1 优雅关闭管道
│   │   ├── 依赖：P1-1 Cleanup Registry
│   │   └── 依赖：P0-4 SessionStore.Flush
│   ├── P0-2 中断状态检测
│   │   └── 依赖：P0-3 parentUuid DAG
│   ├── P0-3 parentUuid DAG 遍历
│   └── P0-4 SessionStore.Flush
│
├── P1（重要用户体验）
│   ├── P1-1 Cleanup Registry ← P0-1 依赖
│   ├── P1-2 文件历史备份
│   │   └── 依赖：P0-4 SessionStore.Flush
│   ├── P1-3 工具结果卸载
│   │   └── 依赖：P0-4 SessionStore.Flush
│   ├── P1-4 会话元数据尾部重写
│   │   └── 依赖：P0-4 SessionStore.Flush
│   ├── P1-5 孤儿进程检测
│   │   └── 依赖：P0-1 优雅关闭管道
│   └── P1-6 Ctrl+X Ctrl+K
│
├── P2（完整性提升）
│   ├── P2-1 /resume 交互式选择器
│   │   └── 依赖：P1-4 会话元数据
│   ├── P2-2 子 Agent 会话持久化
│   │   └── 依赖：P1-3 工具结果卸载
│   ├── P2-3 会话后台化
│   │   └── 依赖：P0-1 优雅关闭管道
│   ├── P2-4 跨项目会话发现
│   └── P2-5 Tombstone 消息删除
│
└── P3（可选增强）
    ├── P3-1 Teleport 远端恢复
    ├── P3-2 100MB 分块策略
    ├── P3-3 EntryType 完整枚举
    └── P3-4 关闭时 Analytics
```

---

## 相关资源

- [`docs/analysis/origin/terminal-resume-system-design.md`](./origin/terminal-resume-system-design.md) — TS 原版详细设计（本文对比基准）
- [`internal/session/store.go`](../../internal/session/store.go) — Go 会话存储实现
- [`internal/bootstrap/session.go`](../../internal/bootstrap/session.go) — Go 会话加载逻辑
- [`internal/bootstrap/root.go`](../../internal/bootstrap/root.go) — CLI 标志 + 信号处理
- [`internal/tui/keys.go`](../../internal/tui/keys.go) — TUI 键盘事件处理
- [`pkg/types/logs.go`](../../pkg/types/logs.go) — JSONL 类型定义
- [`docs/analysis/multi-agent-system-gap-analysis.md`](./multi-agent-system-gap-analysis.md) — 多 Agent 系统差异分析（相关）
