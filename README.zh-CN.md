<p align="center">
  <img src="assets/logo.png" alt="Claude Code Go Logo" width="200">
</p>

<h1 align="center">Claude Code Go</h1>

<p align="center">
  <strong>🤖 Claude Code 的 Go 语言复刻版 — 终端里的 AI 编程助手</strong>
</p>

<p align="center">
  <a href="https://golang.org/dl/"><img src="https://img.shields.io/badge/go-1.21+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 版本"></a>
  <a href="https://goreportcard.com/report/github.com/tunsuy/claude-code-go"><img src="https://goreportcard.com/badge/github.com/tunsuy/claude-code-go?style=flat-square" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/tunsuy/claude-code-go"><img src="https://pkg.go.dev/badge/github.com/tunsuy/claude-code-go.svg" alt="Go Reference"></a>
  <a href="https://github.com/tunsuy/claude-code-go/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/tunsuy/claude-code-go/ci.yml?branch=main&style=flat-square&logo=github&label=CI" alt="CI"></a>
  <a href="https://github.com/tunsuy/claude-code-go/releases"><img src="https://img.shields.io/github/v/release/tunsuy/claude-code-go?style=flat-square&logo=github" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="License"></a>
  <a href="https://github.com/tunsuy/claude-code-go/pulls"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square" alt="PRs Welcome"></a>
</p>

<p align="center">
  <a href="README.md">English</a> •
  <a href="README.zh-CN.md">简体中文</a>
</p>

---

## 这是什么

