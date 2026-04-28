package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// cacheParamsFilename is the default filename for persisted cache parameters.
const cacheParamsFilename = "cache-params.json"

// CreateCacheSafeParams creates cache parameters from the current query context.
// It extracts the system prompt and messages from params and returns a CacheSafeParams
// that can be shared with forked agents for Prompt Cache reuse.
func CreateCacheSafeParams(params QueryParams, messages []types.Message) *CacheSafeParams {
	// Copy messages to avoid aliasing.
	contextMessages := make([]types.Message, len(messages))
	copy(contextMessages, messages)

	return &CacheSafeParams{
		SystemPrompt:    params.SystemPrompt,
		ContextMessages: contextMessages,
		ToolUseContext:  params.ToolUseContext,
	}
}

// cacheParamsJSON is the on-disk representation of CacheSafeParams.
// ToolUseContext is excluded from serialization as it contains runtime-only data
// (context.Context, channels) that cannot be meaningfully persisted.
type cacheParamsJSON struct {
	SystemPrompt    SystemPrompt    `json:"system_prompt"`
	ContextMessages []types.Message `json:"context_messages"`
}

// SaveCacheSafeParams persists cache parameters to ~/.claude/cache-params.json.
// Only the serializable fields (SystemPrompt, ContextMessages) are written;
// ToolUseContext must be reconstructed at load time.
func SaveCacheSafeParams(params *CacheSafeParams) error {
	if params == nil {
		return fmt.Errorf("engine: save cache params: params must not be nil")
	}

	dir, err := cacheParamsDir()
	if err != nil {
		return fmt.Errorf("engine: save cache params: %w", err)
	}

	// Ensure the directory exists.
	if mkErr := os.MkdirAll(dir, 0700); mkErr != nil {
		return fmt.Errorf("engine: save cache params: mkdir: %w", mkErr)
	}

	data := cacheParamsJSON{
		SystemPrompt:    params.SystemPrompt,
		ContextMessages: params.ContextMessages,
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("engine: save cache params: marshal: %w", err)
	}

	path := filepath.Join(dir, cacheParamsFilename)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return fmt.Errorf("engine: save cache params: write: %w", err)
	}

	return nil
}

// LoadCacheSafeParams loads cache parameters from ~/.claude/cache-params.json.
// The returned CacheSafeParams will have ToolUseContext set to nil; callers must
// populate it before passing to RunForkedAgent.
func LoadCacheSafeParams() (*CacheSafeParams, error) {
	dir, err := cacheParamsDir()
	if err != nil {
		return nil, fmt.Errorf("engine: load cache params: %w", err)
	}

	path := filepath.Join(dir, cacheParamsFilename)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("engine: load cache params: read: %w", err)
	}

	var data cacheParamsJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("engine: load cache params: unmarshal: %w", err)
	}

	return &CacheSafeParams{
		SystemPrompt:    data.SystemPrompt,
		ContextMessages: data.ContextMessages,
		ToolUseContext:  nil, // Must be populated by caller.
	}, nil
}

// cacheParamsDir returns the directory path for cache parameter storage (~/.claude).
func cacheParamsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// SaveCacheSafeParamsTo persists cache parameters to a specified file path.
// This is primarily useful for testing or custom storage locations.
func SaveCacheSafeParamsTo(params *CacheSafeParams, filePath string) error {
	if params == nil {
		return fmt.Errorf("engine: save cache params: params must not be nil")
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("engine: save cache params: mkdir: %w", err)
	}

	data := cacheParamsJSON{
		SystemPrompt:    params.SystemPrompt,
		ContextMessages: params.ContextMessages,
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("engine: save cache params: marshal: %w", err)
	}

	if err := os.WriteFile(filePath, raw, 0600); err != nil {
		return fmt.Errorf("engine: save cache params: write: %w", err)
	}
	return nil
}

// LoadCacheSafeParamsFrom loads cache parameters from a specified file path.
// This is primarily useful for testing or custom storage locations.
func LoadCacheSafeParamsFrom(filePath string) (*CacheSafeParams, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("engine: load cache params: not found: %w", err)
		}
		return nil, fmt.Errorf("engine: load cache params: read: %w", err)
	}

	var data cacheParamsJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("engine: load cache params: unmarshal: %w", err)
	}

	return &CacheSafeParams{
		SystemPrompt:    data.SystemPrompt,
		ContextMessages: data.ContextMessages,
		ToolUseContext:  nil, // Must be populated by caller.
	}, nil
}

// WithToolUseContext returns a shallow copy of the CacheSafeParams with the
// ToolUseContext field set. This is a convenience for chaining after LoadCacheSafeParams.
func (p *CacheSafeParams) WithToolUseContext(ctx *tools.UseContext) *CacheSafeParams {
	return &CacheSafeParams{
		SystemPrompt:    p.SystemPrompt,
		ContextMessages: p.ContextMessages,
		ToolUseContext:  ctx,
	}
}
