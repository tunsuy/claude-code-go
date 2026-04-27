package memdir

import (
	"context"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/engine"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// --- hasMemoryWritesSince tests ---

func TestHasMemoryWritesSince_NoWrites(t *testing.T) {
	t.Parallel()

	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Hello")}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{textBlock("Hi there!")}},
	}

	if hasMemoryWritesSince(messages) {
		t.Error("expected false when no MemoryWrite in recent messages")
	}
}

func TestHasMemoryWritesSince_HasWrites(t *testing.T) {
	t.Parallel()

	memWriteName := "MemoryWrite"
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Remember this")}},
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{
					Type: types.ContentTypeToolUse,
					Name: &memWriteName,
					Input: map[string]any{
						"title":   "Test",
						"content": "Some memory",
					},
				},
			},
		},
	}

	if !hasMemoryWritesSince(messages) {
		t.Error("expected true when MemoryWrite found in last turn")
	}
}

func TestHasMemoryWritesSince_PreviousTurnOnly(t *testing.T) {
	t.Parallel()

	memWriteName := "MemoryWrite"
	messages := []types.Message{
		// Previous turn: assistant used MemoryWrite.
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("First question")}},
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{
					Type: types.ContentTypeToolUse,
					Name: &memWriteName,
					Input: map[string]any{
						"title":   "Old memory",
						"content": "Old content",
					},
				},
			},
		},
		// Current turn: no MemoryWrite.
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Second question")}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{textBlock("Answer")}},
	}

	if hasMemoryWritesSince(messages) {
		t.Error("expected false when MemoryWrite is only in a previous turn")
	}
}

func TestHasMemoryWritesSince_EmptyMessages(t *testing.T) {
	t.Parallel()

	if hasMemoryWritesSince(nil) {
		t.Error("expected false for nil messages")
	}

	if hasMemoryWritesSince([]types.Message{}) {
		t.Error("expected false for empty messages slice")
	}
}

func TestHasMemoryWritesSince_OtherToolUse(t *testing.T) {
	t.Parallel()

	fileReadName := "FileRead"
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Read the file")}},
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{
					Type:  types.ContentTypeToolUse,
					Name:  &fileReadName,
					Input: map[string]any{"path": "/tmp/test.go"},
				},
			},
		},
	}

	if hasMemoryWritesSince(messages) {
		t.Error("expected false when only non-MemoryWrite tool use is present")
	}
}

// --- buildConversationSummary tests ---

func TestBuildConversationSummary_FormatsMessages(t *testing.T) {
	t.Parallel()

	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("How do I write tests?")}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{textBlock("Use table-driven tests.")}},
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Thanks!")}},
	}

	summary := buildConversationSummary(messages, 10)

	if !strings.Contains(summary, "[User]: How do I write tests?") {
		t.Errorf("expected user message in summary, got:\n%s", summary)
	}
	if !strings.Contains(summary, "[Assistant]: Use table-driven tests.") {
		t.Errorf("expected assistant message in summary, got:\n%s", summary)
	}
	if !strings.Contains(summary, "[User]: Thanks!") {
		t.Errorf("expected second user message in summary, got:\n%s", summary)
	}
}

func TestBuildConversationSummary_RespectsMaxMessages(t *testing.T) {
	t.Parallel()

	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Message 1")}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{textBlock("Reply 1")}},
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Message 2")}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{textBlock("Reply 2")}},
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Message 3")}},
	}

	// Only include last 2 messages.
	summary := buildConversationSummary(messages, 2)

	if strings.Contains(summary, "Message 1") {
		t.Error("should not contain Message 1 when maxMessages=2")
	}
	if !strings.Contains(summary, "Message 3") {
		t.Errorf("should contain Message 3 (most recent), got:\n%s", summary)
	}
}

func TestBuildConversationSummary_Truncation(t *testing.T) {
	t.Parallel()

	// Create a message with very long text that exceeds maxSummaryChars.
	longText := strings.Repeat("This is a long message. ", 500) // ~12,000 chars
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock(longText)}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{textBlock("Short reply.")}},
	}

	summary := buildConversationSummary(messages, 10)

	// Summary should be truncated to approximately maxSummaryChars.
	if len(summary) > maxSummaryChars+100 {
		t.Errorf("summary length %d exceeds limit %d by too much", len(summary), maxSummaryChars)
	}
}

func TestBuildConversationSummary_EmptyMessages(t *testing.T) {
	t.Parallel()

	summary := buildConversationSummary(nil, 10)
	if summary != "" {
		t.Errorf("expected empty summary for nil messages, got %q", summary)
	}

	summary = buildConversationSummary([]types.Message{}, 10)
	if summary != "" {
		t.Errorf("expected empty summary for empty messages, got %q", summary)
	}
}

