package compact

import (
	"github.com/anthropics/claude-code-go/pkg/types"
)

// SnipResult is the output of a SnipCompactIfNeeded call.
type SnipResult struct {
	// Messages is the pruned message list.
	Messages []types.Message
	// TokensFreed is the estimated number of tokens removed.
	TokensFreed int
	// BoundaryMessage is an optional marker message to insert at the snip point.
	BoundaryMessage *types.Message
}

// snipKeepRecentTurns is the number of most-recent assistant/user turn pairs
// that are never pruned by the snip strategy.
const snipKeepRecentTurns = 5

// SnipCompactIfNeeded removes old tool_use/tool_result pairs from the message
// history when the list is long enough to benefit from pruning.
//
// Strategy:
//   - Identify the earliest snip-eligible pair: a tool_use assistant block
//     followed by the corresponding tool_result user block.
//   - Remove the tool_use input and the tool_result content, replacing them
//     with brief placeholder text.
//   - Preserve all assistant text blocks and the most-recent snipKeepRecentTurns
//     full turn pairs.
//
// The operation is purely local (no LLM call required).
func SnipCompactIfNeeded(messages []types.Message) SnipResult {
	if !needsSnip(messages) {
		return SnipResult{Messages: messages}
	}

	snipped, freed, boundary := snipMessages(messages)
	return SnipResult{
		Messages:        snipped,
		TokensFreed:     freed,
		BoundaryMessage: boundary,
	}
}

// needsSnip returns true when the message list is long enough to be worth snipping.
// Threshold: more than (snipKeepRecentTurns * 2 + 2) messages.
func needsSnip(messages []types.Message) bool {
	return len(messages) > snipKeepRecentTurns*2+2
}

// snipMessages iterates through the message list and removes tool_use inputs
// and tool_result contents from early turns, returning the modified list,
// the estimated tokens freed, and an optional boundary marker message.
func snipMessages(messages []types.Message) ([]types.Message, int, *types.Message) {
	if len(messages) == 0 {
		return messages, 0, nil
	}

	// Determine how many messages to protect from the tail.
	protectedFromIdx := len(messages) - snipKeepRecentTurns*2
	if protectedFromIdx < 0 {
		protectedFromIdx = 0
	}

	result := make([]types.Message, len(messages))
	freed := 0

	for i, msg := range messages {
		if i >= protectedFromIdx {
			result[i] = msg
			continue
		}
		newContent := make([]types.ContentBlock, len(msg.Content))
		for j, blk := range msg.Content {
			switch blk.Type {
			case types.ContentTypeToolUse:
				// Remove input from tool_use blocks.
				if blk.Input != nil {
					freedBytes := estimateMapBytes(blk.Input)
					freed += freedBytes / 4 // chars → tokens estimate
					blk.Input = nil
				}
				newContent[j] = blk
			case types.ContentTypeToolResult:
				// Remove content from tool_result blocks.
				for _, c := range blk.Content {
					if c.Text != nil {
						freed += len(*c.Text) / 4
					}
				}
				placeholder := "[snipped]"
				blk.Content = []types.ContentBlock{
					{Type: types.ContentTypeText, Text: &placeholder},
				}
				newContent[j] = blk
			default:
				newContent[j] = blk
			}
		}
		msg.Content = newContent
		result[i] = msg
	}

	// Build a boundary marker message if any tokens were freed.
	var boundary *types.Message
	if freed > 0 && protectedFromIdx > 0 {
		notice := "[Earlier tool call inputs and outputs have been removed to reduce context length.]"
		m := types.Message{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: &notice},
			},
		}
		boundary = &m
	}

	return result, freed, boundary
}

// estimateMapBytes returns a rough byte estimate for a map[string]any by
// counting key + value string lengths.
func estimateMapBytes(m map[string]any) int {
	total := 0
	for k, v := range m {
		total += len(k)
		if s, ok := v.(string); ok {
			total += len(s)
		} else {
			total += 8 // rough estimate for non-string values
		}
	}
	return total
}
