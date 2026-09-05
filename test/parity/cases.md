# 行为对比用例总清单（cases.md）

> 本文件是**行为缺口台账**：每一条 `unimplemented` 都是一个待关闭的差距。
> 用例三档定义见 [README.md](README.md)。首批只收录**无 LLM 调用即可观察**的行为。

## 状态标记

- ✅ 已纳入 `parity_test.go` 断言
- ⏳ 已识别、尚未写断言（下批）
- 🚧 Go 侧为 `not yet implemented` 桩（与 debt 台账联动，B2/B5 落地后共 27 处，2026-09-05）
- 🧪 需要 LLM/网络，待录制回放机制

## A. 版本与帮助（无依赖路径）

| # | 用例 | oracle 行为 | Go 现状 | 状态 |
|---|------|------------|---------|------|
| A1 | `--version` / `-v` | 打印 `<semver> (Claude Code)`，exit 0 | 打印 `<semver> (Claude Code)`（2026-08-30 已对齐，原为 `claude 0.1.0`） | ✅ Tier-2 |
| A2 | `--help` / `-h` | 列出 flags，exit 0 | 有帮助文本 | ✅ Tier-0 |
| A3 | `--help` 关键 flags（`-p`、`--model`、`--output-format`、`--resume`、`--dangerously-skip-permissions`） | 均出现 | 均出现 | ✅ Tier-1 |
| A4 | help 文本完整逐字对比 | — | 差异较大（flag 集合本身不全） | ⏳ 差距清单见 §D |
| A5 | `-p` 无 prompt | 报错、exit ≠ 0 | 报错 `no prompt provided` | ✅ Tier-0 |
| A6 | 未知子命令 | **exit 0**，把未知词当 prompt 交给 LLM 回答（实测 2026-08-30） | cobra 报错 exit 1 | ⛔ `waived: 未知参数走 LLM 属于 oracle 的交互设计决策，重写版保持 cobra 传统语义更可预测 2026-08-30`（负责人确认） |

## A7. 未知参数语义（2026-08-30 实测发现的行为差异）——已 waive

oracle（TS 版）对**未知词**的处理不是"报错退出"，而是**当作 prompt** 走 LLM：

```
$ claude definitely-not-a-command
(模型回答："That's not a command — just text, so nothing ran...")  # exit 0
```

Go 版是标准 cobra 行为：`Error: unknown command ...` exit 1。

**决策（2026-08-30，负责人确认）**：**waive，保持 Go 现状**。理由：
oracle 行为依赖 LLM（每次敲错命令都消耗 token、输出不确定），cobra 的
传统 CLI 语义更可预测。落地方式：

- A6 行标注 `waived`（见上表），记录保留；
- parity 测试不再对齐两版（`unknown-subcommand` 用例从 Tier-0 移除）；
- 新增 `TestTargetUnknownSubcommandError`（无 oracle 也跑）**钉住 Go 侧
  契约**：未知子命令必须 exit ≠ 0 且输出含 unknown 诊断——防止未来
  有人"顺手对齐"把 waive 决策无声推翻。

同理 A1 版本格式：oracle 是 `2.1.251 (Claude Code)`，Go 是 `claude 0.1.0`。
**已对齐（2026-08-30）**：Go 侧 `--version` 改为 `<semver> (Claude Code)`
格式（`bootstrap.appVersionString()`），parity Tier-2 恢复通过。

## B. 子命令树

| # | 用例 | oracle 行为 | Go 现状 | 状态 |
|---|------|------------|---------|------|
| B1 | `mcp --help` | 存在，exit 0 | 存在 | ✅ Tier-0/1 |
| B2 | `mcp add/remove/list/get/add-json/add-from-claude-desktop/reset-project-choices` | 真实功能 | 已实现（7 个命令，字节级钉在 `internal/bootstrap/mcp_*_test.go`，对 oracle v2.1.261 captures） | ✅ 2026-09-05 |
| B3 | `mcp serve` | 启动 MCP server | 有实现（未对比） | ⏳ |
| B4 | `plugin --help` | 存在，exit 0 | 存在 | ✅ Tier-0/1 |
| B5 | `plugin list/install/uninstall/enable/disable/update/validate/marketplace` | 真实功能 | 已实现（8 个命令 + marketplace 子树，字节级钉在 `internal/bootstrap/plugin_*_test.go`） | ✅ 2026-09-05 |
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
| `plugin` | `details <name>` | 无 — `waived: B5 周期未实现，从帮助行与建议名中整体移除 2026-09-05` |
| `plugin` | `eval`（插件 eval 套件） | 无 — `waived: 同上 2026-09-05` |
| `plugin` | `init\|new`（脚手架） | 无 — `waived: 同上 2026-09-05` |
| `plugin` | `prune\|autoremove` | 无 — `waived: 同上（uninstall 留 .orphaned_at 标记，清理语义留给将来） 2026-09-05` |
| `plugin` | `tag` | 无 — `waived: 同上 2026-09-05` |
| `plugin marketplace` | `add` / `list` / `remove\|rm` / `update` 整棵子树 | 已实现（2026-09-05，B5 一并落地；远程源见 §B12 分歧） |
| `agents` | oracle 无二级子命令（options 驱动） | Go 是 `list/add/remove` 三桩，**形态不一致** |
| `update` | 别名 `upgrade` | 无别名 |

