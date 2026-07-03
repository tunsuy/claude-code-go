package memdir

import (
	"fmt"
	"strings"
)

// FormatMemoryStatus builds a human-readable summary of discovered memory files
// and auto-memory contents for the /memory slash command.
func FormatMemoryStatus(workingDir string, store *MemoryStore) string {
	var sb strings.Builder
	sb.WriteString("📝 Memory System Status\n\n")

	enabled := IsAutoMemoryEnabled(false, nil)
	if enabled {
		sb.WriteString("Status: enabled\n\n")
	} else {
		sb.WriteString("Status: disabled (set CLAUDE_CODE_DISABLE_AUTO_MEMORY or autoMemoryEnabled=false)\n\n")
	}

	sb.WriteString("## CLAUDE.md Files\n")
	scoped, err := DiscoverAll(workingDir)
	if err != nil || len(scoped) == 0 {
		paths := DiscoverClaudeMd(workingDir)
		if len(paths) == 0 {
			sb.WriteString("  (none found)\n")
		}
		for _, p := range paths {
			sb.WriteString(fmt.Sprintf("  • %s\n", p))
		}
	} else {
		for _, f := range scoped {
			sb.WriteString(fmt.Sprintf("  • [%s] %s\n", f.Scope, f.Path))
		}
	}

	sb.WriteString("\n## Auto Memories\n")
	if store == nil {
		sb.WriteString("  (memory store unavailable)\n")
		return strings.TrimRight(sb.String(), "\n")
	}

	sb.WriteString(fmt.Sprintf("  Directory: %s\n", store.Dir()))
	memories, listErr := store.ListMemories()
	if listErr != nil {
		sb.WriteString(fmt.Sprintf("  (error listing memories: %v)\n", listErr))
		return strings.TrimRight(sb.String(), "\n")
	}
	if len(memories) == 0 {
		sb.WriteString("  (no memory files yet)\n")
	} else {
		sb.WriteString(fmt.Sprintf("  %d memory file(s):\n", len(memories)))
		for _, mf := range memories {
			title := mf.Header.Title
			if title == "" {
				title = mf.Path
			}
			sb.WriteString(fmt.Sprintf("  • %s (%s)\n", title, mf.Header.Type))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