func TestBuildConversationSummary_SkipsNonTextBlocks(t *testing.T) {
	t.Parallel()

	toolName := "FileRead"
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Read the file")}},
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{
					Type:  types.ContentTypeToolUse,
					Name:  &toolName,
					Input: map[string]any{"path": "/tmp/test.go"},
				},
			},
		},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{textBlock("Here is the content.")}},
	}

	summary := buildConversationSummary(messages, 10)

	// The tool_use block should not appear as text.
	if strings.Contains(summary, "FileRead") {
		t.Error("summary should not contain tool_use block names")
	}
	if !strings.Contains(summary, "Here is the content.") {
		t.Errorf("summary should contain text from assistant, got:\n%s", summary)
	}
}

// --- BuildExtractionPrompt tests ---

func TestBuildExtractionPrompt_ContainsSummary(t *testing.T) {
	t.Parallel()

	summary := "[User]: I prefer tabs over spaces.\n\n[Assistant]: Got it, noted."
	prompt := BuildExtractionPrompt(summary)

	if !strings.Contains(prompt, summary) {
		t.Error("prompt should contain the conversation summary")
	}
	if !strings.Contains(prompt, "## Conversation") {
		t.Error("prompt should contain the Conversation header")
	}
	if !strings.Contains(prompt, "nothing worth remembering") {
		t.Error("prompt should contain the guard instruction")
	}
}

func TestBuildExtractionPrompt_EmptySummary(t *testing.T) {
	t.Parallel()

	prompt := BuildExtractionPrompt("")

	if !strings.Contains(prompt, "## Conversation") {
		t.Error("prompt should still contain structure even with empty summary")
	}
}

// --- ExecuteExtractMemories guard tests ---
//
// These tests verify the early-return guards in ExecuteExtractMemories.
// They use minimal StopHookContext values with nil Engine/CacheParams,
// so if a guard fails to short-circuit, the forked agent call will panic —
// ensuring the guard was effective.

func TestExecuteExtractMemories_Disabled(t *testing.T) {
	t.Parallel()

	cfg := ExtractMemoriesConfig{
		Enabled: false,
	}
	hookCtx := &engine.StopHookContext{
		QuerySource: foregroundQuerySource,
	}
	// Should return immediately — Engine is nil, so reaching RunForkedAgent would panic.
	ExecuteExtractMemories(context.Background(), hookCtx, cfg)
}

func TestExecuteExtractMemories_NilStore(t *testing.T) {
	t.Parallel()

	cfg := ExtractMemoriesConfig{
		Enabled: true,
		Store:   nil,
	}
	hookCtx := &engine.StopHookContext{
		QuerySource: foregroundQuerySource,
	}
	ExecuteExtractMemories(context.Background(), hookCtx, cfg)
}

func TestExecuteExtractMemories_BareMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewMemoryStoreWithPath(dir)

	cfg := ExtractMemoriesConfig{
		Enabled: true,
		Store:   store,
	}
	hookCtx := &engine.StopHookContext{
		IsBareMode:  true,
		QuerySource: foregroundQuerySource,
	}
	// Should skip in bare mode — Engine is nil, so reaching RunForkedAgent would panic.
	ExecuteExtractMemories(context.Background(), hookCtx, cfg)
}

func TestExecuteExtractMemories_NonForeground(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewMemoryStoreWithPath(dir)

	cfg := ExtractMemoriesConfig{
		Enabled: true,
		Store:   store,
	}
	hookCtx := &engine.StopHookContext{
		IsBareMode:  false,
		QuerySource: "sub_agent",
	}
	// Should skip for non-foreground queries — Engine is nil.
	ExecuteExtractMemories(context.Background(), hookCtx, cfg)
}

func TestExecuteExtractMemories_SkipsWhenMemoryWritten(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewMemoryStoreWithPath(dir)

	memWriteName := "MemoryWrite"
	hookCtx := &engine.StopHookContext{
		IsBareMode:  false,
		QuerySource: foregroundQuerySource,
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{textBlock("Remember this")}},
			{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{
					{
						Type: types.ContentTypeToolUse,
						Name: &memWriteName,
						Input: map[string]any{
							"title":   "Existing",
							"content": "Already saved",
						},
					},
				},
			},
		},
	}

	cfg := ExtractMemoriesConfig{
		Enabled: true,
		Store:   store,
	}
	// Should skip because main agent already wrote a memory.
	// Engine is nil, so if it tried to run the forked agent it would panic.
	ExecuteExtractMemories(context.Background(), hookCtx, cfg)
}

// --- DefaultExtractConfig test ---

func TestDefaultExtractConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultExtractConfig()
	if cfg.MaxTurns != 3 {
		t.Errorf("MaxTurns: got %d, want 3", cfg.MaxTurns)
	}
	if !cfg.Enabled {
		t.Error("Enabled should be true by default")
	}
	if cfg.Store != nil {
		t.Error("Store should be nil by default")
	}
}

// --- helpers ---

// textBlock creates a text ContentBlock for test convenience.
func textBlock(text string) types.ContentBlock {
	return types.ContentBlock{
		Type: types.ContentTypeText,
		Text: strPtr(text),
	}
}