### 盘点结论

- 缺口集中在两处：**后台会话体系**（`--bg`/`attach`/`logs`/`respawn`/`rm`，
  5 个命令同生共死，应作为一个 gap 整体决策——做或整体 waive）和
  **plugin 生态**（marketplace 子树 + details/eval/init/prune/tag）。
- `agents` 的形态差异（oracle 用 options，Go 用子命令桩）提示关闭 B9 时
  **不要照 Go 现状实现**，先对齐 oracle 形态。
- 加上 B2/B5 的桩：行为缺口合计 11 顶层 + 9 二级 + 桩（2026-08-30 快照）。
- **2026-09-05 更新**：B2/B5 落地关闭 15 个桩（勘误：原记 16，B2 实为 7 个命令
  而非 8 个；42→27 与 debt 基线一致），marketplace 子树一并实现，
  `details/eval/init/prune/tag` 5 个二级缺口 waive（见上表）。
  剩余二级缺口：`mcp login/logout`、`agents` 形态、`update` 别名 = 3 处。

## B12. B2/B5 落地记录（2026-09-05，oracle = claude v2.1.261）

**范围**：`mcp` 7 命令 + `plugin` 8 命令 + `plugin marketplace` 子树，
全部字节级钉在 `internal/bootstrap/{mcp,plugin}_*_test.go`（测试内注释标注
对应 capture 编号）。断言放在单测而非 `parity_test.go`：这些命令需要
HOME/项目目录注入与固定时间戳，双二进制 parity 骨架给不了。

**已记录的分歧**（决策：诚实报错优于隐形桩）：

1. **远程 marketplace 源**（`owner/repo`、`https://…`、`npm:`、`--claudeai`）
   未实现 → 明确报错 "not supported by this build"，仅支持本地路径。
2. **`plugin details/eval/init/prune/tag`** 从帮助行与建议名中整体移除
   （未实现的命令不展示，避免用户踩桩）。
3. **JSON 解析错误措辞**：Go 与 JS 运行时措辞不同
   （`invalid character 'o' in literal null…` vs `Unexpected identifier "no"`），
   外层 `Invalid JSON syntax: JSON Parse error: …` 格式一致。文档化接受。
4. **hooks modules 文件**只做存在性检查，不按 JS 语义加载。
5. **`--sparse`/`--config`/`--keep-data`/`-y`** 接受但忽略
   （其门控的确认交互只在 TTY 下触发，本实现无此路径）。

**未验证边缘**（无 oracle capture，形状为合理推断，已作为回归测试钉住，
测试内有 `Unverified edge` 注释）：

- `plugin validate` 不存在路径 / 无 manifest 目录的错误形状；
- `marketplace remove --scope` 只删声明、保留 registry 记录；
- `marketplace update`（update-all）零市场时的失败文案；
- `marketplace add <file>` 的 `sourceType: "file"` 分类；
- hooks.json `hooks` 键非对象时的错误；
- `plugin disable --all` 跨 scope 的遍历顺序；
- `github.com/owner/repo`（无 scheme）→ 归入 Path does not exist 而非远程源。

## C. headless 输出契约

| # | 用例 | oracle 行为 | Go 现状 | 状态 |
|---|------|------------|---------|------|
| C1 | `-p --output-format json` 单轮 | 单个 JSON 对象，`type: "result"` | 已实现（见 test/integration） | 🧪 |
| C2 | `-p --output-format stream-json` | NDJSON 事件流 | 已实现 | 🧪 |
| C3 | 工具调用轮次的 stream-json 事件序 | 事件类型集合与顺序 | 部分 | 🧪 |
| C4 | `--max-turns` 达上限时的退出行为 | 明确退出码/结果类型 | 未对比 | 🧪 |

## D. 首要差距（从用例反推）

1. ~~**B11 子命令盘点**~~：✅ 已完成（2026-08-30，见 §B11），产出 11 个顶层 + 9 个二级缺口。
2. ~~**B2/B5 的 16 个桩**~~：✅ 已完成（2026-09-05，实为 15 桩，见 §B12）——
   全部实现并字节级钉测，42 桩 → 27 桩，debt 基线已随 PR 刷新。
3. **后台会话体系整体决策**（§B11 盘点结论）：`--bg`/`attach`/`logs`/`respawn`/`rm` 五件套，做或整体 waive，不留半吊子。
4. **A4 flag 集合差距**：Go 侧 root flags ≈35 个，oracle 更多（待精确盘点），差距本身即清单。

## 维护规则

- 关闭一条差距：更新本表状态 → 把用例从 skip/桩名单移入断言名单 → 若减少的是 `not yet implemented` 桩，同步 `scripts/debt-check.sh --update`。
- 放弃对齐某行为：在行内标注 `waived: <原因> <日期>`，不删除行——放弃记录与差距记录同样有价值。
