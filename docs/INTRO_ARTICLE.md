# Claude Code Go：一次"零人工代码"的 AI 协作开发实验，诚邀共建

> **本文介绍一个由 9 个 AI Agent 全程协作完成的开源项目——Claude Code Go，并诚邀社区开发者参与共建。**

---

## 项目简介

**Claude Code Go** 是 [Anthropic Claude Code](https://claude.ai/code)（官方 TypeScript CLI）的完整 Go 语言复刻版。它是一个运行在终端中的 AI 编程助手，能够理解你的代码库、执行各种工具操作，并通过自然语言对话帮助你编写、评审和重构代码。

项目地址: [https://github.com/tunsuy/claude-code-go](https://github.com/tunsuy/claude-code-go)

---

## 为什么做这个项目？

作为 Go 开发者，我们常常羡慕 TypeScript/Python 生态中丰富的 AI 开发工具。Claude Code 是 Anthropic 官方推出的一款强大的终端 AI 编程助手，但原版是用 TypeScript 编写的。

我决定用 Go 完整复刻这个项目，不仅是为了给 Go 社区提供一个原生的选择，更是想验证一个大胆的想法：

> **一个非平凡的、多层架构的 Go 应用，能否完全由 AI Agent 协作设计、实现、评审并交付？**

---

## 亮点：零人工代码，全程多 Agent 协作

**本仓库中不存在任何人类编写的生产代码。**

整个项目约 **7,000 行生产代码 + 完整测试套件**，由 9 个 Claude AI Agent 分工协作完成：

| Agent | 职责 |
|-------|------|
| **PM Agent** | 项目计划、里程碑、任务调度 |
| **Tech Lead Agent** | 架构设计、设计文档评审、代码评审 |
| **Agent-Infra** | 基础设施层（类型、配置、状态、会话） |
| **Agent-Services** | 服务层（API 客户端、OAuth、MCP、压缩） |
| **Agent-Core** | 核心引擎（推理循环、工具分发、多 Agent 协调） |
| **Agent-Tools** | 工具层（文件、命令、搜索、Web 等 18 个工具） |
| **Agent-TUI** | 界面层（Bubble Tea MVU、主题、Vim 模式） |
| **Agent-CLI** | 入口层（Cobra CLI、依赖注入、启动流程） |
| **QA Agent** | 测试策略、逐层验收、集成测试 |

各 Agent 在独立的 Git Worktree 分支上并行开发，通过共享代码库、设计文档和 QA 报告进行协作。最终 `go test -race ./...` 全部通过。

这是一次真实规模的 AI 协作开发验证，完整的决策记录都保存在 `docs/project/` 目录中。

---

## 核心功能

### 交互式 TUI
基于 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建的全功能终端界面，支持深色/浅色主题切换。

### 智能工具调用
- 文件读写与编辑
- Shell 命令执行
- 代码搜索（grep/glob）
- Web 抓取与搜索
- 所有操作均经过 **9 层权限审批链**

### 多 Agent 协作
可启动后台子 Agent 并行处理复杂任务，实现真正的"分而治之"。

### MCP 协议支持
通过 [Model Context Protocol](https://modelcontextprotocol.io) 接入外部工具，无限扩展能力边界。

### CLAUDE.md 记忆
自动加载项目目录树中所有 `CLAUDE.md` 文件，让 AI 持续了解你的项目上下文。

### 会话管理
- 恢复历史对话
- 自动压缩过长的上下文
- 3 种压缩策略：Snip（局部截断）、Micro（单消息压缩）、Auto（LLM 驱动总结）

### Vim 模式
输入框支持可选的 Vim 按键绑定，让终端重度用户如鱼得水。

### 灵活认证
- 支持 `ANTHROPIC_API_KEY` 直接认证
- 支持 OAuth 浏览器授权
- 支持多种 API Provider（Anthropic、OpenAI、DeepSeek、Ollama 等）

### 18 个内置斜杠命令
`/help`、`/clear`、`/compact`、`/commit`、`/diff`、`/review`、`/mcp`、`/cost` 等，开箱即用。

---

## 架构设计

项目采用清晰的六层架构，严格遵循"下层不依赖上层"的原则：

```
┌─────────────────────────────────────┐
│  CLI (cmd/claude)                   │  Cobra 入口
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  Bubble Tea MVU 界面
├─────────────────────────────────────┤
│  Tools (internal/tools)             │  18 个内置工具
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  流式推理、工具分发
├─────────────────────────────────────┤
│  Services (internal/api, oauth...)  │  API、OAuth、MCP
├─────────────────────────────────────┤
│  Infra (pkg/types, config, state)   │  类型、配置、状态
└─────────────────────────────────────┘
```

核心设计模式：
- **接口驱动 + 依赖注入**：便于测试和扩展
- **Tool 系统**：BaseTool 嵌入 + 20 余个接口方法
- **权限管道**：9 层决策链保障安全
- **上下文压缩**：3 种策略智能管理 Token

---

## 快速开始

### 安装

```bash
# 从源码编译
git clone https://github.com/tunsuy/claude-code-go.git
cd claude-code-go
make build
export PATH="$PATH:$(pwd)/bin"

# 或使用 go install
go install github.com/tunsuy/claude-code-go/cmd/claude@latest
```

### 使用

```bash
# 设置 API Key
export ANTHROPIC_API_KEY=sk-ant-...

# 启动交互式会话
claude

# 单次提问
claude -p "解释一下这个项目的架构设计"

# 恢复上次会话
claude --resume
```

### 使用其他 LLM Provider

```bash
# 使用 DeepSeek
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.deepseek.com
export OPENAI_MODEL=deepseek-chat
claude

# 使用本地 Ollama
export CLAUDE_PROVIDER=openai
export OPENAI_BASE_URL=http://localhost:11434/v1
export OPENAI_MODEL=llama3
claude
```

---

## 诚邀共建

这个项目虽然已经具备完整功能，但还有很多可以改进和扩展的地方。我们诚挚邀请社区开发者参与共建！

### 你可以贡献的方向

#### Bug 修复与优化
- 发现并修复潜在问题
- 性能优化
- 代码质量改进

#### 新工具开发
在 `internal/tools/` 目录下添加新的内置工具，例如：
- 数据库操作工具
- Docker/K8s 管理工具
- 更多 IDE 集成工具

#### MCP Server 适配
开发更多 MCP Server 集成，扩展 AI 的能力边界。

#### 文档完善
- 改进使用文档
- 添加更多示例
- 翻译成更多语言

#### 测试增强
- 提高测试覆盖率（目标 ≥60%）
- 添加更多集成测试
- 性能基准测试

#### 新功能提议
- 在 [Discussions](https://github.com/tunsuy/claude-code-go/discussions) 中提出你的想法
- 参与功能讨论和设计

### 贡献流程

1. **Fork** 仓库
2. 创建功能分支：`git checkout -b feat/your-feature`
3. 确保测试通过：`make test && make vet`
4. 提交 Pull Request

详细指南请参阅 [CONTRIBUTING.md](../CONTRIBUTING.md)。

### 代码规范

我们遵循严格的 Go 编码规范：
- 使用 `gofmt` 格式化代码
- 所有导出的标识符必须有文档注释
- 错误必须显式处理
- 文件 ≤800 行，函数 ≤80 行

---

## 联系我们

- **GitHub Issues**：报告 Bug 或提出功能请求
- **GitHub Discussions**：技术讨论和问答
- **Pull Requests**：代码贡献

如果你觉得这个项目有价值，请给我们一个 Star，让更多人发现它！

---

## 致谢

- [Anthropic](https://www.anthropic.com/) - 提供 Claude API 和原版 Claude Code
- [Charm](https://charm.sh/) - 提供优秀的终端 UI 库
- 所有贡献者和支持者

---

## 许可证

MIT License - 欢迎自由使用和分发。

---

**让我们一起打造 Go 生态中最好的终端 AI 编程助手！**
