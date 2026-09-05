---
package: memdir
import_path: internal/memdir
layer: services
generated_at: 2026-09-05T09:11:12Z
source_files: [discover.go, dream.go, extract.go, extract_prompt.go, freshness.go, gate.go, include.go, loader.go, quick_memory.go, relevance.go, scope.go, secret_scanner.go, session_memory.go, status.go, store.go, types.go]
---

# internal/memdir

> Layer: **Services** · Files: 16 · Interfaces: 0 · Structs: 10 · Functions: 34

## Structs

- **AutoDreamConfig** — 2 fields: Store, Enabled
- **DiscoveredFile** — 2 fields: Path, Scope
- **ExtractMemoriesConfig** — 3 fields: Store, MaxTurns, Enabled
- **MemoryFile** — 3 fields: Header, Body, Path
- **MemoryHeader** — 6 fields: Title, Type, CreatedAt, UpdatedAt, Tags, Source
- **MemoryPathOptions** — 1 fields: AutoMemoryDirectory
- **MemoryStore** — 1 fields
- **RelevanceConfig** — 3 fields: MaxMemoriesPerTurn, MaxMemoryBytes, MaxSessionBytes
- **RelevantMemory** — 4 fields: Path, Title, Content, FreshnessNote
- **SessionMemoryConfig** — 2 fields: SessionID, HomeDir

## Functions

- `BuildExtractionPrompt(conversationSummary string) string`
- `DefaultAutoDreamConfig() AutoDreamConfig`
- `DefaultExtractConfig() ExtractMemoriesConfig`
- `DefaultRelevanceConfig() RelevanceConfig`
- `DiscoverAll(startDir string) ([]DiscoveredFile, error)`
- `DiscoverClaudeMd(startDir string) []string`
- `ExecuteAutoDream(ctx context.Context, hookCtx *engine.StopHookContext, cfg AutoDreamConfig)`
- `ExecuteExtractMemories(ctx context.Context, hookCtx *engine.StopHookContext, cfg ExtractMemoriesConfig)`
- `ExecuteSessionMemory(ctx context.Context, hookCtx *engine.StopHookContext, cfg SessionMemoryConfig)`
- `FormatFrontmatter(h MemoryHeader) string`
- `FormatMemoryFile(mf *MemoryFile) string`
- `FormatMemoryStatus(workingDir string, store *MemoryStore) string`
- `FormatRelevantMemoriesPrompt(memories []RelevantMemory) string`
- `IsAutoDreamEnabled(bareMode bool, settingsEnabled *bool) bool`
- `IsAutoMemoryEnabled(bareMode bool, settingsEnabled *bool) bool`
- `IsExtractMemoriesEnabled(bareMode bool, settingsEnabled *bool) bool`
- `LoadAllMemory(claudeMdPaths []string, store *MemoryStore) string`
- `LoadAndTruncate(paths []string, maxBytes int) string`
- `LoadMemoryPrompt(paths []string) string`
- `LoadScopedAllMemory(files []DiscoveredFile, store *MemoryStore) string`
- `LoadScopedMemoryPrompt(files []DiscoveredFile) string`
- `MemoryAge(updatedAt time.Time) string`
- `MemoryFreshnessText(updatedAt time.Time) string`
- `NewMemoryStore(projectDir string) (*MemoryStore, error)`
- `NewMemoryStoreWithOptions(projectDir string, opts MemoryPathOptions) (*MemoryStore, error)`
- `NewMemoryStoreWithPath(memoryDir string) *MemoryStore`
- `ParseMemoryFile(content string, filePath string) (*MemoryFile, error)`
- `ProcessIncludes(content string, basePath string, depth int) (string, error)`
- `RedactSecrets(content string) string`
- `RunManualDream(ctx context.Context, eng engine.QueryEngine, cache *engine.CacheSafeParams, store *MemoryStore) error`
- `SaveQuickMemory(store *MemoryStore, text string) (string, error)`
- `SaveSessionMemory(messages []types.Message, summary string, cfg SessionMemoryConfig) (string, error)`
- `ScanSecrets(content string) error`
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
- `MemoryPathOptions` → `internal/bootstrap/wire.go`
- `MemoryStore` → `internal/bootstrap/wire.go`, `internal/tools/memory/memory.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `SessionMemoryConfig` → `internal/tui/keys.go`

## Dependencies

**Imports:** `internal/engine`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/commands`, `internal/tools/memory`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
