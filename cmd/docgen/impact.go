package main

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MockInfo describes a mock type found in a test file.
type MockInfo struct {
	// File is the path relative to module root.
	File string
	// TypeName is the mock struct name.
	TypeName string
}

// TypeReference records where an exported type is referenced outside its package.
type TypeReference struct {
	// File is relative to module root (e.g. "internal/bootstrap/wire.go").
	File string
	// Package is the import path of the file's package.
	Package string
	// IsTest indicates the reference is in a test file.
	IsTest bool
}

// PackageImpact holds all change-impact data for a single package.
type PackageImpact struct {
	// AdapterFiles are files named adapter.go / adapter_*.go in the package.
	AdapterFiles []string
	// Mocks found in test files that likely implement interfaces from this package.
	Mocks []MockInfo
	// TypeRefs maps exported type name → list of files that reference it.
	TypeRefs map[string][]TypeReference
}

// analyzePackageImpact builds change impact data for a single package.
func analyzePackageImpact(pkg *PackageInfo, allPackages []*PackageInfo, moduleRoot string) *PackageImpact {
	impact := &PackageImpact{
		TypeRefs: make(map[string][]TypeReference),
	}

	// Find adapter files in this package.
	pkgDir := filepath.Join(moduleRoot, filepath.FromSlash(pkg.ImportPath))
	entries, _ := os.ReadDir(pkgDir)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "adapter") && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			impact.AdapterFiles = append(impact.AdapterFiles, name)
		}
	}

	// Collect all exported type names from this package.
	exportedNames := make(map[string]bool)
	for _, iface := range pkg.Interfaces {
		exportedNames[iface.Name] = true
	}
	for _, st := range pkg.Structs {
		exportedNames[st.Name] = true
	}
	for _, ft := range pkg.FuncTypes {
		exportedNames[ft.Name] = true
	}

	if len(exportedNames) == 0 {
		return impact
	}

	// Find mocks in test files across the project.
	mocks := findMocks(moduleRoot)
	for _, mock := range mocks {
		for name := range exportedNames {
			if matchesMock(mock, name) {
				impact.Mocks = append(impact.Mocks, mock)
			}
		}
	}

	// Scan other packages for references to this package's exported types.
	// We do a simple text search: look for "<pkgname>.<TypeName>" patterns.
	pkgShortName := pkg.Name // e.g. "coordinator", "tools"
	for _, other := range allPackages {
		if other.ImportPath == pkg.ImportPath {
			continue
		}
		// Check if this package imports our package.
		imports := false
		for _, imp := range other.Imports {
			if imp == pkg.ImportPath {
				imports = true
				break
			}
		}
		if !imports {
			continue
		}

		// Scan source files for type references.
		otherDir := filepath.Join(moduleRoot, filepath.FromSlash(other.ImportPath))
		scanDirForRefs(otherDir, moduleRoot, other.ImportPath, pkgShortName, exportedNames, impact.TypeRefs, false)
		// Also scan test files.
		scanDirForRefs(otherDir, moduleRoot, other.ImportPath, pkgShortName, exportedNames, impact.TypeRefs, true)
	}

	// Sort references for stable output.
	for name := range impact.TypeRefs {
		sort.Slice(impact.TypeRefs[name], func(i, j int) bool {
			return impact.TypeRefs[name][i].File < impact.TypeRefs[name][j].File
		})
	}

	return impact
}

// scanDirForRefs scans .go files in dir for references to exported types.
func scanDirForRefs(
	dir, moduleRoot, dirPkg, pkgShortName string,
	exportedNames map[string]bool,
	refs map[string][]TypeReference,
	testFiles bool,
) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		isTest := strings.HasSuffix(name, "_test.go")
		if testFiles != isTest {
			continue
		}

		path := filepath.Join(dir, name)
		relPath, _ := filepath.Rel(moduleRoot, path)
		relPath = filepath.ToSlash(relPath)

		// Read file and search for "<pkg>.<Type>" patterns.
		found := searchFileForRefs(path, pkgShortName, exportedNames)
		for typeName := range found {
			refs[typeName] = append(refs[typeName], TypeReference{
				File:    relPath,
				Package: dirPkg,
				IsTest:  isTest,
			})
		}
	}
}

// searchFileForRefs does a line-by-line scan for "<pkg>.<TypeName>" in a file.
func searchFileForRefs(path, pkgShortName string, exportedNames map[string]bool) map[string]bool {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	found := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for name := range exportedNames {
			// Look for "pkg.Type" pattern (e.g. "coordinator.SpawnRequest").
			pattern := pkgShortName + "." + name
			if strings.Contains(line, pattern) {
				found[name] = true
			}
		}
	}
	return found
}

// findMocks scans test files for mock struct types.
func findMocks(moduleRoot string) []MockInfo {
	var mocks []MockInfo

	_ = filepath.Walk(moduleRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(path, "vendor/") || strings.Contains(path, "testdata/") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		relPath, _ := filepath.Rel(moduleRoot, path)
		relPath = filepath.ToSlash(relPath)

		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				name := ts.Name.Name
				if !strings.HasPrefix(name, "mock") && !strings.HasPrefix(name, "fake") &&
					!strings.HasPrefix(name, "stub") {
					continue
				}
				mocks = append(mocks, MockInfo{
					File:     relPath,
					TypeName: name,
				})
			}
		}
		return nil
	})

	return mocks
}

// matchesMock guesses if a mock type implements a given interface.
func matchesMock(mock MockInfo, ifaceName string) bool {
	lower := strings.ToLower(mock.TypeName)
	target := strings.ToLower(ifaceName)

	for _, prefix := range []string{"mock", "fake", "stub"} {
		if strings.HasPrefix(lower, prefix) {
			stripped := lower[len(prefix):]
			if stripped == target {
				return true
			}
		}
	}
	return false
}
