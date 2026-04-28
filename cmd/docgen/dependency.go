package main

import "sort"

// DependencyGraph holds the import relationships between packages.
type DependencyGraph struct {
	// Forward maps a package to the packages it imports.
	Forward map[string][]string
	// Reverse maps a package to the packages that import it.
	Reverse map[string][]string
}

// buildDependencyGraph constructs forward and reverse dependency edges
// from all analyzed packages.
func buildDependencyGraph(packages []*PackageInfo) *DependencyGraph {
	g := &DependencyGraph{
		Forward: make(map[string][]string),
		Reverse: make(map[string][]string),
	}

	// Build a set of known packages for filtering.
	known := make(map[string]bool)
	for _, pkg := range packages {
		known[pkg.ImportPath] = true
	}

	for _, pkg := range packages {
		for _, imp := range pkg.Imports {
			if !known[imp] {
				continue // external dependency, skip
			}
			g.Forward[pkg.ImportPath] = append(g.Forward[pkg.ImportPath], imp)
			g.Reverse[imp] = append(g.Reverse[imp], pkg.ImportPath)
		}
	}

	// Sort for stable output.
	for k := range g.Forward {
		sort.Strings(g.Forward[k])
	}
	for k := range g.Reverse {
		sort.Strings(g.Reverse[k])
	}

	return g
}
