// Package memdir handles discovery and loading of CLAUDE.md memory files.
package memdir

import (
	"os"
	"path/filepath"
)

// DiscoverClaudeMd finds all readable CLAUDE.md files starting from startDir
// and walking up to the filesystem root, then appending the user's home
// directory (if not already visited).
//
// The design collects candidate directories first so that the home directory
// is only visited once even if startDir happens to be the home directory.
func DiscoverClaudeMd(startDir string) []string {
	home, _ := os.UserHomeDir()

	// Collect directories in order: startDir → parents → (home if not seen).
	seen := make(map[string]bool)
	dirs := make([]string, 0, 8)

	dir := startDir
	for {
		clean := filepath.Clean(dir)
		if !seen[clean] {
			seen[clean] = true
			dirs = append(dirs, clean)
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			// Reached filesystem root.
			break
		}
		dir = parent
	}

	// Add home only if not already in the list.
	if home != "" {
		cleanHome := filepath.Clean(home)
		if !seen[cleanHome] {
			dirs = append(dirs, cleanHome)
		}
	}

	// Now scan each directory for a readable CLAUDE.md.
	var paths []string
	for _, d := range dirs {
		p := filepath.Join(d, "CLAUDE.md")
		if isReadableFile(p) {
			paths = append(paths, p)
		}
	}
	return paths
}

// isReadableFile returns true if path is a regular, readable file.
func isReadableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}
