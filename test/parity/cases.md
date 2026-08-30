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
| A1 | `--version` / `-v` | 打印 `claude <semver>`，exit 0 | 相同格式（版本号可不同） | ✅ Tier-2 |
| A2 | `--help` / `-h` | 列出 flags，exit 0 | 有帮助文本 | ✅ Tier-0 |
| A3 | `--help` 关键 flags（`-p`、`--model`、`--output-format`、`--resume`、`--dangerously-skip-permissions`） | 均出现 | 均出现 | ✅ Tier-1 |
| A4 | help 文本完整逐字对比 | — | 差异较大（flag 集合本身不全） | ⏳ 差距清单见 §D |
| A5 | `-p` 无 prompt | 报错、exit ≠ 0 | 报错 `no prompt provided` | ✅ Tier-0 |
| A6 | 未知子命令 | exit ≠ 0，stderr 提示 | cobra 默认错误 | ✅ Tier-0 |

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
| B11 | oracle 有、Go 完全缺失的子命令 | （待盘点） | **未盘点** | ⏳ 首要差距清单 |

## C. headless 输出契约

| # | 用例 | oracle 行为 | Go 现状 | 状态 |
|---|------|------------|---------|------|
| C1 | `-p --output-format json` 单轮 | 单个 JSON 对象，`type: "result"` | 已实现（见 test/integration） | 🧪 |
| C2 | `-p --output-format stream-json` | NDJSON 事件流 | 已实现 | 🧪 |
| C3 | 工具调用轮次的 stream-json 事件序 | 事件类型集合与顺序 | 部分 | 🧪 |
| C4 | `--max-turns` 达上限时的退出行为 | 明确退出码/结果类型 | 未对比 | 🧪 |

## D. 首要差距（从用例反推）

1. **B11 子命令盘点**：拿 oracle `--help` 输出与 Go 侧 diff，得到完整缺口清单——这是 cases.md 的第一优先补充项，产出后可直接转为 gap 文档。
2. **B2/B5 的 16 个桩**：占全部 42 桩的 38%，且全部用户直接可见。建议 4 的「了断」（实现或摘除注册）从这里下手性价比最高。
3. **A4 flag 集合差距**：Go 侧 root flags ≈35 个，oracle 更多（待精确盘点），差距本身即清单。

## 维护规则

- 关闭一条差距：更新本表状态 → 把用例从 skip/桩名单移入断言名单 → 若减少的是 `not yet implemented` 桩，同步 `scripts/debt-check.sh --update`。
- 放弃对齐某行为：在行内标注 `waived: <原因> <日期>`，不删除行——放弃记录与差距记录同样有价值。
