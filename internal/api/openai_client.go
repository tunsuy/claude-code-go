// Package api provides the OpenAI-compatible API client implementation.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// openaiClient implements Client using the OpenAI Chat Completions API format.
// This enables compatibility with OpenAI, Azure OpenAI, and other OpenAI-compatible services
// like DeepSeek, Qwen, Moonshot, etc.
type openaiClient struct {
	apiKey       string
	baseURL      string
	httpClient   *http.Client
	headers      map[string]string
	organization string // Optional OpenAI organization ID
	project      string // Optional OpenAI project ID
}

// Compile-time interface assertion: openaiClient must satisfy Client.
var _ Client = (*openaiClient)(nil)

// newOpenAIClient creates a new OpenAI-compatible client.
func newOpenAIClient(cfg ClientConfig, httpClient *http.Client, headers map[string]string) (Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &openaiClient{
		apiKey:       cfg.APIKey,
		baseURL:      baseURL,
		httpClient:   httpClient,
		headers:      headers,
		organization: cfg.OpenAIOrganization,
		project:      cfg.OpenAIProject,
	}, nil
}

// Stream initiates a streaming chat completion request and returns an SSE event stream.
func (c *openaiClient) Stream(ctx context.Context, req *MessageRequest) (StreamReader, error) {
	req.Stream = true
	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &APIError{Kind: ErrKindConnectionError, Message: err.Error()}
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, c.parseHTTPError(resp)
	}
	return newOpenAISSEReader(resp), nil
}

// Complete makes a non-streaming chat completion request and returns the full response.
func (c *openaiClient) Complete(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	req.Stream = false
	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &APIError{Kind: ErrKindConnectionError, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, c.parseHTTPError(resp)
	}
	var openaiResp openaiChatCompletionResponse
	if decErr := json.NewDecoder(resp.Body).Decode(&openaiResp); decErr != nil {
		return nil, fmt.Errorf("api: decode openai response: %w", decErr)
	}
	return c.convertToMessageResponse(&openaiResp), nil
}

// buildRequest constructs the HTTP request for OpenAI API.
func (c *openaiClient) buildRequest(ctx context.Context, req *MessageRequest) (*http.Request, error) {
	openaiReq := c.convertToOpenAIRequest(req)
	body, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("api: marshal openai request: %w", err)
	}
	url := c.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("api: build openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.organization != "" {
		httpReq.Header.Set("OpenAI-Organization", c.organization)
	}
	if c.project != "" {
		httpReq.Header.Set("OpenAI-Project", c.project)
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	return httpReq, nil
}

// convertToOpenAIRequest converts Anthropic MessageRequest to OpenAI ChatCompletionRequest.
func (c *openaiClient) convertToOpenAIRequest(req *MessageRequest) *openaiChatCompletionRequest {
	openaiReq := &openaiChatCompletionRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Stream:    req.Stream,
	}

	// Convert system message
	if req.System != "" {
		openaiReq.Messages = append(openaiReq.Messages, openaiMessage{
			Role:    "system",
			Content: req.System,
		})
	}

	// Convert messages
	for _, msg := range req.Messages {
		openaiMsg := c.convertMessage(msg)
		openaiReq.Messages = append(openaiReq.Messages, openaiMsg...)
	}

	// Convert tools
	if len(req.Tools) > 0 {
		openaiReq.Tools = make([]openaiTool, 0, len(req.Tools))
		for _, tool := range req.Tools {
			openaiReq.Tools = append(openaiReq.Tools, openaiTool{
				Type: "function",
				Function: openaiFunction{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.InputSchema,
				},
			})
		}
	}

	return openaiReq
}

