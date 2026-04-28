---
package: main
import_path: cmd/docgen
layer: unknown
generated_at: 2026-04-28T11:59:48Z
source_files: [analyzer.go, dependency.go, impact.go, layer.go, main.go, renderer.go]
---

# cmd/docgen

> Layer: **unknown** · Files: 6 · Interfaces: 0 · Structs: 10 · Functions: 0

## Structs

- **DependencyGraph** — 2 fields: Forward, Reverse
- **FuncInfo** — 3 fields: Name, Signature, Doc
- **FuncTypeInfo** — 2 fields: Name, Signature
- **InterfaceImpact** — 5 fields: InterfaceName, Package, Implementors, Mocks, Adapters
- **InterfaceInfo** — 3 fields: Name, Methods, Doc
- **Layer** — 3 fields: Name, Label, Order
- **MethodInfo** — 2 fields: Name, Signature
- **MockInfo** — 3 fields: File, TypeName, Implements
- **PackageInfo** — 10 fields: Name, ImportPath, Layer, SourceFiles, Interfaces, Structs, FuncTypes, Functions, ...
- **StructInfo** — 5 fields: Name, FieldCount, FieldNames, Doc, Embeds

## Dependencies

**Imports:** *(none — zero-dependency)*

