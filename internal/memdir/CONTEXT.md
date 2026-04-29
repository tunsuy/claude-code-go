---
package: memdir
import_path: internal/memdir
layer: services
generated_at: 2026-04-29T02:31:52Z
source_files: [discover.go, extract.go, extract_prompt.go, freshness.go, include.go, loader.go, relevance.go, scope.go, store.go, types.go]
---

# internal/memdir

> Layer: **Services** · Files: 10 · Interfaces: 0 · Structs: 7 · Functions: 20

## Structs

- **DiscoveredFile** — 2 fields: Path, Scope
- **ExtractMemoriesConfig** — 3 fields: Store, MaxTurns, Enabled
- **MemoryFile** — 3 fields: Header, Body, Path
- **MemoryHeader** — 6 fields: Title, Type, CreatedAt, UpdatedAt, Tags, Source
- **MemoryStore** — 1 fields
- **RelevanceConfig** — 3 fields: MaxMemoriesPerTurn, MaxMemoryBytes, MaxSessionBytes
- **RelevantMemory** — 4 fields: Path, Title, Content, FreshnessNote

## Functions

- `BuildExtractionPrompt(conversationSummary string) string`
- `DefaultExtractConfig() ExtractMemoriesConfig`
- `DefaultRelevanceConfig() RelevanceConfig`
- `DiscoverAll(startDir string) ([]DiscoveredFile, error)`
- `DiscoverClaudeMd(startDir string) []string`
- `ExecuteExtractMemories(ctx context.Context, hookCtx *engine.StopHookContext, cfg ExtractMemoriesConfig)`
- `FormatFrontmatter(h MemoryHeader) string`
- `FormatMemoryFile(mf *MemoryFile) string`
- `FormatRelevantMemoriesPrompt(memories []RelevantMemory) string`
- `LoadAllMemory(claudeMdPaths []string, store *MemoryStore) string`
- `LoadAndTruncate(paths []string, maxBytes int) string`
- `LoadMemoryPrompt(paths []string) string`
- `LoadScopedMemoryPrompt(files []DiscoveredFile) string`
- `MemoryAge(updatedAt time.Time) string`
- `MemoryFreshnessText(updatedAt time.Time) string`
- `NewMemoryStore(projectDir string) (*MemoryStore, error)`
- `NewMemoryStoreWithPath(memoryDir string) *MemoryStore`
- `ParseMemoryFile(content string, filePath string) (*MemoryFile, error)`
- `ProcessIncludes(content string, basePath string, depth int) (string, error)`
- `SurfaceRelevantMemories(store *MemoryStore, userMessage string, alreadySurfaced map[string]bool, sessionBytesUsed int, cfg RelevanceConfig) ([]RelevantMemory, error)`

## Constants

- `DefaultMemoryBase`
- `MaxIncludeDepth`
- `MaxMemoryIndexBytes`
- `MaxMemoryIndexLines`
- `MemoryFileName`
- `MemoryTypeFeedback`
- `MemoryTypeProject`
- `MemoryTypeReference`
- `MemoryTypeUser`
- `ScopeLocal`
- `ScopeManaged`
- `ScopeProject`
- `ScopeUser`

## Change Impact

**Exported type references (files that use types from this package):**
- `DiscoveredFile` → `internal/tui/messages.go`
- `MemoryFile` → `internal/tools/memory/memory.go`
- `MemoryHeader` → `internal/tools/memory/memory.go`
- `MemoryStore` → `internal/tools/memory/memory.go`, `internal/tui/model.go`

## Dependencies

**Imports:** `internal/engine`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/tools/memory`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
