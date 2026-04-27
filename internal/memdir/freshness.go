package memdir

import (
	"fmt"
	"time"
)

// MemoryAge returns a human-readable description of a memory's age.
func MemoryAge(updatedAt time.Time) string {
	days := int(time.Since(updatedAt).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 7:
		return fmt.Sprintf("%d days ago", days)
	case days < 30:
		return fmt.Sprintf("%d weeks ago", days/7)
	default:
		return fmt.Sprintf("%d months ago", days/30)
	}
}

// MemoryFreshnessText returns a freshness warning for memories older than 1 day.
// Returns empty string for fresh memories.
func MemoryFreshnessText(updatedAt time.Time) string {
	days := int(time.Since(updatedAt).Hours() / 24)
	if days <= 1 {
		return ""
	}
	return fmt.Sprintf("This memory was last updated %s. It may be outdated.", MemoryAge(updatedAt))
}
