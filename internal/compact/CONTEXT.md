---
package: compact
import_path: internal/compact
layer: services
generated_at: 2026-04-29T02:31:52Z
source_files: [auto.go, compact.go, micro.go, snip.go]
---

# internal/compact

> Layer: **Services** · Files: 4 · Interfaces: 1 · Structs: 11 · Functions: 3

## Interfaces

### Compressor (2 methods)
> Compressor is the unified interface for context compaction.

```go
type Compressor interface {
    NeedsCompaction(messages []types.Message, model string, extra CompactionExtra) bool
    Compact(ctx context.Context, messages []types.Message, params CompactionParams) (*CompactionResult, error)
}
```

## Structs

- **AutoCompactTrackingState** — 4 fields: Compacted, TurnCounter, TurnID, ConsecutiveFailures
- **AutoCompactor** — 4 fields
- **CacheEdit** — 2 fields: ToolUseID, Summary
- **CompactionExtra** — 1 fields: SnipTokensFreed
- **CompactionParams** — 5 fields: SystemPromptParts, UserContext, SystemContext, QuerySource, ForkMessages
- **CompactionResult** — 7 fields: SummaryMessages, Attachments, HookResults, PreCompactTokenCount, PostCompactTokenCount, TruePostCompactTokenCount, CompactionUsage
- **MicroCompactResult** — 2 fields: Messages, CompactionInfo
- **MicroCompactionInfo** — 1 fields: PendingCacheEdits
- **MicroCompactor**
- **SnipResult** — 3 fields: Messages, TokensFreed, BoundaryMessage
- **TokenUsage** — 4 fields: InputTokens, OutputTokens, CacheReadInputTokens, CacheCreationInputTokens

## Functions

- `NewAutoCompactor(client api.Client, model string, maxTokens int) *AutoCompactor`
- `NewMicroCompactor() *MicroCompactor`
- `SnipCompactIfNeeded(messages []types.Message) SnipResult`

## Constants

- `AutoCompactBufferTokens`
- `AutoCompactWarningBufferTokens`
- `MaxConsecutiveFailures`
- `MaxOutputTokensForSummary`

## Change Impact

**Exported type references (files that use types from this package):**
- `AutoCompactor` → `internal/engine/engine.go`
- `CompactionExtra` → `internal/engine/query.go`
- `CompactionParams` → `internal/engine/query.go`
- `MicroCompactor` → `internal/engine/engine.go`

## Dependencies

**Imports:** `internal/api`, `pkg/types`

**Imported by:** `internal/engine`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
