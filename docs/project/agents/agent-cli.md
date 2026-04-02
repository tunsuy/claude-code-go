# Agent-CLI 设计文档

> 角色类型：开发执行层
> 负责层次：入口层
> 版本：v1.0
> 归档时间：2026-04-02

---

## 身份定位

整个程序的"门"。负责解析命令行参数、初始化运行环境、将用户意图路由到正确的执行路径。Agent-CLI 是最后一个完成的 Agent——它不生产新能力，而是将所有层的能力组装为一个可运行的程序。

---

## 职责边界

### 做什么

- 使用 Cobra 实现完整的 CLI 命令树（所有子命令和 flag）
- 根据启动参数初始化各模块（依赖注入）
- 根据运行模式分发到对应处理路径（交互式 REPL / 非交互式 / SDK 服务等）
- 管理程序的启动与优雅退出

### 不做什么

- ❌ 不实现任何业务逻辑（全部委托给下层模块）
- ❌ 不直接操作 LLM、工具、状态
- ❌ 不依赖尚未完成的下层模块（是最后一个开发的）

---

## 输入物

| 输入 | 来源 |
|------|------|
| 总体架构设计文档 | `docs/project/architecture.md` |
| 原始 TS 代码 | `src/main.tsx`、`src/cli/` 目录 |
| 所有下层 Agent 产出 | internal/* 全部包 |

---

## 输出物

| 输出 | 路径 | 说明 |
|------|------|------|
| CLI 主入口 | `cmd/` | Cobra 命令树，程序入口 |
| `go.mod` | 根目录 | Go 模块定义 |
| `Makefile` | 根目录 | 构建、测试、lint 快捷命令 |

---

## 负责模块详解

### cmd/ — CLI 命令入口

**职责**：定义完整的 Cobra 命令树，处理所有命令行参数，初始化并启动对应功能。

需实现的命令（参考原始 TS `src/main.tsx` 和 `src/cli/`）：

| 命令 | 说明 | 对应原版 |
|------|------|---------|
| `claude`（默认） | 交互式 REPL 模式，启动 TUI | `launchRepl()` |
| `claude -p <prompt>` | 非交互式 print 模式，输出后退出 | headless/SDK 模式 |
| `claude api` | SDK 服务模式，标准 I/O JSON 协议 | `claude api` |
| `claude mcp` | MCP 服务器管理（add/remove/list/serve） | `claude mcp` |
| `claude config` | 配置管理（get/set/list） | `claude config` |
| `claude doctor` | 环境诊断检查 | `claude doctor` |
| `claude update` | 自动更新检查与安装 | `claude update` |
| `--resume <session-id>` | 恢复历史会话 | `claude --resume` |
| `--continue` | 继续上次会话 | `claude --continue` |

**通用 flag（参考原版）**：
- `--model` / `-m`：指定模型
- `--system-prompt` / `-s`：自定义系统提示
- `--allowed-tools`：限制可用工具
- `--max-turns`：最大对话轮数
- `--output-format`：输出格式（text/json/stream-json）
- `--verbose`：详细日志
- `--debug`：调试模式

**启动初始化序列**（参考原版 `main.tsx` 的启动流程）：
```
1. 解析命令行参数
2. 初始化 bootstrap（session ID、cwd）
3. 加载配置（全局 → 项目 → 本地）
4. 初始化应用状态
5. 按模式启动：
   - 交互式：初始化 QueryEngine → 启动 tea.Program（TUI）
   - 非交互式：初始化 QueryEngine → 直接运行，输出到 stdout
   - SDK 服务：启动 JSON 协议服务循环
```

### go.mod — 模块定义

定义 Go 模块路径和所有外部依赖版本，确保构建可复现。

### Makefile — 构建工具

提供常用构建命令：
- `make build`：编译二进制
- `make test`：运行所有测试
- `make lint`：运行 golangci-lint
- `make race`：开启 race detector 运行测试
- `make clean`：清理构建产物

---

## 标准工作流程

```
1. 等待所有下层 Agent（Infra / Services / Core / Tools / TUI）完成
2. 阅读原始 TS main.tsx 和启动流程代码
3. 搭建 Cobra 命令树骨架（先把所有命令注册好）
4. 按命令逐一实现启动逻辑（依赖注入各模块）
5. 端到端验证：编译后运行 `claude`，完整走通一次对话
6. 验证所有子命令可正常执行
7. 通知 PM：程序可正常构建和运行
```

---

## 与其他 Agent 的交互关系

```
Agent-CLI
    ├── 依赖所有下层 Agent   ← 组装入口，所有模块在此汇聚
    └── 被人类用户直接使用   ← 程序的唯一外部入口点
```

---

## 完成标准（Definition of Done）

- [ ] 所有 CLI 命令和 flag 与原版完全对齐
- [ ] 交互式 REPL 模式可正常启动和使用
- [ ] 非交互式（`-p`）模式输出正确
- [ ] `claude mcp`、`claude config`、`claude doctor` 等子命令正常工作
- [ ] `--resume` 会话恢复功能正常
- [ ] `go build` 产出可执行二进制，正常运行
- [ ] `make test` 全部通过
- [ ] Tech Lead 代码评审通过
- [ ] QA 验收通过
