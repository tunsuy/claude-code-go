---
package: config
import_path: internal/config
layer: infra
generated_at: 2026-09-05T09:11:12Z
source_files: [loader.go, mcpstore.go, orderedjson.go, pluginstore.go, settings.go, writer.go]
---

# internal/config

> Layer: **Infra** · Files: 6 · Interfaces: 1 · Structs: 18 · Functions: 19

## Interfaces

### ConfigLoader (1 methods)
> ConfigLoader is the interface for loading layered settings.

```go
type ConfigLoader interface {
    Load() (*LayeredSettings, error)
}
```

## Structs

- **AttributionConfig** — 2 fields: Commit, PR
- **EnvPair** — 2 fields: Key, Value
- **InstalledPluginRecord** — 6 fields: Scope, InstallPath, Version, InstalledAt, LastUpdated, ProjectPath
- **InstalledPlugins** — 2 fields
- **KnownMarketplace** — 4 fields: SourceType, SourcePath, InstallLocation, LastUpdated
- **KnownMarketplaces** — 1 fields
- **LayeredSettings** — 5 fields: User, Project, Local, Policy, Merged
- **Loader** — 2 fields
- **MCPServerEntry** — 2 fields: Name, Raw
- **MCPStore** — 2 fields
- **MarketplaceEntry** — 4 fields: Name, Source, Version, Description
- **MarketplaceManifest** — 6 fields: Name, Description, HasOwner, Plugins, RootDir, ManifestPath
- **OrderedMap** — 2 fields
- **PermissionsConfig** — 6 fields: Allow, Deny, Ask, DefaultMode, DisableBypass, AdditionalDirs
- **PluginManifest** — 4 fields: Name, Version, Description, HasAuthor
- **PluginStore** — 2 fields
- **SettingsJson** — 28 fields: Schema, APIKey, APIKeyHelper, BaseURL, Provider, AWSCredentialExport, AWSAuthRefresh, GCPAuthRefresh, ...
- **WorktreeConfig** — 2 fields: SymlinkDirectories, SparsePaths

## Functions

- `AddPermissionRule(projectDir string, list string, rule string) error`
- `ClaudeJsonPath(homeDir string) string`
- `DesktopConfigServers(homeDir string) ([]MCPServerEntry, error)`
- `LoadMarketplaceManifest(rootDir string) (*MarketplaceManifest, error)`
- `LoadMarketplaceManifestFile(path string) (*MarketplaceManifest, error)`
- `LoadOrderedJSON(path string) (*OrderedMap, error)`
- `LoadPluginManifest(dir string) (*PluginManifest, error)`
- `McpJsonPath(projectDir string) string`
- `NewLoader(homeDir string, projectDir string) *Loader`
- `NewMCPStore(homeDir string, projectDir string) *MCPStore`
- `NewOrderedMap() *OrderedMap`
- `NewPluginStore(homeDir string, projectDir string) *PluginStore`
- `OrphanCache(installPath string) error`
- `PluginNamePart(idOrName string) string`
- `PluginTimestamp(t time.Time) string`
- `ScopeLabel(scope string) string`
- `SettingsLocalPath(projectDir string) string`
- `ValidateMCPScope(scope string) error`
- `WriteOrderedJSONAtomic(path string, doc *OrderedMap) error`

## Constants

- `ClaudeDir`
- `ClaudeLocalDir`
- `MCPScopeLocal`
- `MCPScopeProject`
- `MCPScopeUser`
- `ManagedSettingsFile`
- `PermissionListAllow`
- `PermissionListAsk`
- `PermissionListDeny`
- `PluginScopeLocal`
- `PluginScopeProject`
- `PluginScopeUser`
- `SessionsDir`
- `SettingsFile`
- `SourceLocal`
- `SourcePolicy`
- `SourceProject`
- `SourceUser`
- `StatsFile`
- `TodosDir`

## Change Impact

**Exported type references (files that use types from this package):**
- `EnvPair` → `internal/bootstrap/mcp_health.go`, `internal/bootstrap/mcp_render.go`, `internal/bootstrap/mcp_render_test.go` (test), `internal/bootstrap/mcp_run.go`
- `InstalledPluginRecord` → `internal/bootstrap/plugin_install.go`, `internal/bootstrap/plugin_run.go`
- `InstalledPlugins` → `internal/bootstrap/plugin_run.go`
- `KnownMarketplace` → `internal/bootstrap/plugin_marketplace.go`
- `LayeredSettings` → `internal/bootstrap/wire.go`
- `MCPServerEntry` → `internal/bootstrap/mcp_health.go`, `internal/bootstrap/mcp_health_test.go` (test), `internal/bootstrap/mcp_list.go`, `internal/bootstrap/mcp_list_test.go` (test), `internal/bootstrap/mcp_render.go` + 2 more
- `MCPStore` → `internal/bootstrap/mcp_cmd_test.go` (test), `internal/bootstrap/mcp_list.go`, `internal/bootstrap/mcp_list_test.go` (test), `internal/bootstrap/mcp_render.go`, `internal/bootstrap/mcp_run.go`
- `MarketplaceEntry` → `internal/bootstrap/plugin_install.go`
- `MarketplaceManifest` → `internal/bootstrap/plugin_install.go`, `internal/bootstrap/plugin_marketplace.go`
- `OrderedMap` → `internal/bootstrap/mcp_health_test.go` (test), `internal/bootstrap/mcp_list.go`, `internal/bootstrap/mcp_list_test.go` (test), `internal/bootstrap/mcp_render_test.go` (test), `internal/bootstrap/mcp_run.go`
- `PluginStore` → `internal/bootstrap/plugin_cmd_test.go` (test), `internal/bootstrap/plugin_run.go`
- `SettingsJson` → `internal/state/store.go`

## Dependencies

**Imports:** `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/permissions`, `internal/state`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **MCPStore / PluginStore 独立于三级 settings 体系（2026-09-05，parity B2/B5）**：mcp 与
  plugin 的持久化位置不在 settings.json 三级体系内——mcp 服务器在 `~/.claude.json`
  （user 顶层 / projects.<cwd>. 两处），plugin 状态分散在 `~/.claude/plugins/` 下多文件
  （installed.json、known_marketplaces.json、cache/、settings 中的声明键）。且两者都要求
  "任意 home/project 目录可构造"（NewMCPStore/NewPluginStore 接收显式目录）以便测试注入
  t.TempDir()。若并入 Loader，会把专属文件布局耦合进通用配置层，也无法做纯文件系统单测。
- **OrderedMap：保序 JSON 对象（2026-09-05，B2/B5）**：Go map 迭代乱序，而 oracle（TS）
  的 JSON 对象保留插入顺序，且这是用户可见行为——`mcp add` 新服务器追加到文件末尾、
  `plugin list` 按注册顺序渲染。OrderedMap 以键切片 + map 实现双向保序 marshal/unmarshal，
  MCPStore 与 PluginStore 的全部对象字段（mcpServers、enabledPlugins、extraMarketplaces）
  都由它承载。
- **OrphanCache 只标记不删除（2026-09-05，B5）**：uninstall / marketplace remove 之后，
  缓存目录原样保留并写入 `.orphaned_at`（epoch 毫秒）标记，而非递归删除。为什么：与
  oracle 行为一致（其插件缓存同样留痕），且避免误删可能被其他 scope 引用的文件；实际
  清理留给将来显式的 prune 语义。
