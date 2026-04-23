# Permission System Enhancement - GitHub Issues

本文档提供创建 GitHub Issues 时可直接复制使用的内容模板。

---

## Epic Issue（主跟踪 Issue）

### 标题
```
[Epic] Permission System Enhancement - Align with TypeScript Implementation
```

### 标签
```
enhancement, security, epic, priority:high
```

### 正文

```markdown
## 📋 概述

本 Epic 跟踪 claude-code-go 权限系统与 TypeScript 原版对齐的整体进度。

基于 [权限系统差异分析报告](docs/analysis/permission-system-gap-analysis.md) 识别的差距，
规划分阶段实现缺失功能。

## 🎯 目标

将权限系统功能覆盖率从当前的 **~40-50%** 提升到 **90%+**。

## 📊 当前状态 vs 目标状态

| 维度 | 当前状态 | 目标状态 |
|------|---------|---------|
| 权限决策管道 | ⚠️ 80% | ✅ 100% |
| AI 分类器（Auto mode） | ❌ 0% | ⚠️ 50%（基础框架） |
| Bash 安全分析 | ❌ 0% | ✅ 80% |
| 权限持久化 | ❌ 20% | ✅ 100% |
| 文件系统保护 | ❌ 0% | ✅ 100% |
| Shell 规则匹配 | ⚠️ 30% | ✅ 90% |
| 降级机制 | ⚠️ 40% | ✅ 100% |

## 📝 子任务清单

### 🔴 P0 — 关键安全功能（v0.2.0）

- [ ] #__ - 实现危险文件/目录保护列表
- [ ] #__ - 实现 Bash 命令基础安全检查
- [ ] #__ - 实现权限持久化（Always Allow 生效）

### 🟡 P1 — 功能完整性（v0.2.0 - v0.3.0）

- [ ] #__ - 实现拒绝降级机制
- [ ] #__ - 完善 Shell 规则通配符匹配
- [ ] #__ - 实现 PermissionRequest Hook

### 🟢 P2 — 高级功能（v0.4.0+）

- [ ] #__ - YOLO/Auto 分类器基础框架
- [ ] #__ - 沙箱执行机制
- [ ] #__ - 规则冲突检测
- [ ] #__ - MCP 工具命名空间完善

## 📚 相关文档

- [权限系统设计分析](docs/analysis/tool-permission-design.md) - 原版权限系统深度分析
- [差异分析报告](docs/analysis/permission-system-gap-analysis.md) - Go vs TS 实现差异
- [项目 Roadmap](docs/ROADMAP.md) - 整体版本规划

## 🔗 相关代码

- `internal/permissions/` - 权限检查核心逻辑
- `internal/tui/permissions.go` - 权限交互 UI
- `pkg/types/permissions.go` - 权限类型定义
- `internal/tools/shell/` - Shell 工具（需要安全增强）
- `internal/tools/fileops/` - 文件操作工具（需要路径保护）

## ✅ 完成标准

- [ ] 所有 P0 任务完成
- [ ] 危险文件写入会触发额外确认
- [ ] `sudo`/`rm -rf` 等危险命令会被标记
- [ ] "Always Allow" 选项能正确持久化
- [ ] 连续拒绝 3 次后自动降级
- [ ] 通过安全审计测试用例
```

---

## P0 子 Issues

### Issue 1: 危险文件/目录保护

**标题**
```
[Security][P0] Implement dangerous files/directories protection
```

**标签**
```
security, enhancement, priority:critical, good first issue
```

**正文**
```markdown
## 📋 概述

实现危险文件和目录的保护机制，防止 AI 助手意外修改敏感系统配置。

## 🎯 目标

- 定义危险文件列表（如 `.bashrc`, `.gitconfig` 等）
- 定义危险目录列表（如 `.git`, `.ssh` 等）
- 在文件操作工具中集成路径检查
- 对危险路径的操作触发额外确认

## 📝 实现要点

### 危险文件列表
```go
var DangerousFiles = []string{
    ".gitconfig", ".gitmodules",
    ".bashrc", ".bash_profile", ".zshrc", ".zprofile", ".profile",
    ".ripgreprc", ".mcp.json", ".claude.json",
    "settings.json", // Claude 配置
}
```

### 危险目录列表
```go
var DangerousDirectories = []string{
    ".git", ".vscode", ".idea", ".claude",
    ".ssh", ".gnupg",
}
```

### 需要修改的文件
- `pkg/types/security.go`（新增）或 `internal/permissions/filesystem.go`（新增）
- `internal/tools/fileops/file_write.go`
- `internal/tools/fileops/file_edit.go`

## 📚 参考

- 原版实现：`src/utils/permissions/filesystem.ts`
- 差异分析：`docs/analysis/permission-system-gap-analysis.md#24`

