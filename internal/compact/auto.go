package compact

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// Auto-compact constants (mirrors TS autoCompact.ts).
const (
	// AutoCompactBufferTokens is subtracted from the model context window to
	// determine the auto-compact trigger threshold.
	AutoCompactBufferTokens = 13_000

	// AutoCompactWarningBufferTokens is the additional buffer for the warning
	// threshold (context window − maxOutput − warningBuffer).
	AutoCompactWarningBufferTokens = 20_000

	// MaxOutputTokensForSummary is the maximum output tokens allocated to the
	// summarisation request itself.
	MaxOutputTokensForSummary = 20_000

	// MaxConsecutiveFailures is the circuit-breaker limit: after this many
	// consecutive auto-compact failures we stop retrying.
	MaxConsecutiveFailures = 3

	// defaultContextWindow is used when the model's window cannot be resolved.
	defaultContextWindow = 200_000
)

// AutoCompactTrackingState records session-scoped auto-compact state.
type AutoCompactTrackingState struct {
	// Compacted is true if at least one auto-compact has succeeded this session.
	Compacted bool
	// TurnCounter is the number of turns since the last successful compaction.
	TurnCounter int
	// TurnID is an opaque identifier from the last turn.
	TurnID string
	// ConsecutiveFailures counts back-to-back compaction failures.
	ConsecutiveFailures int
}

// AutoCompactor implements Compressor using LLM summarisation.
// It triggers when the estimated context token count exceeds a model-specific
// threshold derived from the effective context window.
type AutoCompactor struct {
	// client is used to call the LLM for summarisation.
	client api.Client
	// model is the summarisation model (may differ from the conversation model).
	model string
	// maxTokens caps the token count for the summarisation request.
	maxTokens int
	// tracking records session-level compaction state.
	tracking AutoCompactTrackingState
}

// NewAutoCompactor constructs an AutoCompactor.
// model is the model used for the summarisation call (typically the same as the
// main conversation model). maxTokens is the conversation model's max_tokens.
func NewAutoCompactor(client api.Client, model string, maxTokens int) *AutoCompactor {
	if maxTokens <= 0 {
		maxTokens = MaxOutputTokensForSummary
	}
	return &AutoCompactor{
		client:    client,
		model:     model,
		maxTokens: maxTokens,
	}
}

// autoCompactThreshold returns the token count at which auto-compact should trigger.
// threshold = effectiveContextWindow − AutoCompactBufferTokens
// effectiveContextWindow = contextWindow − min(maxOutputTokens, 20_000)
func autoCompactThreshold(model string, maxOutputTokens int) int {
	contextWindow := resolveContextWindow(model)
	maxOutput := maxOutputTokens
	if maxOutput > MaxOutputTokensForSummary {
		maxOutput = MaxOutputTokensForSummary
	}
	effective := contextWindow - maxOutput
	return effective - AutoCompactBufferTokens
}

// resolveContextWindow returns the known context window for a model, falling
// back to defaultContextWindow for unrecognised models.
// Env var CLAUDE_CODE_AUTO_COMPACT_WINDOW overrides.
func resolveContextWindow(model string) int {
	if v, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW")); err == nil && v > 0 {
		return v
	}
	// Well-known model windows (approximate).
	switch {
	case strings.HasPrefix(model, "claude-3-5") || strings.HasPrefix(model, "claude-3.5"):
		return 200_000
	case strings.HasPrefix(model, "claude-3") || strings.HasPrefix(model, "claude-opus"):
		return 200_000
	case strings.HasPrefix(model, "claude-2"):
		return 100_000
	default:
		return defaultContextWindow
	}
}

// NeedsCompaction reports whether the current message history is large enough
// to warrant auto-compaction.
// estimatedTokens is derived by a rough character-count heuristic (~4 chars/token).
func (a *AutoCompactor) NeedsCompaction(messages []types.Message, model string, extra CompactionExtra) bool {
	if a.tracking.ConsecutiveFailures >= MaxConsecutiveFailures {
		return false
	}
	threshold := autoCompactThreshold(model, a.maxTokens)
	est := estimateTokens(messages) - extra.SnipTokensFreed
	return est >= threshold
}

