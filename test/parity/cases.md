# 行为对比用例总清单（cases.md）

> 本文件是**行为缺口台账**：每一条 `unimplemented` 都是一个待关闭的差距。
> 用例三档定义见 [README.md](README.md)。首批只收录**无 LLM 调用即可观察**的行为。

## 状态标记

- ✅ 已纳入 `parity_test.go` 断言
- ⏳ 已识别、尚未写断言（下批）
- 🚧 Go 侧为 `not yet implemented` 桩（与 debt 台账联动，共 42 处）
- 🧪 需要 LLM/网络，待录制回放机制

## A. 版本与帮助（无依赖路径）

| # | 用例 | oracle 行为 | Go 现状 | 状态 |
|---|------|------------|---------|------|
| A1 | `--version` / `-v` | 打印 `<semver> (Claude Code)`，exit 0 | 打印 `claude 0.1.0`——**格式不一致**（已实测 2026-08-30，oracle 2.1.251） | ❌ 差距已实测 |
| A2 | `--help` / `-h` | 列出 flags，exit 0 | 有帮助文本 | ✅ Tier-0 |
| A3 | `--help` 关键 flags（`-p`、`--model`、`--output-format`、`--resume`、`--dangerously-skip-permissions`） | 均出现 | 均出现 | ✅ Tier-1 |
| A4 | help 文本完整逐字对比 | — | 差异较大（flag 集合本身不全） | ⏳ 差距清单见 §D |
| A5 | `-p` 无 prompt | 报错、exit ≠ 0 | 报错 `no prompt provided` | ✅ Tier-0 |
| A6 | 未知子命令 | **exit 0**，把未知词当 prompt 交给 LLM 回答（实测 2026-08-30） | cobra 报错 exit 1 | ❌ 行为语义不一致（见下） |

## A7. 未知参数语义（2026-08-30 实测发现的行为差异）

oracle（TS 版）对**未知词**的处理不是"报错退出"，而是**当作 prompt** 走 LLM：

```
$ claude definitely-not-a-command
(模型回答："That's not a command — just text, so nothing ran...")  # exit 0
```

Go 版是标准 cobra 行为：`Error: unknown command ...` exit 1。

**决策待定**：这是"bug 还是对齐目标"？oracle 行为依赖 LLM（会消耗 token、
且输出不确定），Go 行为是传统 CLI 语义。**建议保持 Go 现状并在此行标注
`waived: 未知参数走 LLM 属于 oracle 的交互设计决策，重写版保持 cobra
传统语义更可预测 2026-08-30`**——但这是产品决策，需负责人确认。

同理 A1 版本格式：oracle 是 `2.1.251 (Claude Code)`，Go 是 `claude 0.1.0`。
建议 Go 侧改为 `<version> (Claude Code)` 格式对齐（小改动）。

## B. 子命令树

| # | 用例 | oracle 行为 | Go 现状 | 状态 |
|---|------|------------|---------|------|
| B1 | `mcp --help` | 存在，exit 0 | 存在 | ✅ Tier-0/1 |
| B2 | `mcp add/remove/list/get/add-json/add-from-claude-desktop/reset-project-choices` | 真实功能 | **8 个全部 `not yet implemented` 桩** | 🚧 |
| B3 | `mcp serve` | 启动 MCP server | 有实现（未对比） | ⏳ |
| B4 | `plugin --help` | 存在，exit 0 | 存在 | ✅ Tier-0/1 |
| B5 | `plugin list/install/uninstall/enable/disable/update/validate/marketplace` | 真实功能 | **8 个全部 `not yet implemented` 桩** | 🚧 |
| B6 | `auth login/logout/status` | OAuth 流程 | login/logout/status 已实现 | ⏳ 对比 login 的设备码输出格式 |
| B7 | `doctor` | 环境体检（逐项检查） | 打印最小报告（Go runtime/version），**无逐项检查** | ⏳ |
| B8 | `update` | 自更新；`update --check` 查版本 | `update` 桩；`--check` 半实现（打印当前版本 + "not yet implemented"） | 🚧 |
| B9 | `agents list/add/remove` | 管理 sub-agents | 桩 | 🚧 |
| B10 | `install` | 安装 gh action 等 | 桩 | 🚧 |
| B11 | oracle 有、Go 完全缺失的子命令 | 16 个顶层子命令 | Go 缺 **11 个顶层 + 9 个二级**（明细见 §B11 盘点，2026-08-30，oracle 2.1.251） | ✅ 已盘点 |

## B11. 子命令盘点（2026-08-30，oracle = claude 2.1.251 本机二进制）