## ✅ 完成标准

- [ ] 定义完整的危险文件/目录列表
- [ ] `IsDangerousPath()` 函数实现
- [ ] FileWrite/FileEdit 工具集成路径检查
- [ ] 单元测试覆盖
- [ ] 文档更新

## ⏱️ 预估工作量

0.5 天
```

---

### Issue 2: Bash 命令安全检查

**标题**
```
[Security][P0] Implement Bash command safety analysis
```

**标签**
```
security, enhancement, priority:critical
```

**正文**
```markdown
## 📋 概述

实现 Bash 命令的基础安全分析，识别危险命令模式并提供风险提示。

## 🎯 目标

- 定义危险命令模式列表
- 实现命令风险评估函数
- 在 BashTool 中集成安全检查
- 高风险命令触发额外确认或阻止

## 📝 实现要点

### 危险模式列表
```go
var DangerousPatterns = []regexp.Regexp{
    regexp.MustCompile(`^sudo\s`),           // sudo 命令
    regexp.MustCompile(`^su\s`),             // su 命令
    regexp.MustCompile(`\brm\s+-rf\s+/`),    // rm -rf /
    regexp.MustCompile(`\beval\s`),          // eval 命令
    regexp.MustCompile(`>\s*/etc/`),         // 写入 /etc
    regexp.MustCompile(`\bchmod\s+777\s`),   // 危险权限
    regexp.MustCompile(`\bdd\s+if=`),        // dd 命令
    regexp.MustCompile(`\bpython\s+-c\s`),   // python 执行
    regexp.MustCompile(`\|.*\bsh\b`),        // 管道到 shell
}
```

### API 设计
```go
type RiskLevel int

const (
    RiskLow RiskLevel = iota
    RiskMedium
    RiskHigh
    RiskCritical
)

type CommandAnalysis struct {
    RiskLevel   RiskLevel
    Reasons     []string
    Suggestions []string
}

func AnalyzeCommand(cmd string) CommandAnalysis
```

### 需要修改的文件
- `internal/tools/shell/security.go`（新增）
- `internal/tools/shell/bash.go`

## 📚 参考

- 原版实现：`src/tools/BashTool/bashPermissions.ts`
- 原版 AST 分析：`src/utils/bash/ast.js`
- 差异分析：`docs/analysis/permission-system-gap-analysis.md#23`

## ✅ 完成标准

- [ ] 危险模式正则表达式定义
- [ ] `AnalyzeCommand()` 函数实现
- [ ] BashTool.CheckPermissions() 集成
- [ ] 单元测试覆盖常见危险命令
- [ ] 文档更新

## ⏱️ 预估工作量

2 天

## 💡 注意事项

- 这是基础实现，不包含完整的 AST 解析
- 后续可考虑集成 `mvdan.cc/sh` 库进行更精确的解析
```

---

### Issue 3: 权限持久化

**标题**
```
[Security][P0] Implement permission persistence (Always Allow)
```

**标签**
```
security, enhancement, priority:critical
```

**正文**
```markdown
## 📋 概述

实现权限规则的持久化机制，让用户选择 "Always Allow" 后能正确保存到配置文件。

## 🎯 目标

- 用户选择 "Always Allow" 后持久化到 `settings.json`
- 支持会话级和永久级的规则保存
- 实现规则的动态更新（不需重启）

## 📝 实现要点

### 持久化流程
```
用户选择 "Always Allow"
    ↓
TUI 发送 PermissionResponse{Choice: AlwaysAllow, ...}
    ↓
Checker.PersistPermission(rule, scope)
    ↓
读取 settings.json → 合并规则 → 写回文件
    ↓
更新内存中的规则缓存
```

### API 设计
```go
// internal/permissions/persist.go

type PermissionPersister interface {
    PersistRule(rule PermissionRule, scope PermissionRuleSource) error
    LoadRules() ([]PermissionRule, error)
}

func (c *PermissionsChecker) HandleAlwaysAllow(
    toolName string,
    input tools.Input,
    scope PermissionRuleSource,
) error
```

### 需要修改的文件
- `internal/permissions/persist.go`（新增）
- `internal/permissions/checker.go`
- `internal/tui/permissions.go`
- `internal/config/settings.go`

## 📚 参考

- 原版实现：`src/utils/permissions/PermissionContext.ts#handleUserAllow`
- 差异分析：`docs/analysis/permission-system-gap-analysis.md#25`

## ✅ 完成标准

- [ ] `PersistPermission()` 函数实现
- [ ] settings.json 规则合并逻辑
- [ ] 内存缓存更新机制
- [ ] TUI 层与 Checker 集成
- [ ] 单元测试覆盖
- [ ] 文档更新

