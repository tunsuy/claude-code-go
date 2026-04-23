package memdir

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// DefaultMemoryBase is the default base directory for memory storage.
const DefaultMemoryBase = ".claude"

// MemoryFileName is the memory index file name.
const MemoryFileName = "MEMORY.md"

// MaxMemoryIndexLines is the maximum line count for MEMORY.md.
const MaxMemoryIndexLines = 200

// MaxMemoryIndexBytes is the maximum byte size for MEMORY.md.
const MaxMemoryIndexBytes = 25_000

// MemoryStore manages reading and writing memory files for a project.
type MemoryStore struct {
	// memoryDir is the absolute path to the project's memory directory.
	memoryDir string
}

// NewMemoryStore creates a MemoryStore for the given project directory.
// It computes the memory path as ~/.claude/projects/<slug>/memory/.
func NewMemoryStore(projectDir string) (*MemoryStore, error) {
	memDir, err := autoMemoryPath(projectDir)
	if err != nil {
		return nil, fmt.Errorf("memdir: compute memory path: %w", err)
	}
	return &MemoryStore{memoryDir: memDir}, nil
}

// NewMemoryStoreWithPath creates a MemoryStore with an explicit memory directory.
// Useful for testing and custom configurations.
func NewMemoryStoreWithPath(memoryDir string) *MemoryStore {
	return &MemoryStore{memoryDir: memoryDir}
}

// Dir returns the absolute path to the memory directory.
func (ms *MemoryStore) Dir() string {
	return ms.memoryDir
}

// EnsureDir creates the memory directory if it does not exist.
func (ms *MemoryStore) EnsureDir() error {
	return os.MkdirAll(ms.memoryDir, 0o755)
}

// WriteMemory creates or updates a memory file.
// The filename is derived from the title if not specified.
func (ms *MemoryStore) WriteMemory(mf *MemoryFile) error {
	if err := ms.EnsureDir(); err != nil {
		return fmt.Errorf("memdir: ensure dir: %w", err)
	}

	if mf.Path == "" {
		mf.Path = filepath.Join(ms.memoryDir, slugify(mf.Header.Title)+".md")
	}

	mf.Header.UpdatedAt = time.Now()
	if mf.Header.CreatedAt.IsZero() {
		mf.Header.CreatedAt = mf.Header.UpdatedAt
	}

	content := FormatMemoryFile(mf)
	return os.WriteFile(mf.Path, []byte(content), 0o644)
}

// ReadMemory reads a single memory file by name (without .md extension).
func (ms *MemoryStore) ReadMemory(name string) (*MemoryFile, error) {
	path := filepath.Join(ms.memoryDir, name+".md")
	return readMemoryFile(path)
}

// ListMemories returns all memory files in the memory directory, sorted by
// update time (most recent first).
func (ms *MemoryStore) ListMemories() ([]*MemoryFile, error) {
	entries, err := os.ReadDir(ms.memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("memdir: list memories: %w", err)
	}

	var memories []*MemoryFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == MemoryFileName {
			continue // Skip the index file itself
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue // Skip hidden files
		}
		path := filepath.Join(ms.memoryDir, entry.Name())
		mf, err := readMemoryFile(path)
		if err != nil {
			continue // Skip unreadable files
		}
		memories = append(memories, mf)
	}

	// Sort by UpdatedAt descending (most recent first).
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Header.UpdatedAt.After(memories[j].Header.UpdatedAt)
	})

	return memories, nil
}

// DeleteMemory removes a memory file by name (without .md extension).
func (ms *MemoryStore) DeleteMemory(name string) error {
	path := filepath.Join(ms.memoryDir, name+".md")
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("memdir: delete memory %q: %w", name, err)
	}
	return nil
}

