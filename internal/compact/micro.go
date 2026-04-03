package compact

import (
	"github.com/anthropics/claude-code-go/pkg/types"
)

// MicroCompactionInfo carries metadata about a micro-compact operation.
type MicroCompactionInfo struct {
	// PendingCacheEdits is non-nil in cached microcompact mode.
	PendingCacheEdits []CacheEdit
}

// CacheEdit describes a single cached-microcompact substitution.
type CacheEdit struct {
	// ToolUseID is the tool_use block that was summarised.
	ToolUseID string
	// Summary is the replacement text for the tool result.
	Summary string
}

// MicroCompactResult is the output of a MicroCompactor.Compact call.
type MicroCompactResult struct {
	// Messages is the message list after micro-compaction.
	Messages []types.Message
	// CompactionInfo carries cache-edit metadata (nil in non-cached mode).
	CompactionInfo *MicroCompactionInfo
}

// microCompactThreshold is the approximate character count beyond which a
// single tool_result content block is summarised.
// Corresponds to TS microcompact threshold (~8 000 tokens × 4 chars/token).
const microCompactThreshold = 32_000

// MicroCompactor compresses oversized tool results in-place.
//
// For each tool_result content block whose serialised text exceeds
// microCompactThreshold, the block's text content is replaced with a
// truncation notice. This is a best-effort, locally-computed operation that
// requires no LLM call.
type MicroCompactor struct{}

// NewMicroCompactor constructs a MicroCompactor.
func NewMicroCompactor() *MicroCompactor {
	return &MicroCompactor{}
}

// NeedsCompaction reports whether any tool_result in the message list exceeds
// the micro-compact threshold.
func (m *MicroCompactor) NeedsCompaction(messages []types.Message, _ string, _ CompactionExtra) bool {
	for _, msg := range messages {
		for _, blk := range msg.Content {
			if blk.Type == types.ContentTypeToolResult {
				if toolResultSize(blk) > microCompactThreshold {
					return true
				}
			}
		}
	}
	return false
}

// Compact replaces oversized tool_result content with a truncation notice.
// It returns a shallow-copy of the message list with affected blocks replaced.
func (m *MicroCompactor) Compact(messages []types.Message) MicroCompactResult {
	result := make([]types.Message, len(messages))
	for i, msg := range messages {
		if msg.Role != types.RoleUser {
			result[i] = msg
			continue
		}
		newContent := make([]types.ContentBlock, len(msg.Content))
		modified := false
		for j, blk := range msg.Content {
			if blk.Type == types.ContentTypeToolResult && toolResultSize(blk) > microCompactThreshold {
				blk = truncateToolResult(blk)
				modified = true
			}
			newContent[j] = blk
		}
		if modified {
			msg.Content = newContent
		}
		result[i] = msg
	}
	return MicroCompactResult{Messages: result}
}

// toolResultSize returns the approximate character count of a tool_result block.
func toolResultSize(blk types.ContentBlock) int {
	total := 0
	for _, c := range blk.Content {
		if c.Text != nil {
			total += len(*c.Text)
		}
	}
	return total
}

// truncateToolResult replaces the text content of a tool_result with a
// truncation notice, preserving the ToolUseID and IsError fields.
func truncateToolResult(blk types.ContentBlock) types.ContentBlock {
	notice := "[Tool result was too large and has been truncated. The original output exceeded the context limit.]"
	blk.Content = []types.ContentBlock{
		{Type: types.ContentTypeText, Text: &notice},
	}
	return blk
}