## ⏱️ 预估工作量

1 天
```

---

## P1 子 Issues

### Issue 4: 拒绝降级机制

**标题**
```
[Security][P1] Implement denial fallback mechanism
```

**标签**
```
security, enhancement, priority:high
```

**正文**
```markdown
## 📋 概述

实现权限拒绝的追踪和自动降级机制，防止 AI 在自动模式下无限重试被拒绝的操作。

## 🎯 目标

- 追踪连续拒绝次数和总拒绝次数
- 超过阈值后自动降级到交互模式
- 提供用户可配置的阈值

## 📝 实现要点

### 阈值配置
```go
var DenialLimits = struct {
    MaxConsecutive int
    MaxTotal       int
}{
    MaxConsecutive: 3,   // 连续拒绝 3 次
    MaxTotal:       20,  // 总共拒绝 20 次
}
```

### API 设计
```go
// internal/permissions/denial.go

func (d *DenialTrackingState) RecordDenial(toolName string, reason string)
func (d *DenialTrackingState) ShouldFallbackToPrompting() bool
func (d *DenialTrackingState) Reset()
```

### 需要修改的文件
- `internal/permissions/denial.go`
- `internal/permissions/checker.go`

## 📚 参考

- 原版实现：`src/utils/permissions/denialTracking.ts`
- 差异分析：`docs/analysis/permission-system-gap-analysis.md#22`

## ✅ 完成标准

- [ ] `ShouldFallbackToPrompting()` 实现
- [ ] Checker 集成降级判断
- [ ] 配置文件支持自定义阈值
- [ ] 单元测试覆盖

## ⏱️ 预估工作量

0.5 天
```

---

### Issue 5: Shell 规则通配符匹配

**标题**
```
[Security][P1] Implement shell rule wildcard matching
```

**标签**
```
security, enhancement, priority:high
```

**正文**
```markdown
## 📋 概述

完善 Shell 命令的规则匹配引擎，支持通配符和前缀匹配。

## 🎯 目标

- 支持 `git *` 风格的通配符（匹配所有 git 子命令）
- 支持 `npm test` 精确匹配
- 支持 `prefix:*` 风格的前缀匹配
- 支持基本的 glob 语法

## 📝 实现要点

### 匹配规则示例
```
git *           → 匹配 git status, git commit, git push 等
npm test        → 只匹配 npm test
npm run *       → 匹配 npm run build, npm run dev 等
python *.py     → 匹配 python script.py, python test.py 等
```

### API 设计
```go
// internal/permissions/shellmatch.go

type ShellRuleMatcher struct {
    // ...
}

func (m *ShellRuleMatcher) Match(rule string, command string) bool
func (m *ShellRuleMatcher) ExtractCommandPrefix(command string) string
```

### 需要修改的文件
- `internal/permissions/shellmatch.go`（新增）
- `internal/permissions/checker.go`

## 📚 参考

- 原版实现：`src/utils/permissions/shellRuleMatching.ts`
- 差异分析：`docs/analysis/permission-system-gap-analysis.md#27`

## ✅ 完成标准

- [ ] 通配符匹配实现
- [ ] 前缀匹配实现
- [ ] Checker 集成
- [ ] 单元测试覆盖各种模式

## ⏱️ 预估工作量

1 天
```

---

## 使用说明

1. **创建 Epic Issue**
   - 复制上述 Epic Issue 内容
   - 在 GitHub 仓库创建新 Issue
   - 记录 Issue 编号

2. **创建子 Issues**
   - 按优先级依次创建 P0、P1 子 Issues
   - 在每个子 Issue 中引用 Epic Issue 编号
   - 在 Epic Issue 的子任务清单中更新子 Issue 编号

3. **Project Board（可选）**
   - 创建 GitHub Project 用于看板管理
   - 将 Epic 和子 Issues 添加到 Project
   - 使用列：`Backlog` → `In Progress` → `Review` → `Done`

4. **Milestone 关联**
   - 将 P0 Issues 关联到 `v0.2.0` milestone
   - 将 P1 Issues 关联到 `v0.2.0` 或 `v0.3.0` milestone
   - 将 P2 Issues 关联到 `v0.4.0` milestone

---

## 标签说明

建议在仓库中创建以下标签（如果不存在）：

| 标签 | 颜色 | 说明 |
|------|------|------|
| `security` | `#d73a4a` | 安全相关 |
| `epic` | `#3E4B9E` | 史诗级任务 |
| `priority:critical` | `#B60205` | P0 优先级 |
| `priority:high` | `#D93F0B` | P1 优先级 |
| `priority:medium` | `#FBCA04` | P2 优先级 |
| `good first issue` | `#7057ff` | 适合新手 |
