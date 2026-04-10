# 多 Agent Harness 设计讨论归档

> 日期：2026-04-10
> 参与者：PM Agent、Tech Lead
> 状态：已归档

---

## 背景

Claude Code Go 项目采用多 Agent 并行开发模式完成了全量交付（M0–M3），26/26 测试通过，综合覆盖率 69.4%。在实际执行过程中暴露了六类协调问题，本次讨论旨在为下一个多 Agent 项目设计可复用的 Harness（脚手架基础设施）。

---

## 问题诊断

### 问题 1：角色越界

**现象**：PM Agent 在协调工作中直接修改了源代码（应由对应 Agent 负责）。

**根因**：角色边界仅靠 Prompt 约束，没有技术层面的强制机制。当 PM 发现某个"小问题"时，动手修复比派发任务成本更低。

**影响**：污染了 Git 历史的职责归属；同时可能引入未经对应 Agent 测试的变更。

---

### 问题 2：权限缺失

**现象**：专职 Agent（如 Agent-Tools）无法写入自己职责范围内的目录（`internal/tools/`），任务被阻塞。

**根因**：`settings.local.json` 的 `permissions.allow` 列表没有预先配置各 Agent 的写入路径。

**影响**：Agent 需要 PM 介入修改配置，增加了协调开销，破坏了并行流。

---

### 问题 3：状态漂移

**现象**：`status.md` 手动维护，出现"任务已完成但标记为进行中"的失真状态；PM 在下一轮协调时依据错误状态做决策。

**根因**：没有自动化机制在 Agent 完成任务后更新状态文件。

**影响**：PM 难以准确判断哪些任务真正完成，哪些需要跟进。

---

### 问题 4：上下文丢失

**现象**：跨对话（context window 切换）后，新的 Claude Code 会话需要从头阅读所有文档才能重建项目状态，耗时且容易遗漏。

**根因**：项目状态分散在多个 Markdown 文档中，没有一个机器可读的单一状态源。

**影响**：每次会话启动都需要额外的"重建上下文"阶段，降低了协作效率。

---

### 问题 5：输出格式不统一

**现象**：各 Agent 完成任务后的汇报格式不一——有的用 Markdown 列表，有的用散文，有的只有部分信息（如缺少覆盖率数据）。

**根因**：没有强制执行的输出协议，Agent 自由发挥。

**影响**：PM 无法机器化解析 Agent 输出，必须人工阅读和提取信息。

---

### 问题 6：仅靠 Prompt 约束角色

**现象**：某些 Agent 在特殊情况下（如路径不存在、依赖未就绪）绕过了 Prompt 中定义的行为限制。

**根因**：Prompt 是建议性的，不是强制性的；Claude 的决策过程可以在"帮助用户"的驱动下覆盖角色约束。

**影响**：项目治理退化为依赖 AI 的自觉性，而不是系统设计。

---

## Harness 设计

### 总体架构（5 层）

```
┌─────────────────────────────────────────────────┐
│ Layer 5: 上下文持久化                             │
│  .claude/harness/state.json                      │
│  → 机器可读项目状态，跨会话快速恢复               │
├─────────────────────────────────────────────────┤
│ Layer 4: 自动化钩子                              │
│  .claude/hooks/update-state.sh                   │
│  → PostToolUse 触发，自动更新 last_updated        │
├─────────────────────────────────────────────────┤
│ Layer 3: 输出协议                                │
│  docs/project/harness/protocols/agent-output.md  │
│  → 强制 JSON 输出格式，机器可解析                 │
├─────────────────────────────────────────────────┤
│ Layer 2: 任务注册表                              │
│  docs/project/harness/tasks/task-registry.yaml   │
│  → 结构化任务数据，替代手动 status.md             │
├─────────────────────────────────────────────────┤
│ Layer 1: 角色强制                                │
│  docs/project/agents/<role>.md                   │
│  → 路径白名单、禁止操作、输出协议引用             │
│  （Harness Integration 章节追加至 v2.0 文件末尾） │
└─────────────────────────────────────────────────┘
```

### Layer 1：角色强制

**机制**：每个 Agent 在启动时加载对应的 `docs/project/agents/<role>.md` 文件中的 `## Harness Integration` 章节。该章节明确定义：
- **Allowed Write Paths**：Agent 被允许写入的目录（精确到层级）
- **Forbidden Actions**：禁止操作的白名单（如"不得修改 `pkg/types/`"）
- **Output Protocol**：完成任务后必须遵循的输出格式引用

**设计决策**：
- 文件名与角色名一一对应，方便快速查找
- 路径白名单优先于 Prompt 中的描述性约束
- 采用声明式格式，避免歧义

### Layer 2：任务注册表

**机制**：`task-registry.yaml` 作为所有任务的单一真相来源，替代手动维护的 `status.md`。

**字段设计**：
```yaml
- id: "T-001"           # 唯一标识，便于交叉引用
  role: "agent-cli"     # 负责角色，防止任务错派
  status: "completed"   # 机器可读状态
  depends_on: []        # 显式依赖声明
  artifacts: []         # 产出物路径，便于验证
  acceptance_criteria:  # 明确的完成标准
```

