# Agent Output Protocol

> 版本：1.0
> 适用范围：所有 Claude Code Go 项目的 Agent 角色

## 规范目的

统一所有 Agent 完成任务后的输出格式，使 PM Agent 可以机器化解析结果，自动更新任务状态，而不依赖人工阅读散文。

## 强制格式

每个 Agent 完成任务后，**必须**在回复末尾附加如下 JSON 块。此 JSON 块必须是回复中的最后一个代码块。

```json
{
  "task_id": "T-001",
  "role": "agent-cli",
  "status": "completed",
  "artifacts": [
    "internal/bootstrap/wire.go",
    "internal/bootstrap/root.go"
  ],
  "test_results": "go test ./internal/bootstrap/... PASS 12/12",
  "coverage": "46.1%",
  "issues": [],
  "notes": "补充说明（可选）"
}
```

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `task_id` | string | ✅ | 任务注册表中的任务 ID（如 `T-001`） |
| `role` | string | ✅ | Agent 角色名（如 `agent-cli`、`qa`） |
| `status` | string | ✅ | 任务状态：`completed` \| `in_progress` \| `blocked` |
| `artifacts` | string[] | ✅ | 本次任务创建或修改的文件路径列表（相对于项目根目录） |
| `test_results` | string | ✅ | 测试命令输出摘要（如 `PASS 12/12` 或 `FAIL 2/12`） |
| `coverage` | string | ✅ | 受影响包的测试覆盖率（如 `46.1%`；如无法获取填 `"N/A"`） |
| `issues` | object[] | ✅ | 阻塞项或问题列表（无问题时填空数组 `[]`） |
| `notes` | string | ❌ | 补充说明（可选，如设计决策、已知限制） |

## issues 字段格式

当 `status` 为 `blocked` 或存在问题时，`issues` 数组中每项格式如下：

```json
{
  "severity": "blocking",
  "description": "pkg/types/message.go 接口签名已变更，需 Tech Lead 更新设计文档",
  "action_required": "tech-lead",
  "related_task": "T-005"
}
```

| 字段 | 说明 |
|------|------|
| `severity` | `blocking`（阻塞任务）\| `warning`（警告，不阻塞） |
| `description` | 问题描述 |
| `action_required` | 需要哪个角色处理（如 `pm`、`tech-lead`、`qa`） |
| `related_task` | 相关任务 ID（可选） |

## 状态语义

| status | 含义 | PM 处理方式 |
|--------|------|------------|
| `completed` | 任务完成，所有验收标准通过 | 更新 `task-registry.yaml` 中对应任务的 `status` 为 `completed` |
| `in_progress` | 任务进行中（部分完成，发送中间汇报） | 无需更新状态，等待后续 `completed` 汇报 |
| `blocked` | 任务被阻塞，无法继续 | 解析 `issues` 字段，协调处理阻塞项，更新状态为 `blocked` |

## 示例：正常完成

```json
{
  "task_id": "T-012",
  "role": "agent-tools",
  "status": "completed",
  "artifacts": [
    "internal/tools/fileops/file_read.go",
    "internal/tools/fileops/file_read_test.go"
  ],
  "test_results": "go test ./internal/tools/fileops/... PASS 18/18",
  "coverage": "73.2%",
  "issues": [],
  "notes": "FileRead 工具支持 PDF 和 Jupyter notebook 格式，通过 BaseTool 嵌入实现默认方法"
}
```

## 示例：任务阻塞

```json
{
  "task_id": "T-023",
  "role": "agent-core",
  "status": "blocked",
  "artifacts": [],
  "test_results": "N/A",
  "coverage": "N/A",
  "issues": [
    {
      "severity": "blocking",
      "description": "internal/api.Client 接口缺少 StreamMessages 方法，Engine 无法完成实现",
      "action_required": "agent-services",
      "related_task": "T-018"
    }
  ],
  "notes": "等待 T-018（API Client 实现）完成后可继续"
}
```

## 合规检查

PM Agent 在解析 Agent 输出时，若 JSON 块：
- 缺失：提示 Agent 补充输出，不更新任务状态
- 格式错误：提示 Agent 修正，记录问题
- `status` 为 `completed` 但 `test_results` 含 `FAIL`：标记为需要复查，不更新为 completed
