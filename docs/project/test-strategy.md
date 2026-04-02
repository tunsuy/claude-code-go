# 测试策略文档

> 负责 Agent：QA Agent
> 日期：2026-04-02

## 1. 测试目标

- **行为对齐**：确保 Go 重写版本与原版 TypeScript Claude Code 在核心行为上保持一致，包括 LLM 交互、工具调用、会话管理等关键路径。
- **质量保障**：通过分层测试覆盖各架构层次，提前发现回归缺陷，保障每次迭代的交付质量。
- **可维护性**：建立标准化测试规范和基础设施，降低测试编写成本，使测试随业务代码同步演进。
- **性能基线**：建立关键路径的性能 Benchmark，防止性能退化，并为与 TS 版本的横向对比提供数据支撑。

## 2. 覆盖率目标

| 层次 | 目标覆盖率 | 说明 |
|------|-----------|------|
| 基础设施层（infra） | ≥ 80% | 文件系统、HTTP 客户端、配置加载等，逻辑确定性强 |
| 服务层（service） | ≥ 70% | LLM 调用、MCP 协议等，外部依赖多，以集成测试为主 |
| 核心层（core） | ≥ 75% | Agent 主循环、上下文管理，业务核心，重点覆盖 |
| 工具层（tools） | ≥ 70% | 各工具实现，以单元 + 集成混合方式覆盖 |
| TUI 层（tui） | ≥ 60% | 渲染逻辑较难自动化，以组件逻辑测试为主 |
| 入口层（cmd） | ≥ 60% | CLI 参数解析、启动流程，以集成测试为主 |

> 覆盖率统计命令：`go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out`

## 3. 测试分层策略

### 3.1 单元测试

**工具选型**

| 工具 | 用途 |
|------|------|
| 标准库 `testing` | 基础测试框架 |
| `github.com/stretchr/testify/assert` | 断言库，提升可读性 |
| `github.com/stretchr/testify/require` | 断言失败时立即停止 |
| `github.com/stretchr/testify/mock` | 接口 Mock 生成 |
| `github.com/golang/mock/gomock`（备选） | 编译时类型安全的 Mock |

**Mock 策略**

采用 **interface-based mock** 模式：所有外部依赖（LLM API、文件系统、时钟等）均通过接口抽象，测试时注入 Mock 实现。推荐优先使用 `testify/mock`，复杂场景可使用 `gomock` 搭配 `mockgen` 自动生成。

**各层单元测试重点**

- **基础设施层**：文件读写边界条件、配置解析错误路径、HTTP 重试逻辑。
- **服务层**：请求构造、响应解析、错误映射；LLM streaming 解析逻辑。
- **核心层**：Agent 状态机流转、工具调用分发、上下文截断策略、消息历史管理。
- **工具层**：各工具的参数校验、执行结果格式化、错误处理分支。
- **TUI 层**：Model 状态更新逻辑（`Update` 函数）、消息渲染格式（`View` 函数）。
- **入口层**：CLI flag 解析、环境变量读取、配置优先级。

### 3.2 集成测试

**跨模块集成测试范围**

- `core` ↔ `service`：Agent 主循环调用 LLM 服务，验证完整的一次对话往返。
- `core` ↔ `tools`：工具调用分发及结果回填全链路。
- `service` ↔ `infra`：HTTP 客户端与 Anthropic API 协议适配（使用 httptest mock server）。
- `cmd` ↔ 全栈：CLI 启动到首条 LLM 响应的端到端 smoke test。

**外部依赖 Mock 策略**

| 外部依赖 | Mock 方案 |
|---------|----------|
| Anthropic API | `net/http/httptest` 搭建本地 mock server，回放预录制响应 |
| MCP Server | 实现轻量 MCP stub server，覆盖协议握手与工具调用 |
| 文件系统 | 使用 `t.TempDir()` 创建临时目录，测试后自动清理 |
| 系统时钟 | 通过 `clock` 接口注入，测试时使用固定时间 |
| 环境变量 | `t.Setenv()` 覆盖，测试结束后自动还原 |

### 3.3 行为对比测试

**对比方法**

