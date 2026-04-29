// Command docgen generates per-package CONTEXT.md files and a global INDEX.md
// by parsing the Go AST of all packages in the project. The generated documents
// are consumed by AI coding assistants (via CLAUDE.md references) to reduce
// token usage when exploring the codebase.
//
// Usage:
//
//	go run ./cmd/docgen -out docs/generated ./internal ./pkg ./cmd
//	go run ./cmd/docgen -out docs/generated -check ./internal ./pkg ./cmd
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	outDir := flag.String("out", "docs/generated", "Output directory for generated docs")
	check := flag.Bool("check", false, "Check mode: exit 1 if generated docs are out of date")
	flag.Parse()

	roots := flag.Args()
	if len(roots) == 0 {
		roots = []string{"./internal", "./pkg", "./cmd"}
	}

	// Determine module root (directory containing go.mod).
	moduleRoot, err := findModuleRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Discover all Go packages.
	dirs, err := discoverPackages(moduleRoot, roots)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error discovering packages: %v\n", err)
		os.Exit(1)
	}

	// Analyze each package.
	var packages []*PackageInfo
	for _, dir := range dirs {
		pkg, analyzeErr := analyzePackage(dir, moduleRoot)
		if analyzeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", analyzeErr)
			continue
		}
		packages = append(packages, pkg)
	}

	// Sort packages by layer order then name.
	sort.Slice(packages, func(i, j int) bool {
		li := layerOrder(packages[i].Layer)
		lj := layerOrder(packages[j].Layer)
		if li != lj {
			return li > lj // higher layers first
		}
		return packages[i].ImportPath < packages[j].ImportPath
	})

	// Build dependency graph.
	deps := buildDependencyGraph(packages)

	// Build per-package change impact (type references, mocks, adapters).
	pkgImpacts := make(map[string]*PackageImpact)
	for _, pkg := range packages {
		pkgImpacts[pkg.ImportPath] = analyzePackageImpact(pkg, packages, moduleRoot)
	}

	// Generate all documents.
	// Per-package CONTEXT.md goes into each package directory.
	// INDEX.md goes into the -out directory.
	globalDocs := make(map[string]string) // relative to -out dir
	pkgDocs := make(map[string]string)    // absolute paths

	// INDEX.md
	globalDocs["INDEX.md"] = renderIndex(packages)

	// Per-package files → write to package directory.
	// Preserve existing Design Notes sections across regeneration.
	for _, pkg := range packages {
		absPath := filepath.Join(moduleRoot, filepath.FromSlash(pkg.ImportPath), "CONTEXT.md")
		impact := pkgImpacts[pkg.ImportPath]
		generated := renderPackage(pkg, deps, impact)

		// Extract and re-append any existing Design Notes.
		existingNotes := extractDesignNotes(absPath)
		pkgDocs[absPath] = appendDesignNotes(generated, existingNotes)
	}

	// Write or check.
	absOut := filepath.Join(moduleRoot, *outDir)

	if *check {
		exitCode := 0

		// Check 1: AST-generated content is up to date.
		stale := checkStaleMulti(absOut, globalDocs, pkgDocs)
		if len(stale) > 0 {
			fmt.Fprintf(os.Stderr, "docs are out of date. Run 'make docs' to regenerate.\n")
			fmt.Fprintf(os.Stderr, "stale files:\n")
			for _, f := range stale {
				fmt.Fprintf(os.Stderr, "  %s\n", f)
			}
			exitCode = 1
		}

		// Check 2: Changed packages must have Design Notes.
		missing := checkDesignNotes(moduleRoot, packages)
		if len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "\npackages with code changes but no Design Notes:\n")
			for _, m := range missing {
				fmt.Fprintf(os.Stderr, "  %s/CONTEXT.md — add ## Design Notes explaining why\n", m)
			}
			exitCode = 1
		}

		if exitCode == 0 {
			fmt.Println("docs are up to date.")
		}
		os.Exit(exitCode)
	}

	// Write files.
	written := 0

	// Global docs (INDEX.md, change-impact.md) → docs/generated/
	for relPath, content := range globalDocs {
		fullPath := filepath.Join(absOut, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating dir: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", fullPath, err)
			os.Exit(1)
		}
		written++
	}

	// Per-package CONTEXT.md → each package directory.
	for absPath, content := range pkgDocs {
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", absPath, err)
			os.Exit(1)
		}
		written++
	}

	fmt.Printf("generated %d files (%d package CONTEXT.md + %d global)\n",
		written, len(pkgDocs), len(globalDocs))
}

// findModuleRoot walks up from cwd to find go.mod.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

