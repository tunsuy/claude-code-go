package main

import (
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
	// File is the path relative to module root (e.g. "internal/tools/agent/agent_test.go").
	File string
	// TypeName is the mock struct name (e.g. "mockCoordinator").
	TypeName string
	// Implements is the interface it mocks (best-effort guess from field patterns).
	Implements string
}

// InterfaceImpact describes the change impact of modifying an interface.
type InterfaceImpact struct {
	// InterfaceName is the full name (e.g. "tools.AgentCoordinator").
	InterfaceName string
	// Package is where the interface is defined.
	Package string
	// Implementors are packages that contain concrete implementations.
	Implementors []string
	// Mocks are test mock locations.
	Mocks []MockInfo
	// Adapters are adapter/bridge files.
	Adapters []string
}

// analyzeChangeImpact builds change impact data for all interfaces.
func analyzeChangeImpact(packages []*PackageInfo, moduleRoot string) []InterfaceImpact {
	// Collect all interfaces.
	type ifaceRef struct {
		pkg  string
		name string
	}
	var allIfaces []ifaceRef
	for _, pkg := range packages {
		for _, iface := range pkg.Interfaces {
			allIfaces = append(allIfaces, ifaceRef{pkg: pkg.ImportPath, name: iface.Name})
		}
	}

	// Find mocks in test files.
	mocks := findMocks(moduleRoot)

	// Build impact for each interface.
	var impacts []InterfaceImpact
	for _, ref := range allIfaces {
		impact := InterfaceImpact{
			InterfaceName: ref.name,
			Package:       ref.pkg,
		}

		// Find adapters — files named "adapter.go" in the same package.
		adapterPath := filepath.Join(moduleRoot, filepath.FromSlash(ref.pkg), "adapter.go")
		if _, err := os.Stat(adapterPath); err == nil {
			impact.Adapters = append(impact.Adapters, ref.pkg+"/adapter.go")
		}

		// Find mocks that likely implement this interface.
		for _, mock := range mocks {
			if matchesMock(mock, ref.name) {
				impact.Mocks = append(impact.Mocks, mock)
			}
		}

		impacts = append(impacts, impact)
	}

	sort.Slice(impacts, func(i, j int) bool {
		return impacts[i].Package+"."+impacts[i].InterfaceName < impacts[j].Package+"."+impacts[j].InterfaceName
	})

	return impacts
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
		// Skip vendor, testdata, etc.
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
// Uses naming convention: mockCoordinator → Coordinator, mockClient → Client.
func matchesMock(mock MockInfo, ifaceName string) bool {
	lower := strings.ToLower(mock.TypeName)
	target := strings.ToLower(ifaceName)

	// "mockCoordinator" → "coordinator" matches "Coordinator"
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