// Compact runs the LLM-summarisation compaction pipeline.
// It returns a CompactionResult containing the summarised message history.
func (a *AutoCompactor) Compact(
	ctx context.Context,
	messages []types.Message,
	params CompactionParams,
) (*CompactionResult, error) {
	preCount := estimateTokens(messages)

	// Build the summarisation system prompt.
	summarySystem := buildSummarySystemPrompt(params)

	// Convert messages to API params.
	apiMessages, err := messagesToAPIParams(messages)
	if err != nil {
		a.tracking.ConsecutiveFailures++
		return nil, fmt.Errorf("compact: marshal messages: %w", err)
	}

	req := &api.MessageRequest{
		Model:     a.model,
		MaxTokens: MaxOutputTokensForSummary,
		Messages:  apiMessages,
		System:    summarySystem,
		Stream:    false,
	}

	resp, err := a.client.Complete(ctx, req)
	if err != nil {
		a.tracking.ConsecutiveFailures++
		return nil, fmt.Errorf("compact: LLM summarisation: %w", err)
	}

	// Extract the summary text from the response.
	summaryText := extractTextFromResponse(resp)
	if summaryText == "" {
		a.tracking.ConsecutiveFailures++
		return nil, fmt.Errorf("compact: empty summary from LLM")
	}

	// Build the replacement message list: one user summary message.
	summaryMsg := types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr(summaryText)},
		},
	}

	// Reset consecutive failure counter on success.
	a.tracking.ConsecutiveFailures = 0
	a.tracking.Compacted = true
	a.tracking.TurnCounter = 0

	postCount := estimateTokens([]types.Message{summaryMsg})

	return &CompactionResult{
		SummaryMessages:      []types.Message{summaryMsg},
		PreCompactTokenCount: preCount,
		PostCompactTokenCount: postCount,
		CompactionUsage: &TokenUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
			CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
		},
	}, nil
}

// GetTracking returns a copy of the current tracking state.
func (a *AutoCompactor) GetTracking() AutoCompactTrackingState {
	return a.tracking
}

// buildSummarySystemPrompt builds the system prompt for the summarisation LLM call.
func buildSummarySystemPrompt(params CompactionParams) string {
	parts := []string{
		"Your task is to create a detailed summary of the conversation so far, paying close attention to the user's explicit requests and your previous actions. This summary will be used to compact the context window while preserving all important information.",
		"The summary should include:",
		"1. The primary task or goal the user is trying to accomplish",
		"2. Key decisions made and their rationale",
		"3. Important findings, errors encountered, and how they were resolved",
		"4. Current state: what has been completed and what remains",
		"5. Any critical context needed to continue the work",
		"Please provide a comprehensive yet concise summary that will allow the conversation to continue seamlessly.",
	}
	for _, p := range params.SystemPromptParts {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n\n")
}

// extractTextFromResponse extracts plain text from the first text content block.
func extractTextFromResponse(resp *api.MessageResponse) string {
	if resp == nil {
		return ""
	}
	for _, blk := range resp.Content {
		if blk.Type == "text" && blk.Text != "" {
			return blk.Text
		}
	}
	return ""
}

// messagesToAPIParams converts types.Message slice to api.MessageParam slice.
func messagesToAPIParams(messages []types.Message) ([]api.MessageParam, error) {
	params := make([]api.MessageParam, 0, len(messages))
	for _, m := range messages {
		raw, err := json.Marshal(m.Content)
		if err != nil {
			return nil, err
		}
		params = append(params, api.MessageParam{
			Role:    string(m.Role),
			Content: raw,
		})
	}
	return params, nil
}

// estimateTokens approximates the token count for a message list using the
// rough heuristic of 4 characters per token.
func estimateTokens(messages []types.Message) int {
	raw, err := json.Marshal(messages)
	if err != nil {
		return 0
	}
	return len(raw) / 4
}

func strPtr(s string) *string { return &s }
