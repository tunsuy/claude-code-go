---
package: agent
import_path: internal/tools/agent
layer: tools
generated_at: 2026-09-23T11:03:32Z
source_files: [agent.go, doc.go, getagentstatus.go, sendmessage.go]
---

# internal/tools/agent

> Layer: **Tools** · Files: 4 · Interfaces: 0 · Structs: 6 · Functions: 0

## Structs

- **AgentInput** — 8 fields: Prompt, SystemPrompt, AllowedTools, MaxTurns, AgentType, Background, AgentName, Model
- **AgentOutput** — 2 fields: Response, AgentID
- **GetAgentStatusInput** — 1 fields: AgentID
- **GetAgentStatusOutput** — 3 fields: AgentID, Status, Result
- **SendMessageInput** — 3 fields: To, AgentID, Content
- **SendMessageOutput** — 1 fields: Response

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。
