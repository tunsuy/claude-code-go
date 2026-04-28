---
package: agenttype
import_path: internal/agenttype
layer: infra
generated_at: 2026-04-28T11:59:48Z
source_files: [agenttype.go, builtins.go, loader.go, prompts.go, registry.go, tasktype.go]
---

# internal/agenttype

> Layer: **Infra** · Files: 6 · Interfaces: 0 · Structs: 3 · Functions: 3

## Structs

- **AgentProfile** — 9 fields: Type, DisplayName, Description, WhenToUse, SystemPrompt, ToolFilter, Model, MaxTurns, ...
- **Registry** — 3 fields
- **ToolFilter** — 2 fields: Mode, Tools

## Functions

- `LoadCustomAgents(dir string) ([]*AgentProfile, error)`
- `NewRegistry() *Registry`
- `RegisterBuiltins(r *Registry)`

## Constants

- `AgentTypeCustom`
- `AgentTypeExplore`
- `AgentTypeGuide`
- `AgentTypePlan`
- `AgentTypeVerify`
- `AgentTypeWorker`
- `TaskTypeInProcessTeammate`
- `TaskTypeLocalAgent`
- `TaskTypeLocalBash`
- `TaskTypeRemote`
- `ToolFilterAllowlist`
- `ToolFilterDenylist`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`

