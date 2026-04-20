// Package compact implements context compaction strategies for the claude-code-go engine.
// Three strategies are provided:
//   - AutoCompactor: token-threshold-based automatic compaction (LLM summarisation)
//   - MicroCompactor: per-tool-result compaction
//   - SnipCompactor: lightweight message history pruning
package compact

import (
	"context"

	"github.com/tunsuy/claude-code-go/pkg/types"
)

// Compressor is the unified interface for context compaction.
type Compressor interface {
	// NeedsCompaction reports whether the current messages warrant compaction.
	NeedsCompaction(messages []types.Message, model string, extra CompactionExtra) bool

	// Compact executes the compaction strategy and returns the resulting state.
	Compact(
		ctx context.Context,
		messages []types.Message,
		params CompactionParams,
	) (*CompactionResult, error)
}

// CompactionExtra carries ancillary information used when deciding whether to compact.
type CompactionExtra struct {
	// SnipTokensFreed is the number of tokens already freed by a prior snip operation.
	SnipTokensFreed int
}

// CompactionParams is the configuration passed to Compact.
type CompactionParams struct {
	// SystemPrompt parts to include in the summarisation request.
	SystemPromptParts []string
	// UserContext is injected into the summarisation request.
	UserContext map[string]string
	// SystemContext is injected into the summarisation request.
	SystemContext map[string]string
	// QuerySource tags the internal API request ("foreground" | "background").
	QuerySource string
	// ForkMessages is an optional set of messages used as the fork baseline.
	ForkMessages []types.Message
}

// CompactionResult is the output of a successful Compact call.
type CompactionResult struct {
	// SummaryMessages is the compacted replacement for the original history.
	SummaryMessages []types.Message
	// Attachments are additional messages appended after the summary (e.g. hook output).
	Attachments []types.Message
	// HookResults are messages injected by pre/post compact hooks.
	HookResults []types.Message
	// PreCompactTokenCount is the estimated token count before compaction.
	PreCompactTokenCount int
	// PostCompactTokenCount is the estimated token count after compaction.
	PostCompactTokenCount int
	// TruePostCompactTokenCount is the token count as reported by the API.
	TruePostCompactTokenCount int
	// CompactionUsage records the API token usage incurred during compaction.
	CompactionUsage *TokenUsage
}

// TokenUsage records Anthropic API token consumption.
type TokenUsage struct {
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
}