1. **Golden File 模式**：对于工具调用结果、消息格式化等有确定性输出的场景，将 TS 版本的实际输出保存为 `testdata/*.golden` 文件，Go 版本测试输出与之对比（使用 `-update` flag 更新 golden file）。
2. **协议录制回放**：使用 TS 版本与 API 的真实交互录制 HTTP 请求/响应对（VCR 模式），Go 版本在相同输入下回放，对比最终行为。
3. **并行运行对比**：在 CI 中对同一组测试用例分别运行 TS 版和 Go 版，对比 stdout/exit code。

**关键行为基准**

- [ ] `Bash` 工具执行结果与 TS 版一致（stdout/stderr/exit code）
- [ ] `ReadFile` / `WriteFile` / `EditFile` 工具的文件操作语义
- [ ] LLM 对话历史格式（system prompt、message roles、tool_use/tool_result blocks）
- [ ] 流式输出（streaming）的 token 拼接与事件顺序
- [ ] 错误场景下的重试与降级行为
- [ ] MCP 协议握手及工具注册流程

### 3.4 性能测试

**Benchmark 框架**

使用标准库 `testing.B`，通过 `go test -bench=. -benchmem -benchtime=10s` 运行。

**性能目标（与 TS 版本对比）**

| 基准场景 | 目标 |
|---------|------|
| Agent 主循环单次迭代（不含 LLM 延迟） | ≤ TS 版本 1.5x 时间 |
| 工具调用分发（100 种工具） | < 1ms/op |
| 消息历史序列化（1000 条消息） | < 50ms/op |
| TUI 渲染单帧（1000 行输出） | < 16ms/frame（60fps） |
| 配置文件加载 | < 5ms |

**关键路径**

1. **LLM 主循环**：`core.Agent.Run()` 的单次迭代，重点关注内存分配次数。
2. **工具执行**：工具注册查找 + 参数反序列化 + 执行调度的综合耗时。
3. **TUI 渲染**：Bubble Tea `Update` + `View` 的端到端耗时。

## 4. 测试基础设施

### 4.1 目录约定

```
claude-code-go/
├── internal/
│   └── <package>/
│       ├── xxx.go              # 实现文件
│       └── xxx_test.go         # 单元测试（与实现同目录，package xxx_test）
├── pkg/
│   └── testutil/               # 共享测试工具包
│       ├── testutil.go         # 通用 helper（AssertNoError 等）
│       ├── mock_llm.go         # LLM 服务 Mock
│       ├── mock_fs.go          # 文件系统 Mock
│       └── fixtures/           # 测试夹具数据
├── testdata/                   # Golden files、录制的 HTTP 响应等
│   ├── golden/
│   └── vcr/
└── test/
    └── integration/            # 跨模块集成测试（独立目录）
        └── agent_e2e_test.go
```

**约定说明**：
- 单元测试与实现文件同目录，使用 `package xxx_test`（黑盒测试）；需要访问内部符号时使用 `package xxx`（白盒测试）。
- 集成测试放在独立的 `test/integration/` 目录，使用 build tag `//go:build integration` 与单元测试隔离。
- Golden file 和 VCR 录制统一放在顶层 `testdata/` 目录。

### 4.2 测试辅助包

`pkg/testutil/` 提供项目全局共享的测试工具，**仅在 `_test.go` 文件中导入**：

| 文件 | 内容 |
|------|------|
| `testutil.go` | `AssertNoError`、`AssertError`、`MustReadFile`、`TempDir` 等通用 helper |
| `mock_llm.go` | `MockLLMClient`：实现 `service.LLMClient` 接口，可配置返回内容 |
| `mock_fs.go` | `MockFS`：内存文件系统，用于工具层测试 |
| `golden.go` | `CheckGolden`：Golden file 断言，支持 `-update` flag 自动更新 |
| `httptest.go` | `NewMockAPIServer`：预置 Anthropic API mock server 工厂函数 |

### 4.3 CI 命令

```bash
# 运行全部单元测试（含竞态检测）
go test -race ./...

# 生成覆盖率报告
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# 运行集成测试（需要设置 build tag）
go test -race -tags=integration ./test/integration/...

# 运行性能 Benchmark
go test -bench=. -benchmem -benchtime=10s ./...

# 查看 HTML 覆盖率报告
go tool cover -html=coverage.out -o coverage.html
```

**CI 流水线阶段建议**：