// convertMessage converts an Anthropic message to OpenAI message(s).
func (c *openaiClient) convertMessage(msg MessageParam) []openaiMessage {
	var result []openaiMessage

	// Try to unmarshal content as string first
	var textContent string
	if err := json.Unmarshal(msg.Content, &textContent); err == nil {
		result = append(result, openaiMessage{
			Role:    c.convertRole(msg.Role),
			Content: textContent,
		})
		return result
	}

	// Try to unmarshal as array of content blocks
	var blocks []ContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		// Fallback: treat as raw string
		result = append(result, openaiMessage{
			Role:    c.convertRole(msg.Role),
			Content: string(msg.Content),
		})
		return result
	}

	// Process content blocks
	for _, block := range blocks {
		switch block.Type {
		case "text":
			result = append(result, openaiMessage{
				Role:    c.convertRole(msg.Role),
				Content: block.Text,
			})
		case "tool_use":
			// Tool call from assistant
			inputJSON, _ := json.Marshal(block.Input)
			result = append(result, openaiMessage{
				Role: "assistant",
				ToolCalls: []openaiToolCall{
					{
						ID:   block.ID,
						Type: "function",
						Function: openaiToolCallFunction{
							Name:      block.Name,
							Arguments: string(inputJSON),
						},
					},
				},
			})
		case "tool_result":
			// Tool result from user
			var contentStr string
			if err := json.Unmarshal(block.Content, &contentStr); err != nil {
				contentStr = string(block.Content)
			}
			result = append(result, openaiMessage{
				Role:       "tool",
				Content:    contentStr,
				ToolCallID: block.ToolUseID,
			})
		case "thinking":
			// Convert thinking to a separate message or skip
			// OpenAI doesn't have native thinking support, so we can include it as a comment
			if block.Thinking != "" {
				result = append(result, openaiMessage{
					Role:    c.convertRole(msg.Role),
					Content: fmt.Sprintf("[Thinking]: %s", block.Thinking),
				})
			}
		}
	}

	return result
}

// convertRole converts Anthropic role to OpenAI role.
func (c *openaiClient) convertRole(role string) string {
	switch role {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	default:
		return role
	}
}

// convertToMessageResponse converts OpenAI response to Anthropic MessageResponse.
func (c *openaiClient) convertToMessageResponse(resp *openaiChatCompletionResponse) *MessageResponse {
	msgResp := &MessageResponse{
		ID:    resp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}

	// Convert stop reason
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		switch choice.FinishReason {
		case "stop":
			msgResp.StopReason = "end_turn"
		case "length":
			msgResp.StopReason = "max_tokens"
		case "tool_calls":
			msgResp.StopReason = "tool_use"
		default:
			msgResp.StopReason = choice.FinishReason
		}

		// Convert content
		if choice.Message.Content != "" {
			msgResp.Content = append(msgResp.Content, ContentBlock{
				Type: "text",
				Text: choice.Message.Content,
			})
		}

		// Convert tool calls
		for _, tc := range choice.Message.ToolCalls {
			var input json.RawMessage
			if tc.Function.Arguments != "" {
				input = json.RawMessage(tc.Function.Arguments)
			}
			msgResp.Content = append(msgResp.Content, ContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
	}

	return msgResp
}

// parseHTTPError reads an error response body and returns an *APIError.
func (c *openaiClient) parseHTTPError(resp *http.Response) *APIError {
	body, _ := io.ReadAll(resp.Body)
	var errBody struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	msg := string(body)
	if json.Unmarshal(body, &errBody) == nil && errBody.Error.Message != "" {
		msg = errBody.Error.Message
	}
	kind := classifyOpenAIError(resp.StatusCode, msg)
	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    msg,
		Headers:    resp.Header,
		Kind:       kind,
	}
}

// classifyOpenAIError maps OpenAI HTTP status codes to APIErrorKind.
func classifyOpenAIError(statusCode int, message string) APIErrorKind {
	switch statusCode {
	case 401:
		return ErrKindUnauthorized
	case 403:
		return ErrKindForbidden
	case 429:
		return ErrKindRateLimit
	case 400:
		if contextOverflowRe.MatchString(message) {
			return ErrKindContextWindow
		}
		return ErrKindInvalidRequest
	default:
		if statusCode >= 500 {
			return ErrKindServerError
		}
		return ErrKindUnknown
	}
}