**oracle 顶层子命令共 16 个**：`agents` `attach` `auth` `auto-mode` `doctor`
`gateway` `import` `install` `logs` `mcp` `project` `respawn` `rm`
`setup-token` `ultrareview`（+ `plugin`，见下）。

**Go 侧顶层**：`auth` `mcp` `plugin` `doctor` `update` `agents` `install`（7 个）。

### 顶层缺口（oracle 有、Go 无）— 11 个

| 子命令 | oracle 功能 | 优先级建议 |
|--------|------------|-----------|
| `attach <id>` | 打开后台会话 | 与 `--bg` 体系绑定，可延后 |
| `auto-mode` | 查看/重置 auto 权限模式配置 | 低（Go 侧无 auto mode 概念） |
| `gateway` | 企业网关 auth/telemetry | 低（企业场景） |
| `import [codex\|gemini]` | 从其他 AI CLI 导入配置 | 中（迁移场景实用） |
| `logs <id>` | 打印后台会话日志 | 与 `--bg` 体系绑定，可延后 |
| `project purge` | 删除项目级 Claude 状态 | 中（数据清理，实现简单） |
| `respawn <id\|--all>` | 重启后台会话 | 与 `--bg` 体系绑定，可延后 |
| `rm <id>` | 删除后台会话 | 与 `--bg` 体系绑定，可延后 |
| `setup-token` | 配置长效 token | 中 |
| `ultrareview` | 云端多 Agent 代码审查 | 低（依赖云端服务） |
| `update`（别名 `upgrade`） | Go 有 `update`，oracle 支持 `upgrade` 别名 | 补别名即可，微小 |

### 二级缺口（同名父命令下 oracle 有、Go 无）— 9 个

| 位置 | oracle 有 | Go 现状 |
|------|-----------|---------|
| `mcp` | `login <name>` / `logout <name>`（MCP OAuth） | 无 |
| `plugin` | `details <name>` | 无 |
| `plugin` | `eval`（插件 eval 套件） | 无 |
| `plugin` | `init\|new`（脚手架） | 无 |
| `plugin` | `prune\|autoremove` | 无 |
| `plugin` | `tag` | 无 |
| `plugin marketplace` | `add` / `list` / `remove\|rm` / `update` 整棵子树 | Go 只有 `marketplace` 桩（无子命令） |
| `agents` | oracle 无二级子命令（options 驱动） | Go 是 `list/add/remove` 三桩，**形态不一致** |
| `update` | 别名 `upgrade` | 无别名 |

### 盘点结论

- 缺口集中在两处：**后台会话体系**（`--bg`/`attach`/`logs`/`respawn`/`rm`，
  5 个命令同生共死，应作为一个 gap 整体决策——做或整体 waive）和
  **plugin 生态**（marketplace 子树 + details/eval/init/prune/tag）。
- `agents` 的形态差异（oracle 用 options，Go 用子命令桩）提示关闭 B9 时
  **不要照 Go 现状实现**，先对齐 oracle 形态。
- 加上 B2/B5 的 16 个桩：行为缺口合计 **11 顶层 + 9 二级 + 16 桩 = 36 处**。

## C. headless 输出契约

| # | 用例 | oracle 行为 | Go 现状 | 状态 |
|---|------|------------|---------|------|
| C1 | `-p --output-format json` 单轮 | 单个 JSON 对象，`type: "result"` | 已实现（见 test/integration） | 🧪 |
| C2 | `-p --output-format stream-json` | NDJSON 事件流 | 已实现 | 🧪 |
| C3 | 工具调用轮次的 stream-json 事件序 | 事件类型集合与顺序 | 部分 | 🧪 |
| C4 | `--max-turns` 达上限时的退出行为 | 明确退出码/结果类型 | 未对比 | 🧪 |

## D. 首要差距（从用例反推）

1. ~~**B11 子命令盘点**~~：✅ 已完成（2026-08-30，见 §B11），产出 11 个顶层 + 9 个二级缺口。
2. **B2/B5 的 16 个桩**：占全部 42 桩的 38%，且全部用户直接可见。建议 4 的「了断」（实现或摘除注册）从这里下手性价比最高。
3. **后台会话体系整体决策**（§B11 盘点结论）：`--bg`/`attach`/`logs`/`respawn`/`rm` 五件套，做或整体 waive，不留半吊子。
4. **A4 flag 集合差距**：Go 侧 root flags ≈35 个，oracle 更多（待精确盘点），差距本身即清单。

## 维护规则

- 关闭一条差距：更新本表状态 → 把用例从 skip/桩名单移入断言名单 → 若减少的是 `not yet implemented` 桩，同步 `scripts/debt-check.sh --update`。
- 放弃对齐某行为：在行内标注 `waived: <原因> <日期>`，不删除行——放弃记录与差距记录同样有价值。
