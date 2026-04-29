---
package: agent
import_path: internal/tools/agent
layer: tools
generated_at: 2026-04-29T02:31:52Z
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

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
