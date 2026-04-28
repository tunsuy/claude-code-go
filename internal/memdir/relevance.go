package memdir

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
)

// RelevanceConfig configures relevant memory surfacing.
type RelevanceConfig struct {
	// MaxMemoriesPerTurn is the maximum number of memories surfaced per turn (default: 5).
	MaxMemoriesPerTurn int
	// MaxMemoryBytes is the maximum bytes per individual memory (default: 4096).
	MaxMemoryBytes int
	// MaxSessionBytes is the cumulative byte limit for all surfaced memories in a session (default: 60000).
	MaxSessionBytes int
}

// DefaultRelevanceConfig returns the default relevance configuration.
func DefaultRelevanceConfig() RelevanceConfig {
	return RelevanceConfig{
		MaxMemoriesPerTurn: 5,
		MaxMemoryBytes:     4096,
		MaxSessionBytes:    60_000,
	}
}

// RelevantMemory represents a single relevant memory ready for injection.
type RelevantMemory struct {
	// Path is the absolute path to the memory file.
	Path string
	// Title is the memory title from the header.
	Title string
	// Content is the memory body (truncated if needed).
	Content string
	// FreshnessNote is the optional freshness warning (empty if fresh).
	FreshnessNote string
}

// scoredMemory pairs a memory file with its relevance score.
type scoredMemory struct {
	file  *MemoryFile
	score float64
}

// SurfaceRelevantMemories finds memories relevant to the user's message.
// It scans the memory store, uses keyword-based scoring to evaluate relevance,
// and returns the most relevant memories ready for system prompt injection.
//
// alreadySurfaced tracks previously surfaced memory paths to avoid duplicates.
// sessionBytesUsed is the cumulative bytes of memories already surfaced this session.
func SurfaceRelevantMemories(
	store *MemoryStore,
	userMessage string,
	alreadySurfaced map[string]bool,
	sessionBytesUsed int,
	cfg RelevanceConfig,
) ([]RelevantMemory, error) {
	if store == nil {
		return nil, nil
	}

	memories, err := store.ListMemories()
	if err != nil {
		return nil, fmt.Errorf("memdir: surface relevant memories: %w", err)
	}
	if len(memories) == 0 {
		return nil, nil
	}

	// Filter out already-surfaced memories.
	var candidates []*MemoryFile
	for _, mf := range memories {
		if !alreadySurfaced[mf.Path] {
			candidates = append(candidates, mf)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Score each candidate by keyword relevance.
	scored := make([]scoredMemory, 0, len(candidates))
	for _, mf := range candidates {
		s := scoreRelevance(userMessage, mf)
		if s > 0 {
			scored = append(scored, scoredMemory{file: mf, score: s})
		}
	}
	if len(scored) == 0 {
		return nil, nil
	}

	// Sort by score descending.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Take the top N candidates, respecting byte limits.
	bytesRemaining := cfg.MaxSessionBytes - sessionBytesUsed
	if bytesRemaining <= 0 {
		return nil, nil
	}

	var results []RelevantMemory
	for _, sm := range scored {
		if len(results) >= cfg.MaxMemoriesPerTurn {
			break
		}

		content := sm.file.Body
		// Truncate individual memory if needed.
		if len(content) > cfg.MaxMemoryBytes {
			content = truncateUTF8Safe(content, cfg.MaxMemoryBytes)
			content += "\n... (truncated)"
		}

		// Check cumulative session byte limit.
		if bytesRemaining-len(content) < 0 {
			break
		}
		bytesRemaining -= len(content)

		freshnessNote := MemoryFreshnessText(sm.file.Header.UpdatedAt)

		results = append(results, RelevantMemory{
			Path:          sm.file.Path,
			Title:         sm.file.Header.Title,
			Content:       content,
			FreshnessNote: freshnessNote,
		})
	}

	return results, nil
}

// scoreRelevance computes a keyword-based relevance score between a user message
// and a memory file. Returns a score from 0.0 to 1.0 based on keyword overlap.
// Returns 0 if there is no keyword match at all (recency alone is not sufficient).
func scoreRelevance(userMessage string, mf *MemoryFile) float64 {
	userWords := extractWords(strings.ToLower(userMessage))
	if len(userWords) == 0 {
		return 0
	}

	// Title match (high weight: 0.4).
	titleWords := extractWords(strings.ToLower(mf.Header.Title))
	titleOverlap := wordOverlap(userWords, titleWords)

	// Body first line match (medium weight: 0.25).
	bodyFirstLine := firstNonEmptyLine(mf.Body)
	bodyWords := extractWords(strings.ToLower(bodyFirstLine))
	bodyOverlap := wordOverlap(userWords, bodyWords)

	// Tag match (high weight: 0.25).
	tagScore := tagOverlap(userWords, mf.Header.Tags)

	// Require at least some keyword match to consider the memory relevant.
	keywordScore := titleOverlap*0.4 + bodyOverlap*0.25 + tagScore*0.25
	if keywordScore == 0 {
		return 0
	}

	// Recency boost (small weight: 0.1).
	recency := recencyBoost(mf.Header.UpdatedAt)
	score := keywordScore + recency*0.1

	// Clamp to [0, 1].
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// extractWords splits text into lowercase words, filtering out very short words.
func extractWords(text string) map[string]bool {
	words := make(map[string]bool)
	for _, w := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(w) >= 2 {
			words[w] = true
		}
	}
	return words
}

// wordOverlap computes the fraction of query words found in the target set.
// Returns 0 if query is empty.
func wordOverlap(query, target map[string]bool) float64 {
	if len(query) == 0 || len(target) == 0 {
		return 0
	}
	matches := 0
	for w := range query {
		if target[w] {
			matches++
		}
	}
	return float64(matches) / float64(len(query))
}

// tagOverlap computes the fraction of user words that match any tag (case-insensitive).
func tagOverlap(userWords map[string]bool, tags []string) float64 {
	if len(userWords) == 0 || len(tags) == 0 {
		return 0
	}
	tagSet := make(map[string]bool, len(tags))
	for _, tag := range tags {
		for _, w := range strings.FieldsFunc(strings.ToLower(tag), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			if len(w) >= 2 {
				tagSet[w] = true
			}
		}
	}
	matches := 0
	for w := range userWords {
		if tagSet[w] {
			matches++
		}
	}
	return float64(matches) / float64(len(userWords))
}

// recencyBoost returns a value from 0.0 to 1.0 based on how recently the memory
// was updated. Today=1.0, exponential decay with half-life of 14 days.
func recencyBoost(updatedAt time.Time) float64 {
	days := time.Since(updatedAt).Hours() / 24
	if days < 0 {
		days = 0
	}
	// Exponential decay: half-life of 14 days.
	return math.Exp(-0.693 * days / 14)
}

// firstNonEmptyLine returns the first non-empty line of a string.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// FormatRelevantMemoriesPrompt formats relevant memories for system prompt injection.
// Returns content wrapped in <system-reminder> tags.
// Returns empty string if no memories are provided.
func FormatRelevantMemoriesPrompt(memories []RelevantMemory) string {
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<system-reminder>\n## Relevant Memories\n\n")
	for _, rm := range memories {
		title := rm.Title
		if title == "" {
			title = "Untitled Memory"
		}
		sb.WriteString(fmt.Sprintf("### %s\n", title))
		sb.WriteString(rm.Content)
		sb.WriteString("\n")
		if rm.FreshnessNote != "" {
			sb.WriteString(rm.FreshnessNote)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("</system-reminder>")
	return sb.String()
}
