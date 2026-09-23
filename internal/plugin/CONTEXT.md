---
package: plugin
import_path: internal/plugin
layer: infra
generated_at: 2026-09-23T11:03:32Z
source_files: [plugin.go]
---

# internal/plugin

> Layer: **Infra** · Files: 1 · Interfaces: 0 · Structs: 1 · Functions: 2

## Structs

- **Manager** — 2 fields

## Functions

- `EnsureNoFatalErrors(errs []error) error`
- `NewManager(configs []types.PluginConfig) (*Manager, []error)`

## Dependencies

**Imports:** *(none — zero-dependency)*

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。
