package memdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxIncludeDepth is the maximum recursion depth for @include directives.
const MaxIncludeDepth = 5

// ProcessIncludes processes @include directives in file content.
// Supported syntax:
//
//	@./relative/path.md  — relative to the including file's directory
//	@~/home/path.md      — relative to user's home directory
//
// Returns the processed content with includes expanded inline.
// Tracks visited paths to prevent circular includes.
func ProcessIncludes(content string, basePath string, depth int) (string, error) {
	if depth > MaxIncludeDepth {
		return content, fmt.Errorf("memdir: include depth exceeded maximum of %d", MaxIncludeDepth)
	}
	visited := make(map[string]bool)
	// Add the base file itself to visited set.
	absBase, err := filepath.Abs(basePath)
	if err == nil {
		visited[absBase] = true
	}
	return processIncludesRecursive(content, basePath, depth, visited)
}

// processIncludesRecursive is the internal recursive implementation.
func processIncludesRecursive(
	content string,
	basePath string,
	depth int,
	visited map[string]bool,
) (string, error) {
	if depth > MaxIncludeDepth {
		return content, nil
	}

	baseDir := filepath.Dir(basePath)
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !isIncludeDirective(trimmed) {
			result = append(result, line)
			continue
		}

		// Extract the path after @
		includePath := trimmed[1:] // strip leading @

		resolved, err := resolveIncludePath(includePath, baseDir)
		if err != nil {
			result = append(result, fmt.Sprintf("<!-- include not found: %s -->", includePath))
			continue
		}

		// Normalize for circular detection.
		absResolved, err := filepath.Abs(resolved)
		if err != nil {
			result = append(result, fmt.Sprintf("<!-- include not found: %s -->", includePath))
			continue
		}

		// Check for circular includes.
		if visited[absResolved] {
			result = append(result, fmt.Sprintf("<!-- circular include skipped: %s -->", includePath))
			continue
		}

		// Read the included file.
		data, err := os.ReadFile(resolved)
		if err != nil {
			result = append(result, fmt.Sprintf("<!-- include not found: %s -->", includePath))
			continue
		}

		// Mark as visited.
		visited[absResolved] = true

		// Recursively process includes in the loaded content.
		included, _ := processIncludesRecursive(string(data), resolved, depth+1, visited)
		result = append(result, included)

		// Remove from visited after processing to allow the same file
		// to be included from different branches (only prevent cycles).
		// Actually, keep it visited to prevent any re-inclusion in this tree.
	}

	return strings.Join(result, "\n"), nil
}

// isIncludeDirective returns true if the trimmed line is an @include directive.
// The line must start with @ followed by ./ or ~/
func isIncludeDirective(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	if trimmed[0] != '@' {
		return false
	}
	rest := trimmed[1:]
	return strings.HasPrefix(rest, "./") ||
		strings.HasPrefix(rest, "../") ||
		strings.HasPrefix(rest, "~/")
}

// resolveIncludePath resolves an include path to an absolute filesystem path.
func resolveIncludePath(includePath string, baseDir string) (string, error) {
	if strings.HasPrefix(includePath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("memdir: resolve home path: %w", err)
		}
		return filepath.Join(home, includePath[2:]), nil
	}

	// Relative path (starts with ./ or ../)
	resolved := filepath.Join(baseDir, includePath)
	return filepath.Clean(resolved), nil
}