| 阶段 | 命令 | 触发条件 |
|------|------|---------|
| 快速验证 | `go test -race ./...` | 每次 PR push |
| 覆盖率检查 | `go test -coverprofile=... ./...` | 每次 PR push |
| 集成测试 | `go test -tags=integration ./test/...` | PR merge 到 main |
| Benchmark | `go test -bench=. ./...` | 每周定期 / 性能相关 PR |

### 4.4 Mock 基础设施

各层关键接口及对应 Mock 桩：

```go
// 服务层 —— LLM 客户端
type LLMClient interface {
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error)
}
// Mock：pkg/testutil/mock_llm.go -> MockLLMClient

// 基础设施层 —— 文件系统
type FileSystem interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm os.FileMode) error
    Stat(path string) (os.FileInfo, error)
    // ...
}
// Mock：pkg/testutil/mock_fs.go -> MockFS

// 基础设施层 —— 时钟
type Clock interface {
    Now() time.Time
    Sleep(d time.Duration)
}
// Mock：pkg/testutil/testutil.go -> FakeClock

// 工具层 —— 工具接口
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, params json.RawMessage) (ToolResult, error)
}
// Mock：各工具测试文件中内联定义 MockTool
```

## 5. 验收流程

各模块开发完成后，QA 按以下流程执行验收：

1. **代码审查**：确认测试文件已随实现代码一并提交，无遗漏的公共函数测试。
2. **覆盖率检查**：运行 `go test -coverprofile=coverage.out ./...`，确认该模块覆盖率达到目标值。
3. **竞态检测**：运行 `go test -race ./...`，确认无 data race 报告。
4. **行为对比**：针对该模块涉及的关键行为，执行 Golden file 对比或协议录制回放测试，确认与 TS 版本行为一致。
5. **集成验证**：运行相关集成测试，确认跨模块交互正常。
6. **性能回归**：对性能敏感模块，运行 Benchmark 并与基线对比，确认无性能退化（超过 20% 视为退化）。
7. **验收结论**：在对应 Issue/PR 中更新测试结果，标记 `QA: approved` 或列出待修复项。

## 6. 风险与注意事项

### 6.1 TUI 层测试难点

- **问题**：Bubble Tea 的渲染输出依赖终端环境，自动化测试难以验证最终视觉效果。
- **应对**：
  - 优先测试 `Model` 的状态逻辑（`Update` 函数），不依赖终端渲染。
  - 对 `View` 函数输出使用 Golden file 对比纯字符串内容。
  - 使用 `tea.NewTestProgram` 或构造 mock `tea.Msg` 驱动状态变更。
  - 复杂交互场景（键盘输入、窗口 resize）优先通过手动测试补充。

### 6.2 外部 API 依赖

- **问题**：Anthropic API 调用涉及网络、计费，不适合在 CI 中真实请求。
- **应对**：
  - 单元测试和集成测试一律使用 `httptest.Server` 回放预录制响应。
  - 在专用的 E2E 测试阶段（手动触发）才真实调用 API，需设置 `ANTHROPIC_API_KEY` 环境变量。
  - 录制的响应文件存入 `testdata/vcr/`，纳入版本控制。

### 6.3 MCP 协议测试

- **问题**：MCP 协议为 JSON-RPC over stdio，测试时需要模拟完整的 stdio 交互。
- **应对**：实现轻量级 `MockMCPServer`，通过 `os.Pipe()` 模拟 stdio，覆盖握手、工具列举、工具调用全流程。

### 6.4 并发与竞态

- **问题**：Agent 主循环、流式输出处理存在并发访问，容易引入 data race。
- **应对**：所有测试必须通过 `-race` 检测；对并发逻辑优先编写压力测试（`-count=100`）。

### 6.5 测试数据污染

- **问题**：文件系统操作、全局状态修改可能导致测试间相互干扰。
- **应对**：
  - 文件操作统一使用 `t.TempDir()` 隔离。
  - 全局变量/单例在测试中通过依赖注入替换，避免直接修改。
  - 环境变量修改使用 `t.Setenv()`，自动还原。

### 6.6 TS 版本行为基准维护

- **问题**：TS 版本持续更新，Golden file 和行为基准可能过时。
- **应对**：建立定期同步机制，当 TS 版本有重大更新时，同步更新 Golden file 并触发对比测试重新验证。
