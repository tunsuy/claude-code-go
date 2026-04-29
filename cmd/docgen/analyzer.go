package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PackageInfo holds extracted metadata for a single Go package.
type PackageInfo struct {
	// Name is the Go package name (e.g. "coordinator").
	Name string
	// ImportPath is relative to module root (e.g. "internal/coordinator").
	ImportPath string
	// Layer is the architecture layer (e.g. "core").
	Layer string
	// SourceFiles lists non-test .go files.
	SourceFiles []string
	// Interfaces lists exported interface types with their methods.
	Interfaces []InterfaceInfo
	// Structs lists exported struct types with field counts.
	Structs []StructInfo
	// FuncTypes lists exported function types (type X func(...)).
	FuncTypes []FuncTypeInfo
	// Functions lists exported package-level functions.
	Functions []FuncInfo
	// Constants lists exported constant names.
	Constants []string
	// Imports lists packages this package imports (relative paths).
	Imports []string
}

// InterfaceInfo describes an exported interface.
type InterfaceInfo struct {
	Name    string
	Methods []MethodInfo
	Doc     string
}

// MethodInfo describes a single method in an interface.
type MethodInfo struct {
	Name      string
	Signature string // e.g. "(ctx context.Context, req SpawnRequest) (AgentID, error)"
}

// StructInfo describes an exported struct.
type StructInfo struct {
	Name       string
	FieldCount int
	FieldNames []string // exported field names
	Doc        string
	Embeds     []string // embedded type names
}

// FuncTypeInfo describes an exported function type.
type FuncTypeInfo struct {
	Name      string
	Signature string
}

// FuncInfo describes an exported package-level function.
type FuncInfo struct {
	Name      string
	Signature string
	Doc       string
}

// analyzePackage parses all Go files in dir and extracts package metadata.
func analyzePackage(dir string, moduleRoot string) (*PackageInfo, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", dir, err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go packages found in %s", dir)
	}

	// Take the first (usually only) package.
	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
		break
	}

	relPath, _ := filepath.Rel(moduleRoot, dir)
	info := &PackageInfo{
		Name:       pkg.Name,
		ImportPath: filepath.ToSlash(relPath),
		Layer:      resolveLayer(filepath.ToSlash(relPath)),
	}

	// Collect source files.
	for fname := range pkg.Files {
		base := filepath.Base(fname)
		if !strings.HasSuffix(base, "_test.go") {
			info.SourceFiles = append(info.SourceFiles, base)
		}
	}
	sort.Strings(info.SourceFiles)

	importSet := make(map[string]bool)

	// Walk all files.
	for _, file := range pkg.Files {
		// Collect imports.
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(path, "claude-code-go/") {
				// Extract relative path.
				parts := strings.SplitN(path, "claude-code-go/", 2)
				if len(parts) == 2 {
					importSet[parts[1]] = true
				}
			}
		}

		// Walk declarations.
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				info.processGenDecl(d)
			case *ast.FuncDecl:
				info.processFuncDecl(d, fset)
			}
		}
	}

	// Convert import set to sorted list.
	for imp := range importSet {
		info.Imports = append(info.Imports, imp)
	}
	sort.Strings(info.Imports)

	// Sort all collections for stable output.
	sort.Slice(info.Interfaces, func(i, j int) bool { return info.Interfaces[i].Name < info.Interfaces[j].Name })
	sort.Slice(info.Structs, func(i, j int) bool { return info.Structs[i].Name < info.Structs[j].Name })
	sort.Slice(info.Functions, func(i, j int) bool { return info.Functions[i].Name < info.Functions[j].Name })
	sort.Slice(info.FuncTypes, func(i, j int) bool { return info.FuncTypes[i].Name < info.FuncTypes[j].Name })
	sort.Strings(info.Constants)

	return info, nil
}

// processGenDecl handles type, const, and var declarations.
func (info *PackageInfo) processGenDecl(d *ast.GenDecl) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			doc := extractDoc(d.Doc)
			switch t := s.Type.(type) {
			case *ast.InterfaceType:
				info.Interfaces = append(info.Interfaces, extractInterface(s.Name.Name, t, doc))
			case *ast.StructType:
				info.Structs = append(info.Structs, extractStruct(s.Name.Name, t, doc))
			case *ast.FuncType:
				info.FuncTypes = append(info.FuncTypes, FuncTypeInfo{
					Name:      s.Name.Name,
					Signature: formatFuncType(t),
				})
			}
		case *ast.ValueSpec:
			if d.Tok == token.CONST {
				for _, name := range s.Names {
					if name.IsExported() {
						info.Constants = append(info.Constants, name.Name)
					}
				}
			}
		}
	}
}

