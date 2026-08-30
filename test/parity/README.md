# test/parity — 行为对比测试（原版 TS 为 oracle）

> 复盘建议 3 的落地骨架（docs/project/discussions/2026-08-29-process-retrospective.md）。
> 目的：把「重写是否成立」的判定从内部指标（覆盖率、自测通过）换成**外部锚点**——
> 与原版 Claude Code（TypeScript 发行版）的 CLI 行为逐一对比。

## 设计原则

1. **黑盒对比**：只看两版二进制对同一命令的 stdout / stderr / exit code，不看实现。
2. **不依赖网络与 API key**：首批用例全部选择无 LLM 调用即可观察行为的路径
   （--version、--help、子命令树、错误信息）。涉及 LLM 的行为后续用录制回放再做。
3. **oracle 通过环境变量注入**：`CCG_PARITY_ORACLE` 指向原版可执行文件。
   未设置时测试**跳过（SKIP）而非失败**——CI 无 oracle 也能绿，本地/有 oracle 的
   环境显式开启后执行对比。
4. **被测方通过 `CCG_PARITY_TARGET` 注入**：默认 `bin/claude`（`make build` 产物）。
5. **断言分三档**，从宽到严，避免把骨架变成一上来就全红的负担：
   - `Tier-0 同形`：两版对同一命令的 exit code 一致；
   - `Tier-1 语义`：关键 token（子命令名、flag 名）在两版输出中都出现；
   - `Tier-2 逐字`：输出完全一致（仅对稳定、无版本号路径启用，如 `--version` 的
     格式 `claude <semver>`——注意版本号本身允许不同）。
6. **差异即文档**：失败的用例不是“待修的测试”，而是**待关闭的行为缺口**——
   修 Go 侧实现去对齐，而不是改断言迁就差异（除非经人工确认原版行为不值得对齐，
   此时在 cases.md 该用例旁标注 `waived: <原因>` 并保留记录）。

## 运行方式

```bash
make parity                                   # build + go test ./test/parity/...
export CCG_PARITY_ORACLE=/path/to/claude-ts  # 原版 TS 可执行文件
make parity                                   # 未设 oracle 时对比类用例 SKIP

# 只跑某一档：
go test ./test/parity/... -v -run 'TestParityTier0'
go test ./test/parity/... -v -run 'TestParityTier1'
```

## 目录结构

```
test/parity/
├── README.md          # 本文件
├── cases.md           # 用例总清单（含未实现部分，作为行为缺口台账）
├── doc.go             # 包文档（oracle/target 注入方式、skip 语义）
├── helpers_test.go    # 骨架：二进制执行、输出归一化、requireBoth/requireTarget
└── parity_test.go     # 首批用例：三档断言 + 离线子命令树检查
```

## 与欠账红线的关系

`cases.md` 中标记 `unimplemented` 的条目就是行为缺口台账；关闭一条即在
`cases.md` 打勾并把用例从 skip 名单移入断言名单。`make debt-check` 保证
not-yet-implemented 桩只降不升，parity 用例保证**降下来的桩对齐的是原版行为**，
两者互补：前者管“有没有”，后者管“对不对”。

## 已知限制

- oracle 二进制不在仓库内，也不在 CI 内（体积与许可原因），由环境提供；
  CI 中常规 `go test ./...` 只编译本包，不含 oracle 时全部 SKIP，不会红；
- TUI 交互行为（alt-screen、键位）暂不对比，首批只覆盖非交互路径；
- 版本号、绝对路径、时间戳等不稳定输出由 `normalizeOutput` 归一化后比较；
- **本机未安装 Go 工具链时无法本地验证编译**——本目录代码需由 CI 首跑验证
  （2026-08-29 记录：开发机 `go` 不在 PATH）。
