package compact

import (
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/pkg/types"
)

// helpers

func textMsg(role types.Role, text string) types.Message {
	return types.Message{
		Role:    role,
		Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: &text}},
	}
}

func toolUseMsg(id, name string, input map[string]any) types.Message {
	return types.Message{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.ContentTypeToolUse, ID: &id, Name: &name, Input: input},
		},
	}
}

func toolResultMsg(toolUseID, text string) types.Message {
	return types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{
				Type:      types.ContentTypeToolResult,
				ToolUseID: &toolUseID,
				Content:   []types.ContentBlock{{Type: types.ContentTypeText, Text: &text}},
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────
// needsSnip tests
// ─────────────────────────────────────────────────────────────────

func TestNeedsSnip_Short(t *testing.T) {
	msgs := make([]types.Message, snipKeepRecentTurns*2+2)
	if needsSnip(msgs) {
		t.Error("expected false for threshold boundary")
	}
}

func TestNeedsSnip_Long(t *testing.T) {
	msgs := make([]types.Message, snipKeepRecentTurns*2+3)
	if !needsSnip(msgs) {
		t.Error("expected true for list above threshold")
	}
}

// ─────────────────────────────────────────────────────────────────
// SnipCompactIfNeeded tests
// ─────────────────────────────────────────────────────────────────

func TestSnipCompactIfNeeded_NoSnipNeeded(t *testing.T) {
	msgs := []types.Message{
		textMsg(types.RoleUser, "hello"),
		textMsg(types.RoleAssistant, "world"),
	}
	result := SnipCompactIfNeeded(msgs)
	if result.TokensFreed != 0 {
		t.Errorf("expected 0 tokens freed, got %d", result.TokensFreed)
	}
	if result.BoundaryMessage != nil {
		t.Error("expected no boundary message")
	}
	if len(result.Messages) != len(msgs) {
		t.Errorf("expected same message count, got %d vs %d", len(result.Messages), len(msgs))
	}
}

func TestSnipCompactIfNeeded_SnipsEarlyToolCalls(t *testing.T) {
	// Build a history long enough to trigger snipping.
	// Threshold: snipKeepRecentTurns*2+2 = 12; we need ≥13.
	var msgs []types.Message
	for i := 0; i < 7; i++ {
		msgs = append(msgs, toolUseMsg("id1", "Read", map[string]any{"path": "/file.txt"}))
		msgs = append(msgs, toolResultMsg("id1", "big tool output content here"))
	}
	// 14 messages — should trigger.

	result := SnipCompactIfNeeded(msgs)

	if result.TokensFreed <= 0 {
		t.Errorf("expected tokens freed > 0, got %d", result.TokensFreed)
	}
	if result.BoundaryMessage == nil {
		t.Error("expected boundary message")
	}
	if len(result.Messages) != len(msgs) {
		t.Errorf("message count changed: want %d, got %d", len(msgs), len(result.Messages))
	}

	// The protected tail (last snipKeepRecentTurns*2 = 10 messages) should be unchanged.
	protectedFrom := len(msgs) - snipKeepRecentTurns*2
	for i := protectedFrom; i < len(msgs); i++ {
		orig := msgs[i]
		got := result.Messages[i]
		if len(got.Content) != len(orig.Content) {
			t.Errorf("protected message %d content changed", i)
		}
	}

	// The early tool_result blocks should have been replaced with [snipped].
	for i := 0; i < protectedFrom; i++ {
		msg := result.Messages[i]
		for _, blk := range msg.Content {
			if blk.Type == types.ContentTypeToolResult {
				if len(blk.Content) != 1 {
					t.Errorf("msg %d: expected 1 placeholder content block", i)
					continue
				}
				if blk.Content[0].Text == nil || *blk.Content[0].Text != "[snipped]" {
					t.Errorf("msg %d: expected [snipped], got %v", i, blk.Content[0].Text)
				}
			}
		}
	}
}

func TestSnipCompactIfNeeded_ToolUseInputCleared(t *testing.T) {
	var msgs []types.Message
	for i := 0; i < 7; i++ {
		msgs = append(msgs, toolUseMsg("id1", "Read", map[string]any{"path": "/file.txt"}))
		msgs = append(msgs, toolResultMsg("id1", "output"))
	}
	result := SnipCompactIfNeeded(msgs)

	protectedFrom := len(msgs) - snipKeepRecentTurns*2
	for i := 0; i < protectedFrom; i++ {
		msg := result.Messages[i]
		for _, blk := range msg.Content {
			if blk.Type == types.ContentTypeToolUse && blk.Input != nil {
				t.Errorf("msg %d: expected Input=nil after snip, got %v", i, blk.Input)
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// estimateMapBytes
// ─────────────────────────────────────────────────────────────────

func TestEstimateMapBytes(t *testing.T) {
	m := map[string]any{
		"path":    "/some/file.go",
		"offset":  42,
		"content": "hello world",
	}
	total := estimateMapBytes(m)
	if total <= 0 {
		t.Error("expected positive byte estimate")
	}
}

// ─────────────────────────────────────────────────────────────────
// MicroCompactor tests
// ─────────────────────────────────────────────────────────────────

func TestMicroCompactor_NeedsCompaction_Small(t *testing.T) {
	m := NewMicroCompactor()
	msgs := []types.Message{
		toolResultMsg("id1", "small result"),
	}
	if m.NeedsCompaction(msgs, "", CompactionExtra{}) {
		t.Error("expected false for small tool result")
	}
}

func TestMicroCompactor_NeedsCompaction_Large(t *testing.T) {
	m := NewMicroCompactor()
	large := strings.Repeat("x", microCompactThreshold+1)
	msgs := []types.Message{
		toolResultMsg("id1", large),
	}
	if !m.NeedsCompaction(msgs, "", CompactionExtra{}) {
		t.Error("expected true for large tool result")
	}
}

func TestMicroCompactor_Compact_ReplacesLargeResult(t *testing.T) {
	m := NewMicroCompactor()
	large := strings.Repeat("x", microCompactThreshold+1)
	msgs := []types.Message{
		toolResultMsg("id1", large),
	}
	result := m.compact(msgs)
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	blk := result.Messages[0].Content[0]
	if blk.Type != types.ContentTypeToolResult {
		t.Fatalf("expected tool_result block, got %q", blk.Type)
	}
	if len(blk.Content) != 1 || blk.Content[0].Text == nil {
		t.Fatal("expected 1 text content block")
	}
	if !strings.Contains(*blk.Content[0].Text, "truncated") {
		t.Errorf("expected truncation notice, got %q", *blk.Content[0].Text)
	}
	// Should be much smaller now.
	if toolResultSize(blk) >= microCompactThreshold {
		t.Error("compacted result is still too large")
	}
}

func TestMicroCompactor_Compact_PreservesSmallResult(t *testing.T) {
	m := NewMicroCompactor()
	small := "tiny result"
	msgs := []types.Message{
		toolResultMsg("id1", small),
	}
	result := m.compact(msgs)
	blk := result.Messages[0].Content[0]
	if blk.Content[0].Text == nil || *blk.Content[0].Text != small {
		t.Errorf("small result should be unchanged, got %v", blk.Content[0].Text)
	}
}

func TestMicroCompactor_Compact_PreservesAssistantMessages(t *testing.T) {
	m := NewMicroCompactor()
	large := strings.Repeat("y", microCompactThreshold+1)
	msgs := []types.Message{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{
					Type:      types.ContentTypeToolResult,
					ToolUseID: strPtr("id1"),
					Content:   []types.ContentBlock{{Type: types.ContentTypeText, Text: &large}},
				},
			},
		},
	}
	// Assistant messages with tool_result-shaped blocks should NOT be compacted
	// (MicroCompactor only touches user messages).
	result := m.compact(msgs)
	blk := result.Messages[0].Content[0]
	if blk.Content[0].Text == nil || *blk.Content[0].Text != large {
		t.Error("assistant message content should be unchanged")
	}
}

// ─────────────────────────────────────────────────────────────────
// AutoCompactor threshold tests (no LLM call)
// ─────────────────────────────────────────────────────────────────

func TestAutoCompactThreshold_Default(t *testing.T) {
	threshold := autoCompactThreshold("claude-3-opus-20240229", 8192)
	// effectiveContextWindow = 200_000 - min(8192, 20_000) = 200_000 - 8_192 = 191_808
	// threshold = 191_808 - 13_000 = 178_808
	expected := 200_000 - 8192 - AutoCompactBufferTokens
	if threshold != expected {
		t.Errorf("expected threshold %d, got %d", expected, threshold)
	}
}

func TestAutoCompactThreshold_MaxOutputCap(t *testing.T) {
	// When maxOutputTokens > MaxOutputTokensForSummary, it is capped.
	threshold := autoCompactThreshold("claude-3-opus-20240229", 100_000)
	expected := 200_000 - MaxOutputTokensForSummary - AutoCompactBufferTokens
	if threshold != expected {
		t.Errorf("expected threshold %d (capped), got %d", expected, threshold)
	}
}

func TestResolveContextWindow_KnownModels(t *testing.T) {
	cases := []struct {
		model string
		want  int
	}{
		{"claude-3-5-sonnet-20241022", 200_000},
		{"claude-3.5-haiku-20241022", 200_000},
		{"claude-3-opus-20240229", 200_000},
		{"claude-opus-4-0", 200_000},
		{"claude-2.1", 100_000},
		{"unknown-model-xyz", defaultContextWindow},
	}
	for _, tc := range cases {
		got := resolveContextWindow(tc.model)
		if got != tc.want {
			t.Errorf("resolveContextWindow(%q) = %d, want %d", tc.model, got, tc.want)
		}
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	// nil marshals to "null" which is 4 chars / 4 = 1 token
	// — any non-negative value is fine; just ensure no panic.
	_ = estimateTokens(nil)
}

func TestEstimateTokens_NonEmpty(t *testing.T) {
	msgs := []types.Message{
		textMsg(types.RoleUser, "Hello, world!"),
	}
	est := estimateTokens(msgs)
	if est <= 0 {
		t.Errorf("expected positive token estimate, got %d", est)
	}
}

func TestAutoCompactor_NeedsCompaction_BelowThreshold(t *testing.T) {
	a := NewAutoCompactor(nil, "claude-3-opus-20240229", 8192)
	// A single short message is well below the threshold.
	msgs := []types.Message{textMsg(types.RoleUser, "hi")}
	if a.NeedsCompaction(msgs, "claude-3-opus-20240229", CompactionExtra{}) {
		t.Error("expected false for small context")
	}
}

func TestAutoCompactor_NeedsCompaction_CircuitBreaker(t *testing.T) {
	a := NewAutoCompactor(nil, "claude-3-opus-20240229", 8192)
	a.tracking.ConsecutiveFailures = MaxConsecutiveFailures
	// Even with a massive context, circuit breaker disables compaction.
	large := strings.Repeat("x", 1_000_000)
	msgs := []types.Message{textMsg(types.RoleUser, large)}
	if a.NeedsCompaction(msgs, "claude-3-opus-20240229", CompactionExtra{}) {
		t.Error("expected false when circuit breaker is open")
	}
}

// compile-time assertion: MicroCompactor must satisfy the Compressor interface.
var _ Compressor = (*MicroCompactor)(nil)