// processFuncDecl handles exported package-level functions (not methods).
func (info *PackageInfo) processFuncDecl(d *ast.FuncDecl, fset *token.FileSet) {
	if !d.Name.IsExported() || d.Recv != nil {
		return
	}
	sig := formatFuncSignature(d)
	doc := extractDoc(d.Doc)
	info.Functions = append(info.Functions, FuncInfo{
		Name:      d.Name.Name,
		Signature: sig,
		Doc:       doc,
	})
}

// extractInterface extracts method signatures from an interface type.
func extractInterface(name string, iface *ast.InterfaceType, doc string) InterfaceInfo {
	info := InterfaceInfo{Name: name, Doc: doc}
	if iface.Methods == nil {
		return info
	}
	for _, field := range iface.Methods.List {
		if len(field.Names) == 0 {
			continue // embedded interface
		}
		methodName := field.Names[0].Name
		sig := ""
		if ft, ok := field.Type.(*ast.FuncType); ok {
			sig = formatFuncType(ft)
		}
		info.Methods = append(info.Methods, MethodInfo{
			Name:      methodName,
			Signature: sig,
		})
	}
	return info
}

// extractStruct extracts field info from a struct type.
func extractStruct(name string, st *ast.StructType, doc string) StructInfo {
	info := StructInfo{Name: name, Doc: doc}
	if st.Fields == nil {
		return info
	}
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			// Embedded type.
			if ident := extractTypeName(field.Type); ident != "" {
				info.Embeds = append(info.Embeds, ident)
			}
			info.FieldCount++
			continue
		}
		for _, n := range field.Names {
			info.FieldCount++
			if n.IsExported() {
				info.FieldNames = append(info.FieldNames, n.Name)
			}
		}
	}
	return info
}

// extractDoc returns the first line of a doc comment, or "".
func extractDoc(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	text := cg.Text()
	if idx := strings.Index(text, "\n"); idx >= 0 {
		return strings.TrimSpace(text[:idx])
	}
	return strings.TrimSpace(text)
}

// extractTypeName returns the name of a type expression.
func extractTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	case *ast.StarExpr:
		return extractTypeName(t.X)
	}
	return ""
}

// formatFuncType returns a string like "(ctx context.Context, x int) (string, error)".
func formatFuncType(ft *ast.FuncType) string {
	var sb strings.Builder
	sb.WriteString("(")
	if ft.Params != nil {
		sb.WriteString(formatFieldList(ft.Params))
	}
	sb.WriteString(")")
	if ft.Results != nil && len(ft.Results.List) > 0 {
		sb.WriteString(" ")
		results := formatFieldList(ft.Results)
		if len(ft.Results.List) > 1 {
			sb.WriteString("(" + results + ")")
		} else {
			sb.WriteString(results)
		}
	}
	return sb.String()
}

// formatFieldList formats a parameter or result list.
func formatFieldList(fl *ast.FieldList) string {
	var parts []string
	for _, field := range fl.List {
		typeName := formatExpr(field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typeName)
		} else {
			for _, name := range field.Names {
				parts = append(parts, name.Name+" "+typeName)
			}
		}
	}
	return strings.Join(parts, ", ")
}

// formatExpr returns a readable string for a type expression.
func formatExpr(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return formatExpr(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + formatExpr(t.X)
	case *ast.ArrayType:
		return "[]" + formatExpr(t.Elt)
	case *ast.MapType:
		return "map[" + formatExpr(t.Key) + "]" + formatExpr(t.Value)
	case *ast.InterfaceType:
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return "any"
		}
		return "interface{...}"
	case *ast.ChanType:
		prefix := "chan "
		if t.Dir == ast.RECV {
			prefix = "<-chan "
		} else if t.Dir == ast.SEND {
			prefix = "chan<- "
		}
		return prefix + formatExpr(t.Value)
	case *ast.FuncType:
		return "func" + formatFuncType(t)
	case *ast.Ellipsis:
		return "..." + formatExpr(t.Elt)
	default:
		return "any"
	}
}

// formatFuncSignature returns "FuncName(params) results" for a FuncDecl.
func formatFuncSignature(d *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString(d.Name.Name)
	sb.WriteString(formatFuncType(d.Type))
	return sb.String()
}
