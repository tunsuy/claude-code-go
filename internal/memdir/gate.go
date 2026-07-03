package memdir

import (
	"os"
	"strings"
)

// IsAutoMemoryEnabled reports whether auto-memory features should run.
//
// Priority:
//  1. CLAUDE_CODE_DISABLE_AUTO_MEMORY env var disables memory
//  2. bare/simple mode disables memory
//  3. settings.json autoMemoryEnabled (when non-nil)
//  4. default enabled
func IsAutoMemoryEnabled(bareMode bool, settingsEnabled *bool) bool {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY")); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		return false
	}
	if bareMode {
		return false
	}
	if settingsEnabled != nil {
		return *settingsEnabled
	}
	return true
}

// IsExtractMemoriesEnabled reports whether the extract-memories stop hook should run.
func IsExtractMemoriesEnabled(bareMode bool, settingsEnabled *bool) bool {
	return IsAutoMemoryEnabled(bareMode, settingsEnabled)
}

// IsAutoDreamEnabled reports whether background memory consolidation should run.
func IsAutoDreamEnabled(bareMode bool, settingsEnabled *bool) bool {
	return IsAutoMemoryEnabled(bareMode, settingsEnabled)
}
