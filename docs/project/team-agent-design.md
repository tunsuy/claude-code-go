# Claude Code Go — 多 Agent 团队方案设计

> 归档时间：2026-04-02
> 背景：将 Claude Code（TypeScript）全量重写为 Go 语言版本，架构和模块设计完全不变，仅按 Go 语言特性重写。

---

## 一、项目基本信息

| 项目名 | claude-code-go |
|-------|---------------|
| 目标 | 将 Claude Code TypeScript 版本全量重写为 Go |
| 原始代码规模 | ~1,900 个文件，512K+ LOC |
| 仓库地址 | /Users/tunsuytang/ts/claude-code-go |
| 仓库策略 | 单一 Git 仓库 + Worktree 分支隔离（方案A） |
| 架构原则 | 模块设计完全不变，按 Go 语言特性重写 |

---

## 二、技术栈映射

| 原始（TypeScript） | Go 对应技术 |
|-------------------|------------|
| TypeScript | Go |
| Bun runtime | Go 原生二进制 |
| React + Ink（Terminal UI） | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Zod schema 验证 | go-playground/validator 或手写 |
| Commander.js | [cobra](https://github.com/spf13/cobra) |
| Anthropic SDK | [anthropic-sdk-go](https://github.com/anthropic-ai/anthropic-sdk-go)（官方） |
| MCP SDK | mcp-go 或手写 |
| OAuth 2.0 / JWT | golang.org/x/oauth2 + golang-jwt/jwt |
| OpenTelemetry | go.opentelemetry.io/otel |
| Protobuf | google.golang.org/protobuf |
| Promise / async-await | goroutine + channel |
| React 状态管理 | Bubble Tea Model/Update（Elm 架构） |
| EventEmitter | channel 广播 / EventBus |
| ripgrep 调用 | 继续调用 rg 二进制，或 Go 原生 filepath.Walk |

---

## 三、团队角色设计

### 3.1 项目治理层

#### 🗂️ 项目经理（PM Agent）
- **运作方式**：专职 PM Agent，自主协调所有开发 Agent，人类只做最终决策
- **核心职责**：
  - 任务拆解：将架构文档分解为原子级任务，录入任务系统
  - 依赖管理：维护任务依赖图，确保 Agent 不会阻塞等待
  - 进度跟踪：定期轮询各 Agent 状态，更新里程碑
  - 风险预警：识别阻塞点、接口不兼容、进度偏差
  - 接口仲裁：当两个 Agent 对接口产生分歧时，协调决策
  - 每日站会：汇总各 Agent 进展，生成每日状态报告
  - 验收把关：每个模块完成后，触发 QA Agent 验收
  - 变更控制：管理需求/接口变更，评估影响范围并通知相关 Agent
- **使用工具**：TaskCreate/Update/List/Get、SendMessage、BashTool、FileWrite

#### 🏗️ 技术负责人（Tech Lead Agent）
- **核心职责**：
  - 架构设计：输出 Go 项目结构、包划分方案
  - 接口规范：定义所有核心 interface，形成"接口契约文档"
  - 公共类型：定义全局 types、error codes、常量
  - 技术决策：TUI 框架、并发模型、测试框架等选型
  - 代码评审：Review 各 Agent 提交的核心代码
  - 规范制定：Go coding style、日志规范、错误处理规范
  - 技术债跟踪：记录临时方案，规划后续优化

---

### 3.2 开发执行层

Agent 划分与架构分层一一对应，每个层次由一个 Agent 全权负责。

| Agent | 对应层次 | 负责模块 | 主要依赖 |
|-------|---------|---------|---------|
| **Agent-Infra** | 基础设施层 | 公共类型包、配置系统、应用状态、会话存储 | 无（最底层） |
| **Agent-Services** | 服务层 | API 客户端、MCP 客户端、OAuth | Agent-Infra |
| **Agent-Core** | 核心层 | 查询引擎、权限系统、上下文压缩、Hooks 系统 | Agent-Infra、Agent-Services |
| **Agent-Tools** | 工具层 | 工具接口定义 + 全部内置工具实现 | Agent-Infra、Agent-Core（接口） |
| **Agent-TUI** | TUI 层 | TUI 界面、Slash 命令、多 Agent 协调 | Agent-Core、Agent-Infra |
| **Agent-CLI** | 入口层 | CLI 入口、程序启动与模式分发 | 所有层（组装点） |

---

### 3.3 质量保障层

#### 🧪 QA Agent
- **核心职责**：
  - 测试规范：制定测试策略、覆盖率要求（目标 ≥ 70%）
  - 单元测试：对接各 Agent 的模块，补充 table-driven tests
  - 集成测试：跨模块端到端测试
  - 回归测试：与 TypeScript 版本行为对比
  - 性能基准：benchmark 测试，与原版对比
  - 验收报告：每模块出具测试报告，PM 依此放行

---

## 四、仓库策略

**选定方案：单一 Git 仓库 + Worktree 分支隔离**

```
claude-code-go/               ← 主仓库（main 分支，骨架+接口）
├── feat/agent-infra          ← Agent-Infra worktree（基础设施层）
├── feat/agent-services       ← Agent-Services worktree（服务层）
├── feat/agent-core           ← Agent-Core worktree（核心层）
├── feat/agent-tools          ← Agent-Tools worktree（工具层）
├── feat/agent-tui            ← Agent-TUI worktree（TUI 层）
└── feat/agent-cli            ← Agent-CLI worktree（入口层）
```

**原则：**
- `main` 分支只接受 Tech Lead Review 通过的 PR
- 各 Agent 在自己的 worktree 分支开发，不直接推 main
- 接口变更必须走 PM 审批 → 评估影响 → 通知受影响方

---

## 五、项目交付计划

### 里程碑

| 阶段 | 周期 | 内容 | 负责方 |
|------|------|------|-------|
| **M0：基础建设** | Week 1-2 | 架构设计、接口文档、公共 types、go.mod、CI/CD | Tech Lead + PM + Agent-Infra |
| **M1：核心并行开发** | Week 3-6 | 8 个开发 Agent 全速并行，QA 持续接收 | 全体开发 Agent + QA |
| **M2：集成稳定** | Week 7-8 | 模块集成、集成测试、回归测试、风险收敛 | Tech Lead + 全体 + QA |
| **M3：交付** | Week 9 | 可运行 Go 版本 + 完整测试报告 + 文档 | PM 主导 |

### 并发节奏（Week 3-6）

```
Agent-Core        [████████████████]
Agent-Tools              [████████████████]
Agent-Services    [████████████████]
Agent-TUI         [████████████████]
Agent-Commands           [████████████████]
Agent-Bridge             [████████████████]
Agent-Coordinator [████████████████]
QA                [持续接收各模块，持续测试>>>>>>>>>>>>]
```

---

## 六、治理机制

### 6.1 任务状态机

```
pending → in_progress → code_review → testing → completed
                                          ↓
                                       blocked（需 PM 介入）
```

### 6.2 PM 每日巡检流程

```
1. TaskList()           → 查看所有任务状态
2. 识别 blocked 任务    → SendMessage 给相关 Agent 解锁
3. 识别进度偏差         → 调整资源/优先级
4. 更新进度文档
5. 输出每日状态报告
```

### 6.3 接口契约优先（Contract-First）

```
Tech Lead 输出接口文档
    → 所有相关 Agent 确认（SendMessage 签字）
    → 才能开始编码
变更必须：PM 审批 → 评估影响范围 → 通知受影响 Agent
```

### 6.4 完成标准（Definition of Done）

每个模块合并到 main 前必须满足：
- [ ] 代码实现完成
- [ ] 单元测试通过，覆盖率 ≥ 70%
- [ ] Tech Lead 代码评审通过
- [ ] QA 验收测试通过
- [ ] 接口文档已更新
- [ ] PM 标记任务为 completed

---

## 七、并发控制策略

- **同时活跃 Agent 数**：3-5 个核心并行（避免接口混乱）
- **优先级顺序**：Agent-Infra > Agent-Core/Agent-Services > 其余开发 Agent > 集成
- **阻塞处理**：任何 Agent 遇到阻塞，立即上报 PM，PM 在 24h 内协调解决

---

## 八、下一步行动

1. **立即**：Tech Lead Agent 启动，输出 Go 项目骨架 + 接口契约文档
2. **同步**：PM Agent 完成完整任务拆解，建立 TaskList
3. **随后**：Agent-Infra 启动，搭建 go.mod + 基础包 + CI/CD
4. **Week 3 起**：其余开发 Agent 按依赖顺序陆续启动
