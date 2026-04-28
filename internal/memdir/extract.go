package memdir

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// maxSummaryChars is the maximum character count for the conversation summary
// passed to the extraction agent. This prevents overwhelming the agent with
// very long conversations.
const maxSummaryChars = 4000

// defaultMaxSummaryMessages is the default number of recent messages to include
// in the conversation summary.
const defaultMaxSummaryMessages = 20

// foregroundQuerySource is the query source tag for the main (interactive) thread.
const foregroundQuerySource = "foreground"

// extractMemoriesQuerySource is the query source tag for the extraction agent.
const extractMemoriesQuerySource = "extract_memories"

// ExtractMemoriesConfig configures memory extraction behavior.
type ExtractMemoriesConfig struct {
	// Store is the memory store for the current project.
	Store *MemoryStore
	// MaxTurns is the maximum turns for the extraction agent (default: 3).
	MaxTurns int
	// Enabled controls whether extraction runs. Default true.
	Enabled bool
}

// DefaultExtractConfig returns the default extraction configuration.
func DefaultExtractConfig() ExtractMemoriesConfig {
	return ExtractMemoriesConfig{
		MaxTurns: 3,
		Enabled:  true,
	}
}

// ExecuteExtractMemories runs memory extraction as a StopHook.
// It analyzes the conversation via a Forked Agent and extracts useful information
// into the project's memory store.
//
// This function is designed to be registered as a StopHookFn:
//
//	registry.Register("extract_memories", func(ctx context.Context, hookCtx *engine.StopHookContext) {
//	    ExecuteExtractMemories(ctx, hookCtx, cfg)
//	})
func ExecuteExtractMemories(
	ctx context.Context,
	hookCtx *engine.StopHookContext,
	cfg ExtractMemoriesConfig,
) {
	// Guard: extraction disabled.
	if !cfg.Enabled {
		return
	}

	// Guard: no memory store configured.
	if cfg.Store == nil {
		return
	}

	// Guard: skip in bare/simple mode — no memories for lightweight sessions.
	if hookCtx.IsBareMode {
		return
	}

	// Guard: only run for the main interactive thread.
	// Forked agents (sub-agents, other hooks) should not trigger extraction.
	if hookCtx.QuerySource != foregroundQuerySource {
		return
	}

	// Guard: skip if the main agent already wrote memories this turn.
	if hasMemoryWritesSince(hookCtx.Messages) {
		return
	}

	// Build conversation summary from recent messages.
	summary := buildConversationSummary(hookCtx.Messages, defaultMaxSummaryMessages)
	if strings.TrimSpace(summary) == "" {
		return
	}

	// Construct the user prompt for the extraction agent.
	promptText := BuildExtractionPrompt(summary)

	// Resolve MaxTurns from config, falling back to default.
	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultExtractConfig().MaxTurns
	}

	// Build the forked agent configuration.
	forkedCfg := engine.ForkedAgentConfig{
		PromptMessages: []types.Message{
			{
				Role: types.RoleUser,
				Content: []types.ContentBlock{
					{
						Type: types.ContentTypeText,
						Text: strPtr(promptText),
					},
				},
			},
		},
		CacheSafeParams: hookCtx.CacheParams,
		QuerySource:     extractMemoriesQuerySource,
		MaxTurns:        maxTurns,
		AllowedTools:    []string{"MemoryRead", "MemoryWrite", "MemoryDelete"},
	}

	// Run the forked agent. Errors are logged but not propagated —
	// extraction is best-effort and must not affect the main session.
	_, err := engine.RunForkedAgent(ctx, hookCtx.Engine, forkedCfg)
	if err != nil {
		log.Printf("memdir: extract memories: %v", err)
	}
}

// hasMemoryWritesSince checks if the main agent has already written memories
// during the current turn. If so, extraction is skipped to avoid duplicates.
//
// It scans messages from the end backwards, looking for tool_use blocks with
// Name "MemoryWrite". Scanning stops at the last user message (turn boundary).
func hasMemoryWritesSince(messages []types.Message) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]

		// The last user message marks the turn boundary.
		if msg.Role == types.RoleUser {
			return false
		}

		// Check assistant messages for MemoryWrite tool use.
		if msg.Role == types.RoleAssistant {
			for _, block := range msg.Content {
				if block.Type == types.ContentTypeToolUse && block.Name != nil && *block.Name == "MemoryWrite" {
					return true
				}
			}
		}
	}
	return false
}

// buildConversationSummary extracts a text summary of recent conversation turns.
// It includes the last maxMessages user/assistant message pairs, focusing on
// text content. The summary is truncated to maxSummaryChars to avoid
// overwhelming the extraction agent.
func buildConversationSummary(messages []types.Message, maxMessages int) string {
	if len(messages) == 0 {
		return ""
	}

	// Determine the start index: only include the last maxMessages messages.
	start := 0
	if len(messages) > maxMessages {
		start = len(messages) - maxMessages
	}

	var sb strings.Builder
	for _, msg := range messages[start:] {
		// Only include user and assistant text content.
		if msg.Role != types.RoleUser && msg.Role != types.RoleAssistant {
			continue
		}

		texts := extractTextContent(msg)
		if texts == "" {
			continue
		}

		label := roleLabel(msg.Role)
		entry := fmt.Sprintf("[%s]: %s\n\n", label, texts)

		// Enforce the character limit.
		if sb.Len()+len(entry) > maxSummaryChars {
			remaining := maxSummaryChars - sb.Len()
			if remaining > 0 {
				sb.WriteString(entry[:remaining])
				sb.WriteString("...\n")
			}
			break
		}
		sb.WriteString(entry)
	}

	return strings.TrimSpace(sb.String())
}

// extractTextContent extracts and joins all text content blocks from a message.
func extractTextContent(msg types.Message) string {
	var parts []string
	for _, block := range msg.Content {
		if block.Type == types.ContentTypeText && block.Text != nil {
			text := strings.TrimSpace(*block.Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

// roleLabel returns a human-readable label for a message role.
func roleLabel(role types.Role) string {
	switch role {
	case types.RoleUser:
		return "User"
	case types.RoleAssistant:
		return "Assistant"
	default:
		return string(role)
	}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
