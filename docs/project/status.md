# Claude Code Go — 任务状态总览

> 维护人：PM Agent
> 最后更新：2026-04-03（第 9 次巡检）

---

## 状态图例

| 图标 | 含义 |
|------|------|
| ✅ | 已完成 |
| 🔄 | 进行中 |
| ⏳ | 待启动（有阻塞依赖） |
| 🚫 | 阻塞（等待外部解决） |

---

## 任务状态

| 任务ID | 任务 | 状态 | 负责方 | 备注 |
|--------|------|------|--------|------|
| #2 | 输出项目计划表 | ✅ 已完成 | PM | |
| #3 | 初始化项目骨架 | ✅ 已完成 | Agent-CLI | |
| #4 | 输出基础设施层详细设计文档 | ✅ 已完成 | Agent-Infra | |
| #5 | 输出服务层详细设计文档 | ✅ 已完成 | Agent-Services | |
| #6 | 输出核心层详细设计文档 | ✅ 已完成 | Agent-Core | |
| #7 | 输出工具层详细设计文档 | ✅ 已完成 | Agent-Tools | |
| #8 | 输出 TUI 层详细设计文档 | ✅ 已完成 | Agent-TUI | |
| #9 | 输出 CLI 入口层详细设计文档 | ✅ 已完成 | Agent-CLI | |
| #10 | 评审基础设施层设计文档 | ✅ 已完成 | Tech Lead | 见 review-infra.md |
| #11 | 评审核心层设计文档（含 Tool 接口契约确认） | ✅ 已完成 | Tech Lead | 见 review-core.md；Tool 接口正式确认 |
| #12 | 评审服务/工具/TUI/CLI 四份设计文档 | ✅ 已完成 | Tech Lead | 见 review-services/tools/tui/cli.md |
| #13 | 实现基础设施层所有模块 | ✅ 已完成 | Agent-Infra | P0-2/P0-3/P0-1 均已修复，全部测试通过 |
| #14 | 实现服务层所有模块 | ✅ 已完成 | Agent-Services | AES-256-GCM token store, singleflight token refresh, MCP adapter |
| #15 | 实现核心层所有模块 | ✅ 已完成 | Agent-Core | QueryEngine、权限决策链、对话压缩，全部测试通过 |
| #16 | 实现所有内置工具 | ✅ 已完成 | Agent-Tools | B-1 已修复（GetPath 已从 Tool 主接口移除），所有工具实现完成 |
| #17 | 实现 TUI 层所有模块 | ✅ 已完成 | Agent-TUI | BubbleTea MVU，流式事件循环，vim 模式，权限对话框，全部测试通过 |
| #18 | 实现 CLI 入口层 | ✅ 已完成 | Agent-CLI | cobra v1.8.1，chat/version/config/mcp 命令，bootstrap 连线，go build ./... 通过 |
| #19 | 输出测试策略文档，搭建测试基础设施 | ✅ 已完成 | QA | |
| #20 | 验收基础设施层 | ✅ 已完成 | QA | 有条件通过；0 P0，1 P1，4 P2 |
| #21 | 验收服务层 | ✅ 已完成 | QA | 有条件通过；1 P0（MCP 并发竞争），8 P1，10 P2 |
| #22 | 验收核心层 | ✅ 已完成 | QA | 有条件通过；2 P0（abortFn data race + MicroCompactor 接口不匹配），3 P1，5 P2 |
| #23 | 验收工具层 | ✅ 已完成 | QA | 有条件通过；0 P0，4 P1（并发分类×3+测试缺失），3 P2 |
| #24 | 验收 TUI 层 | ✅ 已完成 | QA | 有条件通过；1 P0（/model 结果未应用），3 P1，5 P2 |
| #25 | 验收 CLI 入口层 | ✅ 已完成 | QA | 有条件通过；4 P0（零测试/os.Setenv/os.Exit/mcp unimplemented），6 P1，6 P2 |
| #31 | Fix P0-B: engine abortFn data race | ✅ 已完成 | Agent-Core | `abortMu sync.Mutex` 保护 abortFn 读写 |
| #33 | Fix P0-C: CLI layer zero tests | ✅ 已完成 | Agent-CLI | |
| #34 | Fix P0-F: implement mcp serve subcommand | ✅ 已完成 | Agent-CLI | |
| #35 | Fix P0-A: MCP jsonRPC concurrent Recv data race | ✅ 已完成 | Agent-Services | |
| #38 | Fix P0-H: MicroCompactor interface mismatch | ✅ 已完成 | Agent-Core | |
| #40 | Fix P0-G: TUI /model command not applied | ✅ 已完成 | Agent-TUI | |
| #41 | Fix P0-D/E: bootstrap os.Setenv and os.Exit | ✅ 已完成 | Agent-CLI | |
| #42 | 【Tech Lead】代码评审：基础设施层（#13） | ✅ 已完成 | Tech Lead | 见 code-review-infra.md；P0×1，P1×4，P2×6 |
| #43 | 【Tech Lead】代码评审：核心层（#15） | ✅ 已完成 | Tech Lead | 见 code-review-core.md；P0×6，P1×6，P2×7 |
| #44 | 【Tech Lead】代码评审：CLI 层（#18） | ✅ 已完成 | Tech Lead | 见 code-review-cli.md；P0×0，P1×6，P2×8 |
| #45 | 【Tech Lead】代码评审：TUI 层（#17） | ✅ 已完成 | Tech Lead | 见 code-review-tui.md；P0×0，P1×5，P2×10 |
| #47 | 【Tech Lead】代码评审：服务层（#14） | ✅ 已完成 | Tech Lead | 见 code-review-services.md；P0×0，P1×4，P2×9 |
| #50 | 【Tech Lead】代码评审：工具层（#16） | ✅ 已完成 | Tech Lead | 见 code-review-tools.md；P0×2，P1×5，P2×9 |
| #26 | 执行集成测试，出具最终验收报告 | 🚫 阻塞 | QA | 阻塞于代码评审 P0 修复 |