**设计决策**：选择 YAML 而非 JSON，因为任务描述可能包含多行文本，YAML 的可读性更好。

### Layer 3：输出协议

**机制**：所有 Agent 完成任务后，必须输出一个标准 JSON 块。PM Agent 通过解析此 JSON 更新任务状态，而不是阅读散文。

**设计决策**：
- 强制 JSON 格式而非 Markdown，便于自动化处理
- `issues` 字段允许 Agent 上报阻塞项，而不是沉默失败
- `coverage` 字段强制 Agent 运行测试并汇报覆盖率

### Layer 4：自动化钩子

**机制**：`PostToolUse` 钩子在 Agent 执行写操作后自动运行 `update-state.sh`，更新 `state.json` 的 `last_updated` 字段。

**设计决策**：
- 钩子只更新 `last_updated`（活动信号），不做复杂逻辑，避免引入新的故障点
- 使用 `PostToolUse` 而非 `PreToolUse`，确保只有成功的写操作才触发更新

### Layer 5：上下文持久化

**机制**：`state.json` 是项目状态的机器可读快照，包含：
- 当前里程碑
- 各里程碑完成状态
- 活跃任务列表
- 阻塞任务列表
- 一句话摘要

**设计决策**：新会话启动时，Agent 只需读取 `state.json` 即可在 5 秒内重建上下文，而不是阅读 10+ 个 Markdown 文档。

---

## 对下一个多 Agent 项目的复用建议

### 启动清单

1. **项目初始化时**：在 `docs/project/agents/<role>.md` 中为每个角色追加 `## Harness Integration` 章节，按项目角色定制路径白名单
2. **首次 PM 规划时**：填充 `task-registry.yaml`（至少包含 P0 任务）
3. **配置 `settings.local.json`**：添加所有 Agent 的写入权限 + PostToolUse 钩子
4. **发布 agent-output 协议**：在 kick-off 文档中明确引用，确保所有 Agent 知晓

### 已知局限

1. **钩子脚本是轻量级的**：`update-state.sh` 只更新 `last_updated`，不自动解析 Agent 输出更新任务状态。完整的状态同步仍需 PM 手动运行或通过更复杂的脚本实现。

2. **角色强制是软性的**：`docs/project/agents/<role>.md` 的 Harness Integration 路径白名单是 Prompt-level 约束，不是 OS-level 权限控制。强制力度取决于 Agent 是否正确加载了角色文件。

3. **task-registry.yaml 需要手动维护**：任务状态更新目前依赖 PM Agent 手动修改，未来可以通过解析 Agent 输出的 JSON 自动化。

### 演进路径

```
当前 Harness（本次实施）
  ↓ 增加 PM 自动解析 Agent JSON 输出
中级 Harness
  ↓ 增加任务状态机（pending→in_progress→completed 自动流转）
高级 Harness
  ↓ 增加 Agent 健康检查、超时重派
完整 Harness
```

---

## 实施结果

本次 Harness 改进在现有 Agent 定义文件中追加了 Harness Integration 章节，并新建了协议/注册表/状态文件，修改了 1 个配置文件和 1 个上下文注入文件：

| 类型 | 路径 | 作用 |
|------|------|------|
| 角色约束（追加） | `docs/project/agents/pm-agent.md` | PM Agent Harness Integration 章节 |
| 角色约束（追加） | `docs/project/agents/agent-cli.md` | CLI Agent Harness Integration 章节 |
| 角色约束（追加） | `docs/project/agents/agent-core.md` | Core Agent Harness Integration 章节 |
| 角色约束（追加） | `docs/project/agents/agent-infra.md` | Infra Agent Harness Integration 章节 |
| 角色约束（追加） | `docs/project/agents/agent-services.md` | Services Agent Harness Integration 章节 |
| 角色约束（追加） | `docs/project/agents/agent-tools.md` | Tools Agent Harness Integration 章节 |
| 角色约束（追加） | `docs/project/agents/agent-tui.md` | TUI Agent Harness Integration 章节 |
| 角色约束（追加） | `docs/project/agents/qa-agent.md` | QA Agent Harness Integration 章节 |
| 角色约束（追加） | `docs/project/agents/tech-lead-agent.md` | Tech Lead Harness Integration 章节 |
| 任务注册表 | `docs/project/harness/tasks/task-registry.yaml` | 所有历史任务结构化记录 |
| 输出协议 | `docs/project/harness/protocols/agent-output.md` | Agent 标准输出格式 |
| 状态快照 | `.claude/harness/state.json` | 机器可读项目状态（Claude Code 运行时文件） |
| 钩子脚本 | `.claude/hooks/update-state.sh` | PostToolUse 自动更新（Claude Code 运行时文件） |
| 配置更新 | `.claude/settings.local.json` | 添加 hooks 配置 |
| 上下文注入 | `.claude/CLAUDE.md` | 添加 Multi-Agent Session Start Checklist |

> **设计说明**：Harness 文档（角色约束、任务注册表、输出协议）存放在 `docs/project/` 下，而非 `.claude/` 下，以确保非 Claude Code CLI 工具（Cursor、Windsurf 等）的用户也能访问这些协作规范。只有 Claude Code 运行时文件（`state.json`、hooks、`settings.local.json`）保留在 `.claude/` 下。
