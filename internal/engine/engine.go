package engine

import (
	"context"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/tool"
	"github.com/anthropics/claude-code-go/pkg/types"
)

// QueryEngine is the top-level interface exposed to the TUI/SDK layer.
// It owns the LLM query loop, tool orchestration, and streaming event emission.
type QueryEngine interface {
	// Query starts a complete LLM query cycle, returning a channel of stream events.
	// The channel is closed when the cycle finishes (success or error).
	// Cancellation is via ctx; see also Interrupt for immediate user-initiated abort.
	Query(ctx context.Context, params QueryParams) (<-chan Msg, error)

	// Interrupt signals the currently-running query to stop at the next safe
	// opportunity (equivalent to the user pressing Ctrl+C).
	Interrupt(ctx context.Context)

	// GetMessages returns the full conversation history held by this engine.
	GetMessages() []types.Message

	// SetMessages replaces the conversation history (used after /compact).
	SetMessages(messages []types.Message)
}

// QueryParams encapsulates a single Query call's configuration.
type QueryParams struct {
	// Messages is the initial conversation history for this query.
	Messages []types.Message
	// SystemPrompt is the rendered system prompt.
	SystemPrompt SystemPrompt
	// UserContext holds dynamic user-context key→value pairs injected into the prompt.
	UserContext map[string]string
	// SystemContext holds system-level key→value pairs injected into the prompt.
	SystemContext map[string]string
	// ToolUseContext is the per-call tool execution context (must not be nil).
	ToolUseContext *tool.UseContext
	// QuerySource tags the request for retry policy selection ("foreground" | "background").
	QuerySource string
	// MaxOutputTokensOverride overrides the model default when non-zero.
	MaxOutputTokensOverride int
	// MaxTurns caps the number of agent turns (0 = no limit).
	MaxTurns int
	// SkipCacheWrite disables cache_control write for this request.
	SkipCacheWrite bool
	// TaskBudget configures an optional output task budget.
	TaskBudget *TaskBudget
	// FallbackModel is used if the primary model hits its context limit.
	FallbackModel string
}

// TaskBudget defines an optional task-level output token budget.
type TaskBudget struct {
	Total int
}

// SystemPrompt encapsulates the rendered system prompt, possibly with
// cache_control markers to minimise re-tokenisation costs.
type SystemPrompt struct {
	Parts []SystemPromptPart
}

// SystemPromptPart is one segment of the system prompt.
type SystemPromptPart struct {
	Text         string
	CacheControl string // "ephemeral" | ""
}

// engineImpl is the concrete QueryEngine implementation.
type engineImpl struct {
	client    api.Client
	registry  *tool.Registry
	model     string
	maxTokens int

	// messages is the mutable conversation history; protected by the query loop goroutine.
	messages []types.Message

	// abort controls the current query cycle.
	abortFn  context.CancelFunc
	abortCh  chan struct{}
}

// Config is the constructor parameter bundle for New.
type Config struct {
	// Client is the Anthropic API client (required).
	Client api.Client
	// Registry is the tool registry (required).
	Registry *tool.Registry
	// Model is the default LLM model identifier (e.g. "claude-opus-4-5").
	Model string
	// MaxTokens is the default max_tokens for API requests (0 → 8192).
	MaxTokens int
}

// New constructs and returns a new QueryEngine.
func New(cfg Config) QueryEngine {
	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	return &engineImpl{
		client:    cfg.Client,
		registry:  cfg.Registry,
		model:     cfg.Model,
		maxTokens: maxTokens,
		abortCh:   make(chan struct{}),
	}
}

// GetMessages returns the current conversation history.
func (e *engineImpl) GetMessages() []types.Message {
	result := make([]types.Message, len(e.messages))
	copy(result, e.messages)
	return result
}

// SetMessages replaces the conversation history.
func (e *engineImpl) SetMessages(messages []types.Message) {
	e.messages = make([]types.Message, len(messages))
	copy(e.messages, messages)
}

// Interrupt cancels the currently-running query (if any).
func (e *engineImpl) Interrupt(_ context.Context) {
	if e.abortFn != nil {
		e.abortFn()
	}
}
