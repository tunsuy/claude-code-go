package memdir

import (
	"os"
	"strings"
)

// LoadMemoryPrompt loads all CLAUDE.md files at the given paths and
// concatenates them with section headers into a single string.
func LoadMemoryPrompt(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		sb.WriteString("# Memory from ")
		sb.WriteString(p)
		sb.WriteString("\n\n")
		sb.Write(data)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// LoadAndTruncate loads the memory prompt and truncates it to maxBytes bytes.
// A truncation notice is appended when the content is cut.
// Fix P2-3: Uses UTF-8-safe truncation to avoid cutting multi-byte characters.
func LoadAndTruncate(paths []string, maxBytes int) string {
	full := LoadMemoryPrompt(paths)
	if len(full) <= maxBytes {
		return full
	}
	const notice = "\n\n[... memory truncated ...]"
	cutAt := maxBytes - len(notice)
	if cutAt < 0 {
		cutAt = 0
	}
	truncated := truncateUTF8Safe(full, cutAt)
	return truncated + notice
}

// LoadAllMemory loads both CLAUDE.md files and auto-memory index for a project.
// It combines traditional CLAUDE.md discovery with the project's MEMORY.md index.
// Only the index is injected into the system prompt; individual memory files
// are read on demand by the LLM using MemoryRead, consistent with how the
// original Claude Code (TypeScript) implementation works.
func LoadAllMemory(claudeMdPaths []string, store *MemoryStore) string {
	var sb strings.Builder

	// Load CLAUDE.md files first.
	claudePrompt := LoadMemoryPrompt(claudeMdPaths)
	if claudePrompt != "" {
		sb.WriteString(claudePrompt)
		sb.WriteString("\n\n")
	}

	// Load auto-memory index if available.
	// The index contains [Title](filename.md) links so the LLM can locate files.
	if store != nil {
		idx, err := store.LoadMemoryIndex()
		if err == nil && idx != "" {
			sb.WriteString("# Auto Memory\n\n")
			sb.WriteString(idx)
			sb.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(sb.String())
}
