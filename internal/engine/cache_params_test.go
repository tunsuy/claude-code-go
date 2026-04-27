package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

func TestCreateCacheSafeParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		params   QueryParams
		messages []types.Message
	}{
		{
			name: "basic creation",
			params: QueryParams{
				SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{
					{Text: "You are helpful.", CacheControl: "ephemeral"},
				}},
				ToolUseContext: &tools.UseContext{},
			},
			messages: []types.Message{
				{Role: types.RoleUser, Content: []types.ContentBlock{
					{Type: types.ContentTypeText, Text: strPtr("hello")},
				}},
			},
		},
		{
			name: "empty messages",
			params: QueryParams{
				SystemPrompt:   SystemPrompt{},
				ToolUseContext: &tools.UseContext{},
			},
			messages: nil,
		},
		{
			name: "multiple system prompt parts",
			params: QueryParams{
				SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{
					{Text: "Part 1"},
					{Text: "Part 2", CacheControl: "ephemeral"},
				}},
				ToolUseContext: &tools.UseContext{},
			},
			messages: []types.Message{
				{Role: types.RoleUser, Content: []types.ContentBlock{
					{Type: types.ContentTypeText, Text: strPtr("msg1")},
				}},
				{Role: types.RoleAssistant, Content: []types.ContentBlock{
					{Type: types.ContentTypeText, Text: strPtr("msg2")},
				}},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := CreateCacheSafeParams(tc.params, tc.messages)

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			// Verify system prompt is copied.
			if len(result.SystemPrompt.Parts) != len(tc.params.SystemPrompt.Parts) {
				t.Errorf("SystemPrompt parts: want %d, got %d",
					len(tc.params.SystemPrompt.Parts), len(result.SystemPrompt.Parts))
			}

			// Verify messages are copied.
			if len(result.ContextMessages) != len(tc.messages) {
				t.Errorf("ContextMessages: want %d, got %d",
					len(tc.messages), len(result.ContextMessages))
			}

			// Verify ToolUseContext is set.
			if result.ToolUseContext != tc.params.ToolUseContext {
				t.Error("ToolUseContext should be the same reference")
			}
		})
	}
}

func TestCreateCacheSafeParams_MessageIsolation(t *testing.T) {
	t.Parallel()

	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr("original")},
		}},
	}

	params := QueryParams{
		SystemPrompt:   SystemPrompt{},
		ToolUseContext: &tools.UseContext{},
	}

	result := CreateCacheSafeParams(params, messages)

	// Mutate the original slice.
	messages[0].Role = types.RoleAssistant

	// The result should be unaffected.
	if result.ContextMessages[0].Role != types.RoleUser {
		t.Error("CreateCacheSafeParams should copy messages, not reference the slice")
	}
}

func TestSaveCacheSafeParams_RoundTrip(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv — environment mutation is process-wide.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	params := &CacheSafeParams{
		SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{
			{Text: "You are a helpful assistant.", CacheControl: "ephemeral"},
			{Text: "Be concise."},
		}},
		ContextMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("hello")},
			}},
			{Role: types.RoleAssistant, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("hi there")},
			}},
		},
		ToolUseContext: &tools.UseContext{}, // Will not be serialized.
	}

	// Save.
	if err := SaveCacheSafeParams(params); err != nil {
		t.Fatalf("SaveCacheSafeParams failed: %v", err)
	}

	// Verify file exists.
	expectedPath := filepath.Join(tmpDir, ".claude", cacheParamsFilename)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("expected file at %s", expectedPath)
	}

	// Load.
	loaded, err := LoadCacheSafeParams()
	if err != nil {
		t.Fatalf("LoadCacheSafeParams failed: %v", err)
	}

	// Verify system prompt.
	if len(loaded.SystemPrompt.Parts) != 2 {
		t.Fatalf("expected 2 system prompt parts, got %d", len(loaded.SystemPrompt.Parts))
	}
	if loaded.SystemPrompt.Parts[0].Text != "You are a helpful assistant." {
		t.Errorf("part[0].Text: want 'You are a helpful assistant.', got %q",
			loaded.SystemPrompt.Parts[0].Text)
	}
	if loaded.SystemPrompt.Parts[0].CacheControl != "ephemeral" {
		t.Errorf("part[0].CacheControl: want 'ephemeral', got %q",
			loaded.SystemPrompt.Parts[0].CacheControl)
	}
	if loaded.SystemPrompt.Parts[1].Text != "Be concise." {
		t.Errorf("part[1].Text: want 'Be concise.', got %q",
			loaded.SystemPrompt.Parts[1].Text)
	}

	// Verify context messages.
	if len(loaded.ContextMessages) != 2 {
		t.Fatalf("expected 2 context messages, got %d", len(loaded.ContextMessages))
	}
	if loaded.ContextMessages[0].Role != types.RoleUser {
		t.Errorf("msg[0].Role: want user, got %q", loaded.ContextMessages[0].Role)
	}
	if loaded.ContextMessages[1].Role != types.RoleAssistant {
		t.Errorf("msg[1].Role: want assistant, got %q", loaded.ContextMessages[1].Role)
	}

	// ToolUseContext should be nil after loading.
	if loaded.ToolUseContext != nil {
		t.Error("ToolUseContext should be nil after loading from disk")
	}
}

