// Package api provides OpenAI API type definitions.
package api

import "encoding/json"

// OpenAI Chat Completions API request types

// openaiChatCompletionRequest is the request body for OpenAI Chat Completions API.
type openaiChatCompletionRequest struct {
	Model            string          `json:"model"`
	Messages         []openaiMessage `json:"messages"`
	MaxTokens        int             `json:"max_tokens,omitempty"`
	Temperature      float64         `json:"temperature,omitempty"`
	TopP             float64         `json:"top_p,omitempty"`
	N                int             `json:"n,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	PresencePenalty  float64         `json:"presence_penalty,omitempty"`
	FrequencyPenalty float64         `json:"frequency_penalty,omitempty"`
	Tools            []openaiTool    `json:"tools,omitempty"`
	ToolChoice       interface{}     `json:"tool_choice,omitempty"`
	User             string          `json:"user,omitempty"`
	// Stream options for getting usage in stream mode
	StreamOptions *openaiStreamOptions `json:"stream_options,omitempty"`
}

// openaiStreamOptions contains options for streaming requests.
type openaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// openaiMessage represents a message in the conversation.
// Content uses *string so that we can distinguish between absent and empty:
//   - nil  → field omitted (e.g., pure tool messages)
//   - ""   → field present as "content":"" (needed for assistant + tool_calls)
type openaiMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content,omitempty"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// openaiTool represents a tool definition.
type openaiTool struct {
	Type     string         `json:"type"` // always "function"
	Function openaiFunction `json:"function"`
}

// openaiFunction describes a function for tool calling.
type openaiFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// openaiToolCall represents a tool call made by the model.
type openaiToolCall struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // always "function"
	Function openaiToolCallFunction `json:"function"`
}

// openaiToolCallFunction contains the function name and arguments.
type openaiToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// OpenAI Chat Completions API response types

// openaiChatCompletionResponse is the response body for OpenAI Chat Completions API.
type openaiChatCompletionResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	Choices           []openaiChoice `json:"choices"`
	Usage             openaiUsage    `json:"usage"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
}

// openaiChoice represents a completion choice.
type openaiChoice struct {
	Index        int                   `json:"index"`
	Message      openaiResponseMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

// openaiResponseMessage is the assistant's response message.
type openaiResponseMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
}

// openaiUsage contains token usage information.
type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAI Streaming types

// openaiStreamChunk represents a chunk in the streaming response.
type openaiStreamChunk struct {
	ID                string               `json:"id"`
	Object            string               `json:"object"`
	Created           int64                `json:"created"`
	Model             string               `json:"model"`
	Choices           []openaiStreamChoice `json:"choices"`
	Usage             *openaiUsage         `json:"usage,omitempty"`
	SystemFingerprint string               `json:"system_fingerprint,omitempty"`
}

// openaiStreamChoice represents a choice in a streaming chunk.
type openaiStreamChoice struct {
	Index        int               `json:"index"`
	Delta        openaiStreamDelta `json:"delta"`
	FinishReason string            `json:"finish_reason,omitempty"`
}

// openaiStreamDelta represents the delta content in a streaming chunk.
type openaiStreamDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
}