---

## 当前阻塞项（代码评审新发现）

> 原 P0-A～P0-H（QA 验收阶段发现）均已修复。以下为 Tech Lead 代码评审新发现的 P0 问题。

| 编号 | 严重级别 | 层 | 描述 | 影响任务 | 负责解决方 |
|------|----------|----|------|----------|------------|
| P0-CR-1 | P0 | 核心层 | `e.messages` 从未在 `runQueryLoop` 后写回，`GetMessages()` 永远返回初始空历史 | #26 | Agent-Core |
| P0-CR-2 | P0 | 核心层 | `BaseTool.InterruptBehavior()` 默认返回 `InterruptBehaviorCancel`，设计要求 `InterruptBlock` | #26 | Agent-Core |
| P0-CR-3 | P0 | 核心层 | `input_json_delta` 事件发送 `MsgTypeToolUseStart`（错误类型），TUI 产生重复 start 事件渲染 | #26 | Agent-Core |
| P0-CR-4 | P0 | 核心层 | `buildRequest` 调用所有工具的 `Description(nil, nil)`，若工具未兼容 nil 参数则 nil panic | #26 | Agent-Core |
| P0-CR-5 | P0 | 核心层 | `buildRequest` 中 `FallbackModel != ""` 时无条件替换主模型，主模型永远不被使用 | #26 | Agent-Core |
| P0-CR-6 | P0 | 核心层 | 压缩流水线（snip→micro→auto）完全未在查询循环中调用，上下文无限增长 | #26 | Agent-Core |
| P0-CR-7 | P0 | 工具层 | `removeTagBlock`（`web/webfetch.go`）index 偏移 bug，缺少结束标签时截断整段 HTML | #26 | Agent-Tools |
| P0-CR-8 | P0 | 工具层 | `domainAllowed`（`web/websearch.go`）使用裸 `HasSuffix`，`notevil.com` 可绕过 `evil.com` 封锁 | #26 | Agent-Tools |
| P0-CR-9 | P0 | 基础设施层 | `session.newSessionId()` 仅用毫秒时间戳，无随机后缀，同毫秒并发创建导致 SessionId 碰撞 | #26 | Agent-Infra |

---

## 其他补充事项

- **文档同步规范**已制定：`docs/project/doc-sync-policy.md`，各层 Agent 须按规范补齐设计文档至 v1.1+
- **QA 职责边界说明**：QA 评审聚焦测试角度（单元测试质量、可测试性、功能完整性），代码质量/架构问题由 Tech Lead 代码评审覆盖