func TestSaveCacheSafeParams_NilParams(t *testing.T) {
	t.Parallel()

	err := SaveCacheSafeParams(nil)
	if err == nil {
		t.Fatal("expected error for nil params")
	}
}

func TestLoadCacheSafeParams_FileNotFound(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_, err := LoadCacheSafeParams()
	if err == nil {
		t.Fatal("expected error when file does not exist")
	}
}

func TestLoadCacheSafeParams_InvalidJSON(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create an invalid JSON file.
	dir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, cacheParamsFilename)
	if err := os.WriteFile(path, []byte("invalid json{{{"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadCacheSafeParams()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSaveCacheSafeParams_CreatesDirectory(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// The .claude directory does not exist yet.
	params := &CacheSafeParams{
		SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{{Text: "test"}}},
	}

	if err := SaveCacheSafeParams(params); err != nil {
		t.Fatalf("SaveCacheSafeParams should create the directory: %v", err)
	}

	// Verify directory was created.
	dirPath := filepath.Join(tmpDir, ".claude")
	info, err := os.Stat(dirPath)
	if err != nil {
		t.Fatalf("directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}

func TestSaveCacheSafeParamsTo_RoundTrip(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "subdir", "params.json")

	params := &CacheSafeParams{
		SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{
			{Text: "custom path test", CacheControl: "ephemeral"},
		}},
		ContextMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("msg")},
			}},
		},
	}

	// Save to custom path.
	if err := SaveCacheSafeParamsTo(params, filePath); err != nil {
		t.Fatalf("SaveCacheSafeParamsTo failed: %v", err)
	}

	// Verify file exists (also verifies subdirectory creation).
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("expected file at %s", filePath)
	}

	// Load from custom path.
	loaded, err := LoadCacheSafeParamsFrom(filePath)
	if err != nil {
		t.Fatalf("LoadCacheSafeParamsFrom failed: %v", err)
	}

	if len(loaded.SystemPrompt.Parts) != 1 {
		t.Fatalf("expected 1 system prompt part, got %d", len(loaded.SystemPrompt.Parts))
	}
	if loaded.SystemPrompt.Parts[0].Text != "custom path test" {
		t.Errorf("text: want 'custom path test', got %q", loaded.SystemPrompt.Parts[0].Text)
	}
	if len(loaded.ContextMessages) != 1 {
		t.Fatalf("expected 1 context message, got %d", len(loaded.ContextMessages))
	}
	if loaded.ToolUseContext != nil {
		t.Error("ToolUseContext should be nil after loading")
	}
}

func TestSaveCacheSafeParamsTo_NilParams(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "params.json")

	err := SaveCacheSafeParamsTo(nil, filePath)
	if err == nil {
		t.Fatal("expected error for nil params")
	}
}

func TestLoadCacheSafeParamsFrom_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadCacheSafeParamsFrom("/nonexistent/path/params.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadCacheSafeParamsFrom_InvalidJSON(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(filePath, []byte("not valid json!!!"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadCacheSafeParamsFrom(filePath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestWithToolUseContext(t *testing.T) {
	t.Parallel()

	original := &CacheSafeParams{
		SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{{Text: "sys"}}},
		ContextMessages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{
				{Type: types.ContentTypeText, Text: strPtr("hello")},
			}},
		},
		ToolUseContext: nil,
	}

	uctx := &tools.UseContext{}
	result := original.WithToolUseContext(uctx)

	// Result should have the ToolUseContext set.
	if result.ToolUseContext != uctx {
		t.Error("WithToolUseContext should set the ToolUseContext")
	}

	// Original should remain unchanged.
	if original.ToolUseContext != nil {
		t.Error("original CacheSafeParams should not be modified")
	}

	// SystemPrompt and ContextMessages should be preserved.
	if len(result.SystemPrompt.Parts) != 1 {
		t.Errorf("expected 1 system prompt part, got %d", len(result.SystemPrompt.Parts))
	}
	if len(result.ContextMessages) != 1 {
		t.Errorf("expected 1 context message, got %d", len(result.ContextMessages))
	}
}
