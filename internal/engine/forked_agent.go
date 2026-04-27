package engine

import (
	"context"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// CacheSafeParams stores Prompt Cache parameters that can be shared with forked agents.
// By reusing the same system prompt and context messages, forked agents benefit from
// Anthropic's Prompt Cache, significantly reducing latency and cost.
type CacheSafeParams struct {
	// SystemPrompt is the rendered system prompt shared with the parent.
	SystemPrompt SystemPrompt
	// ContextMessages are the initial context messages (e.g. CLAUDE.md content).
	ContextMessages []types.Message
	// ToolUseContext is the per-call tool execution context.
	ToolUseContext *tools.UseContext
}

// ForkedAgentConfig configures a background forked agent.
// A forked agent is a "shadow clone" that runs independently from the parent engine:
// it shares the parent's system prompt for Prompt Cache hits but does NOT write
// messages back to the parent conversation.
type ForkedAgentConfig struct {
	// PromptMessages are the messages for the forked agent's conversation.
	PromptMessages []types.Message
	// CacheSafeParams are the shared cache parameters from the parent.
	CacheSafeParams *CacheSafeParams
	// QuerySource is the source identifier (e.g. "extract_memories").
	QuerySource string
	// MaxTurns caps the number of agent turns (0 = no limit).
	MaxTurns int
	// AllowedTools is a tool whitelist. If empty, all tools are available.
	AllowedTools []string
	// OnMessage is an optional callback invoked for each message emitted by the fork.
	OnMessage func(types.Message)
}

// RunForkedAgent runs an isolated background agent that shares the parent engine's
// Prompt Cache parameters but does NOT write messages back to the parent conversation.
//
// It builds QueryParams from the CacheSafeParams and PromptMessages, sets
// NoWriteBack=true, and calls eng.Query(). All assistant and user messages emitted
// on the channel are collected and returned.
//
// Context cancellation is fully propagated: if ctx is cancelled, the underlying
// query loop terminates and RunForkedAgent returns the messages collected so far
// along with the context error.
func RunForkedAgent(ctx context.Context, eng QueryEngine, cfg ForkedAgentConfig) ([]types.Message, error) {
	if eng == nil {
		return nil, fmt.Errorf("engine: forked agent: engine must not be nil")
	}
	if cfg.CacheSafeParams == nil {
		return nil, fmt.Errorf("engine: forked agent: CacheSafeParams must not be nil")
	}

	// Build the ExcludeTools map from AllowedTools (inverse mapping).
	excludeTools := buildExcludeTools(eng, cfg.AllowedTools)

	// Combine context messages with prompt messages.
	messages := make([]types.Message, 0, len(cfg.CacheSafeParams.ContextMessages)+len(cfg.PromptMessages))
	messages = append(messages, cfg.CacheSafeParams.ContextMessages...)
	messages = append(messages, cfg.PromptMessages...)

	// Build query params.
	params := QueryParams{
		Messages:       messages,
		SystemPrompt:   cfg.CacheSafeParams.SystemPrompt,
		ToolUseContext: cfg.CacheSafeParams.ToolUseContext,
		QuerySource:    cfg.QuerySource,
		MaxTurns:       cfg.MaxTurns,
		NoWriteBack:    true,
		ExcludeTools:   excludeTools,
	}

	// Start the query.
	msgCh, err := eng.Query(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("engine: forked agent: query: %w", err)
	}

	// Collect messages from the channel.
	var collected []types.Message
	for msg := range msgCh {
		switch msg.Type {
		case MsgTypeAssistantMessage:
			if msg.AssistantMsg != nil {
				collected = append(collected, *msg.AssistantMsg)
				if cfg.OnMessage != nil {
					cfg.OnMessage(*msg.AssistantMsg)
				}
			}
		case MsgTypeUserMessage:
			if msg.UserMsg != nil {
				collected = append(collected, *msg.UserMsg)
				if cfg.OnMessage != nil {
					cfg.OnMessage(*msg.UserMsg)
				}
			}
		case MsgTypeError:
			if msg.Err != nil {
				return collected, fmt.Errorf("engine: forked agent: %w", msg.Err)
			}
		}
	}

	// Check if context was cancelled during collection.
	if ctx.Err() != nil {
		return collected, ctx.Err()
	}

	return collected, nil
}

// buildExcludeTools computes the ExcludeTools map from an AllowedTools whitelist.
// If allowedTools is empty, returns nil (all tools allowed).
// Otherwise, it queries the engine's registry to find all tools NOT in the whitelist.
func buildExcludeTools(eng QueryEngine, allowedTools []string) map[string]bool {
	if len(allowedTools) == 0 {
		return nil
	}

	// Build the allowed set for O(1) lookup.
	allowed := make(map[string]bool, len(allowedTools))
	for _, name := range allowedTools {
		allowed[name] = true
	}

	// We need access to the registry to get all tool names.
	// Use type assertion to access the concrete implementation.
	impl, ok := eng.(*engineImpl)
	if !ok {
		// If we can't access the registry, return nil (allow all).
		return nil
	}

	// Build exclude map from tools NOT in the allowed set.
	allTools := impl.registry.All()
	exclude := make(map[string]bool)
	for _, t := range allTools {
		if !allowed[t.Name()] {
			exclude[t.Name()] = true
		}
	}

	if len(exclude) == 0 {
		return nil
	}
	return exclude
}