// BuildIndex regenerates the MEMORY.md index file from all memory files.
// The index uses Markdown link format `[Title](filename.md) — summary` so that
// the LLM can directly identify file names for subsequent read/edit operations,
// consistent with the original Claude Code (TypeScript) implementation.
func (ms *MemoryStore) BuildIndex() error {
	memories, err := ms.ListMemories()
	if err != nil {
		return fmt.Errorf("memdir: build index: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Project Memories\n\n")
	sb.WriteString(fmt.Sprintf("_Last updated: %s_\n\n", time.Now().Format(time.RFC3339)))

	// Group by type.
	byType := make(map[MemoryType][]*MemoryFile)
	for _, mf := range memories {
		byType[mf.Header.Type] = append(byType[mf.Header.Type], mf)
	}

	typeOrder := []MemoryType{MemoryTypeUser, MemoryTypeFeedback, MemoryTypeProject, MemoryTypeReference}
	typeLabels := map[MemoryType]string{
		MemoryTypeUser:      "User Preferences",
		MemoryTypeFeedback:  "Feedback & Corrections",
		MemoryTypeProject:   "Project Knowledge",
		MemoryTypeReference: "External References",
	}

	for _, mt := range typeOrder {
		mfs := byType[mt]
		if len(mfs) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("## %s\n\n", typeLabels[mt]))
		for _, mf := range mfs {
			title := mf.Header.Title
			filename := filepath.Base(mf.Path)
			if title == "" {
				title = strings.TrimSuffix(filename, ".md")
			}
			// Use Markdown link format: [Title](filename.md) — summary
			// This allows the LLM to directly see the filename for read/edit.
			summary := firstLine(mf.Body)
			if summary != "" {
				sb.WriteString(fmt.Sprintf("- [%s](%s) — %s\n", title, filename, summary))
			} else {
				sb.WriteString(fmt.Sprintf("- [%s](%s)\n", title, filename))
			}
		}
		sb.WriteString("\n")
	}

	indexContent := sb.String()
	// Enforce size limits.
	indexContent = truncateUTF8Safe(indexContent, MaxMemoryIndexBytes)

	indexPath := filepath.Join(ms.memoryDir, MemoryFileName)
	return os.WriteFile(indexPath, []byte(indexContent), 0o644)
}

// LoadMemoryIndex reads and returns the content of the MEMORY.md index file.
func (ms *MemoryStore) LoadMemoryIndex() (string, error) {
	indexPath := filepath.Join(ms.memoryDir, MemoryFileName)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("memdir: load memory index: %w", err)
	}
	return string(data), nil
}

// autoMemoryPath computes the project's auto memory directory:
// ~/.claude/projects/<slug>/memory/
func autoMemoryPath(projectDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}

	absProject, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("abs project dir: %w", err)
	}

	slug := sanitizeProjectSlug(absProject)
	return filepath.Join(home, DefaultMemoryBase, "projects", slug, "memory"), nil
}

// sanitizeProjectSlug creates a filesystem-safe slug from a project path.
// Format: <basename>-<hash8>
func sanitizeProjectSlug(absPath string) string {
	base := filepath.Base(absPath)
	base = strings.ToLower(base)

	// Remove non-alphanumeric characters.
	var sb strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	sanitised := sb.String()
	if sanitised == "" {
		sanitised = "project"
	}

	h := sha256.Sum256([]byte(absPath))
	hash8 := hex.EncodeToString(h[:4])

	return sanitised + "-" + hash8
}

// readMemoryFile reads and parses a single memory file from disk.
func readMemoryFile(path string) (*MemoryFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseMemoryFile(string(data), path)
}

// slugify converts a title to a filesystem-safe filename slug.
func slugify(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	if title == "" {
		return fmt.Sprintf("memory_%d", time.Now().UnixMilli())
	}

	var sb strings.Builder
	prevDash := false
	for _, r := range title {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			sb.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash {
				sb.WriteRune('_')
				prevDash = true
			}
		}
	}
	result := strings.Trim(sb.String(), "_")
	if result == "" {
		return fmt.Sprintf("memory_%d", time.Now().UnixMilli())
	}
	// Limit slug length.
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}

// firstLine returns the first non-empty line of a string, truncated to 120 chars.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 120 {
				return line[:117] + "..."
			}
			return line
		}
	}
	return ""
}

// truncateUTF8Safe truncates a string to at most maxBytes without breaking
// multi-byte UTF-8 characters.
func truncateUTF8Safe(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backwards from maxBytes to find a valid UTF-8 boundary.
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