// discoverPackages finds all directories containing .go files under the given roots.
func discoverPackages(moduleRoot string, roots []string) ([]string, error) {
	seen := make(map[string]bool)
	var dirs []string

	for _, root := range roots {
		absRoot := filepath.Join(moduleRoot, root)
		err := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if d.IsDir() {
				// Skip hidden dirs, testdata, vendor.
				name := d.Name()
				if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
				dir := filepath.Dir(path)
				if !seen[dir] {
					seen[dir] = true
					dirs = append(dirs, dir)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(dirs)
	return dirs, nil
}

// checkStaleMulti compares generated content against existing files for both
// global docs (relative to outDir) and package docs (absolute paths).
func checkStaleMulti(outDir string, globalDocs map[string]string, pkgDocs map[string]string) []string {
	var stale []string

	// Check global docs.
	for relPath, content := range globalDocs {
		fullPath := filepath.Join(outDir, relPath)
		existing, err := os.ReadFile(fullPath)
		if err != nil {
			stale = append(stale, relPath+" (missing)")
			continue
		}
		if hashContent(string(existing)) != hashContent(content) {
			stale = append(stale, relPath)
		}
	}

	// Check per-package CONTEXT.md.
	for absPath, content := range pkgDocs {
		existing, err := os.ReadFile(absPath)
		if err != nil {
			stale = append(stale, absPath+" (missing)")
			continue
		}
		if hashContent(string(existing)) != hashContent(content) {
			stale = append(stale, absPath)
		}
	}

	sort.Strings(stale)
	return stale
}

// hashContent returns a SHA-256 hash of the content with generated_at lines stripped.
func hashContent(content string) string {
	lines := strings.Split(content, "\n")
	var filtered []string
	for _, line := range lines {
		if strings.HasPrefix(line, "generated_at:") {
			continue
		}
		filtered = append(filtered, line)
	}
	h := sha256.Sum256([]byte(strings.Join(filtered, "\n")))
	return fmt.Sprintf("%x", h)
}

// layerOrder returns the sort order for a layer (higher = listed first).
func layerOrder(layer string) int {
	for _, l := range layers {
		if l.Name == layer {
			return l.Order
		}
	}
	return -1
}

// checkDesignNotes uses git diff to find packages with changed .go files,
// then checks if those packages have a non-empty Design Notes section in
// their CONTEXT.md. Returns import paths of packages missing notes.
func checkDesignNotes(moduleRoot string, packages []*PackageInfo) []string {
	changedPkgs := getChangedPackages(moduleRoot)
	if len(changedPkgs) == 0 {
		return nil
	}

	// Build a set of known package import paths for filtering.
	knownPkgs := make(map[string]bool)
	for _, pkg := range packages {
		knownPkgs[pkg.ImportPath] = true
	}

	var missing []string
	for _, pkgPath := range changedPkgs {
		if !knownPkgs[pkgPath] {
			continue
		}
		contextPath := filepath.Join(moduleRoot, filepath.FromSlash(pkgPath), "CONTEXT.md")
		notes := extractDesignNotes(contextPath)
		if strings.TrimSpace(notes) == "" {
			missing = append(missing, pkgPath)
		}
	}

	sort.Strings(missing)
	return missing
}

// getChangedPackages runs git diff to find packages with changed .go files.
// It compares against the merge-base with main (i.e. changes on this branch).
// Falls back to HEAD~1 if merge-base fails. Returns relative import paths.
func getChangedPackages(moduleRoot string) []string {
	// Try to find merge-base with main.
	base := "HEAD~1"
	if out, err := exec.Command("git", "merge-base", "HEAD", "main").Output(); err == nil {
		base = strings.TrimSpace(string(out))
	}

	// Get changed .go files (excluding tests and CONTEXT.md).
	out, err := exec.Command("git", "diff", "--name-only", base, "--", "*.go").Output()
	if err != nil {
		// Fallback: check staged + unstaged changes.
		out, err = exec.Command("git", "diff", "--name-only", "HEAD", "--", "*.go").Output()
		if err != nil {
			return nil
		}
	}

	// Extract unique package directories.
	pkgSet := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip test files — they don't need Design Notes.
		if strings.HasSuffix(line, "_test.go") {
			continue
		}
		// Skip generated files.
		if strings.HasSuffix(line, "CONTEXT.md") {
			continue
		}
		dir := filepath.Dir(line)
		dir = filepath.ToSlash(dir)
		pkgSet[dir] = true
	}

	var result []string
	for pkg := range pkgSet {
		result = append(result, pkg)
	}
	sort.Strings(result)
	return result
}
