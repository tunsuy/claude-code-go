package memdir

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// MemoryScope represents the scope level of a CLAUDE.md file.
// Lower values are lower priority; higher values override lower ones.
type MemoryScope int

const (
	// ScopeManaged is the lowest priority — admin-level instructions.
	// Located at /etc/claude-code/CLAUDE.md (Linux/macOS only).
	ScopeManaged MemoryScope = iota
	// ScopeUser is user-global instructions.
	// Located at ~/.claude/CLAUDE.md.
	ScopeUser
	// ScopeProject is project-level instructions (git-tracked).
	// Located at {gitRoot}/CLAUDE.md, {gitRoot}/.claude/CLAUDE.md,
	// and {gitRoot}/.claude/rules/*.md.
	ScopeProject
	// ScopeLocal is the highest priority — personal project overrides (not git-tracked).
	// Located at {gitRoot}/CLAUDE.local.md.
	ScopeLocal
)

// String returns a human-readable scope name.
func (s MemoryScope) String() string {
	switch s {
	case ScopeManaged:
		return "managed"
	case ScopeUser:
		return "user"
	case ScopeProject:
		return "project"
	case ScopeLocal:
		return "local"
	default:
		return "unknown"
	}
}

// DiscoveredFile represents a discovered memory file with its scope.
type DiscoveredFile struct {
	Path  string
	Scope MemoryScope
}

// DiscoverAll finds all CLAUDE.md files across all 4 scope layers.
// Files are returned in priority order (Managed first, Local last).
// startDir is typically the project working directory.
func DiscoverAll(startDir string) ([]DiscoveredFile, error) {
	var files []DiscoveredFile

	// 1. Managed scope: /etc/claude-code/CLAUDE.md (Linux/macOS only)
	if runtime.GOOS != "windows" {
		managedPath := "/etc/claude-code/CLAUDE.md"
		if isReadableFile(managedPath) {
			files = append(files, DiscoveredFile{
				Path:  managedPath,
				Scope: ScopeManaged,
			})
		}
	}

	// 2. User scope: ~/.claude/CLAUDE.md
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		userPath := filepath.Join(home, ".claude", "CLAUDE.md")
		if isReadableFile(userPath) {
			files = append(files, DiscoveredFile{
				Path:  userPath,
				Scope: ScopeUser,
			})
		}
	}

	// 3. Project scope: requires git root
	gitRoot := findGitRoot(startDir)

	// {gitRoot}/CLAUDE.md
	projectRoot := filepath.Join(gitRoot, "CLAUDE.md")
	if isReadableFile(projectRoot) {
		files = append(files, DiscoveredFile{
			Path:  projectRoot,
			Scope: ScopeProject,
		})
	}

	// {gitRoot}/.claude/CLAUDE.md
	projectDotClaude := filepath.Join(gitRoot, ".claude", "CLAUDE.md")
	if isReadableFile(projectDotClaude) {
		files = append(files, DiscoveredFile{
			Path:  projectDotClaude,
			Scope: ScopeProject,
		})
	}

	// {gitRoot}/.claude/rules/*.md (sorted alphabetically)
	rulesDir := filepath.Join(gitRoot, ".claude", "rules")
	if info, statErr := os.Stat(rulesDir); statErr == nil && info.IsDir() {
		entries, readErr := os.ReadDir(rulesDir)
		if readErr == nil {
			var ruleFiles []string
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if filepath.Ext(entry.Name()) == ".md" {
					ruleFiles = append(ruleFiles, entry.Name())
				}
			}
			sort.Strings(ruleFiles)
			for _, name := range ruleFiles {
				rulePath := filepath.Join(rulesDir, name)
				if isReadableFile(rulePath) {
					files = append(files, DiscoveredFile{
						Path:  rulePath,
						Scope: ScopeProject,
					})
				}
			}
		}
	}

	// 4. Local scope: {gitRoot}/CLAUDE.local.md
	localPath := filepath.Join(gitRoot, "CLAUDE.local.md")
	if isReadableFile(localPath) {
		files = append(files, DiscoveredFile{
			Path:  localPath,
			Scope: ScopeLocal,
		})
	}

	return files, nil
}

// findGitRoot walks up from dir to find the nearest .git directory.
// Returns dir itself if no .git is found (fallback to working dir).
func findGitRoot(dir string) string {
	dir = filepath.Clean(dir)
	current := dir
	for {
		gitPath := filepath.Join(current, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding .git
			return dir
		}
		current = parent
	}
}
