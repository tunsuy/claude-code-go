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
	return full[:cutAt] + notice
}