本项目是对 [Claude Code](https://claude.ai/code)（Anthropic 官方 TypeScript CLI）**完整功能的 Go 语言复刻**，逐模块对照原版源码实现，覆盖 TUI 界面、工具调用、权限系统、多 Agent 协调、MCP 协议、会话管理等全部核心功能。

### 开发方式：零人工代码，全程多 Agent 协作

> **本仓库中不存在任何人类编写的生产代码。**

整个项目由 **9 个 Claude AI Agent** 分工协作完成——从架构设计、详细设计文档、并行编码实现、代码评审，到 QA 验收与集成测试，全流程均由 AI 驱动：

```
PM Agent          →  项目计划、里程碑、任务调度
Tech Lead Agent   →  架构设计、设计文档评审、代码评审
Agent-Infra       →  基础设施层（类型、配置、状态、会话）
Agent-Services    →  服务层（API 客户端、OAuth、MCP、压缩）
Agent-Core        →  核心引擎（推理循环、工具分发、多 Agent 协调）
Agent-Tools       →  工具层（文件、命令、搜索、Web 等 18 个工具）
Agent-TUI         →  界面层（Bubble Tea MVU、主题、Vim 模式）
Agent-CLI         →  入口层（Cobra CLI、依赖注入、启动流程）
QA Agent          →  测试策略、逐层验收、集成测试
```

各 Agent 在独立 Git Worktree 分支上并行开发，通过共享代码库、设计文档和 QA 报告协作交互。最终产出约 **7,000 行生产代码 + 完整测试套件**，`go test -race ./...` 全部通过。

这是一次真实规模的验证：**非平凡的多层 Go 应用可以完全由 AI Agent 异步协作设计、实现、评审并交付**。完整决策记录见 [`docs/project/`](docs/project/)。

---

## 功能特性

- **交互式 TUI** — 基于 [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建的全功能终端界面，支持深色/浅色主题
- **智能工具调用** — 文件读写、命令执行、代码搜索等，所有操作均经过权限层审批
- **多 Agent 协作** — 可启动后台子 Agent 并行处理任务
- **MCP 支持** — 通过 [Model Context Protocol](https://modelcontextprotocol.io) 接入外部工具
- **CLAUDE.md 记忆** — 自动加载项目目录树中所有 `CLAUDE.md` 文件作为上下文
- **会话管理** — 恢复历史对话；自动压缩过长的上下文历史
- **Vim 模式** — 输入框支持可选的 Vim 按键绑定
- **OAuth + API Key 认证** — 支持 Anthropic OAuth 登录或直接配置 `ANTHROPIC_API_KEY`
- **18 个内置斜杠命令** — `/help`、`/clear`、`/compact`、`/commit`、`/diff`、`/review`、`/mcp` 等
- **流式响应** — 实时 token 流式输出，支持 thinking block 展示

## 架构设计

Claude Code Go 采用六层架构：

```
┌─────────────────────────────────────┐
│  CLI (cmd/claude)                   │  Cobra 入口
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  Bubble Tea MVU 界面
├─────────────────────────────────────┤
│  Tools (internal/tools)             │  文件、命令、搜索、MCP 工具
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  流式推理、工具分发、多 Agent 协调
├─────────────────────────────────────┤
│  Services (internal/api, oauth,     │  Anthropic API、OAuth、MCP 客户端
│            mcp, compact)            │
├─────────────────────────────────────┤
│  Infra (pkg/types, internal/config, │  类型、配置、状态、钩子、插件
│         state, session, hooks)      │
└─────────────────────────────────────┘
```

详细说明见 [`docs/project/architecture.md`](docs/project/architecture.md)。

## 环境要求

- Go 1.21 或更高版本
- [Anthropic API Key](https://console.anthropic.com/) **或** Claude.ai 账号（OAuth）

## 安装

### 从源码编译

```bash
git clone https://github.com/tunsuy/claude-code-go.git
cd claude-code-go
make build
# 产物路径：./bin/claude
```

将 `bin` 目录加入 `PATH`：

```bash
export PATH="$PATH:$(pwd)/bin"
```

### 使用 `go install`

```bash
go install github.com/tunsuy/claude-code-go/cmd/claude@latest
```

## 快速开始

```bash
# 设置 API Key（或使用 OAuth，参见下方认证说明）
export ANTHROPIC_API_KEY=sk-ant-...

# 在当前目录启动交互式会话
claude

# 单次提问后退出
claude -p "解释一下这个项目的主入口"

# 恢复最近一次会话
claude --resume
```

## 认证

**API Key（推荐用于 CI/脚本）：**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

**OAuth（推荐用于交互式使用）：**

```bash
claude /config    # 在浏览器中打开 OAuth 授权流程
```

## 使用说明

### 交互模式

```
claude [flags]
```

| 参数 | 说明 |
|------|------|
| `--resume` | 恢复最近一次会话 |
| `--session <id>` | 按 ID 恢复指定会话 |
| `--model <name>` | 覆盖默认 Claude 模型 |
| `--dark` / `--light` | 强制使用深色或浅色主题 |
| `--vim` | 启用 Vim 按键绑定 |
| `-p, --print <prompt>` | 非交互模式：执行单次提问后退出 |

### 斜杠命令

在输入框中输入 `/` 即可查看所有可用命令：

| 命令 | 说明 |
|------|------|
| `/help` | 显示所有命令 |
| `/clear` | 清空对话历史 |
| `/compact` | 压缩历史以减少上下文占用 |
| `/exit` | 退出 Claude Code |
| `/model` | 切换 Claude 模型 |
| `/theme` | 切换深色/浅色主题 |
| `/vim` | 切换 Vim 模式 |
| `/commit` | 生成 git commit 信息 |
| `/review` | 评审近期改动 |
| `/diff` | 查看当前 diff |
| `/mcp` | 管理 MCP 服务器 |
| `/memory` | 查看已加载的 CLAUDE.md 文件 |
| `/session` | 查看会话信息 |
| `/status` | 查看 API/连接状态 |
| `/cost` | 查看 token 用量及预估费用 |

## 开发指南

### 前置条件

- Go 1.21+
- `golangci-lint`（可选，用于代码检查）

### 构建与测试

```bash
# 构建
make build

# 运行所有测试
make test

# 生成覆盖率报告
make test-cover

# 静态检查
make vet

# 代码检查（需安装 golangci-lint）
make lint

# 构建 + 测试 + 静态检查
make all
```

### 项目结构

```
claude-code-go/
├── cmd/claude/          # CLI 入口
├── internal/
│   ├── api/             # Anthropic API 客户端与流式传输
│   ├── bootstrap/       # 应用初始化
│   ├── commands/        # 斜杠命令处理器
│   ├── compact/         # 对话压缩
│   ├── config/          # 配置（文件 + 环境变量）
│   ├── coordinator/     # 多 Agent 协调器
│   ├── engine/          # 推理引擎、工具分发
│   ├── hooks/           # 工具前/后钩子
│   ├── mcp/             # MCP 服务器管理
│   ├── memdir/          # CLAUDE.md 加载器
│   ├── oauth/           # OAuth2 流程
│   ├── permissions/     # 工具权限层
│   ├── plugin/          # 插件系统
│   ├── session/         # 会话持久化
│   ├── state/           # 应用状态
│   ├── tools/           # Tool 接口、注册表及内置工具实现
│   │   ├── agent/       #   子 Agent 与消息工具
│   │   ├── fileops/     #   文件读写/编辑/搜索工具
│   │   ├── interact/    #   用户交互与 Worktree 工具
│   │   ├── mcp/         #   MCP 工具适配器
│   │   ├── misc/        #   杂项工具
│   │   ├── shell/       #   Bash 执行工具
│   │   ├── tasks/       #   任务列表工具
│   │   └── web/         #   网页抓取与搜索工具
│   └── tui/             # Bubble Tea UI 组件
├── pkg/
│   └── types/           # 共享公共类型
├── docs/                # 设计文档与 QA 报告
├── Makefile
└── go.mod
```

## 贡献指南

欢迎贡献！提交 Pull Request 前请先阅读 [CONTRIBUTING.md](CONTRIBUTING.md)。

快速检查清单：
- Fork 仓库并创建功能分支
- 确保 `make test` 和 `make vet` 通过
- 为新功能编写测试
- 遵循现有代码风格（运行 `gofmt ./...`）
- 使用提供的模板提交 Pull Request

## 安全

如需报告安全漏洞，请参阅 [SECURITY.md](SECURITY.md)。**请勿**在公开的 GitHub Issue 中披露安全问题。

## 许可证

本项目基于 MIT 许可证——详见 [LICENSE](LICENSE)。

## 相关项目

- [claude-code](https://github.com/anthropics/claude-code) — 原版 TypeScript CLI
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — Anthropic API 官方 Go SDK
- [Model Context Protocol](https://modelcontextprotocol.io) — AI 接入工具的开放标准
