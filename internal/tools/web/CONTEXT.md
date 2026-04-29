---
package: web
import_path: internal/tools/web
layer: tools
generated_at: 2026-04-29T02:31:52Z
source_files: [doc.go, webfetch.go, websearch.go]
---

# internal/tools/web

> Layer: **Tools** · Files: 3 · Interfaces: 1 · Structs: 5 · Functions: 1

## Interfaces

### HTTPClient (1 methods)
> HTTPClient is the interface satisfied by *http.Client and any test double.

```go
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}
```

## Structs

- **WebFetchInput** — 2 fields: URL, Prompt
- **WebFetchOutput** — 3 fields: URL, Content, StatusCode
- **WebSearchInput** — 3 fields: Query, AllowedDomains, BlockedDomains
- **WebSearchOutput** — 2 fields: Query, Results
- **WebSearchResult** — 3 fields: Title, URL, Description

## Functions

- `ClearFetchCache()`

## Dependencies

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
