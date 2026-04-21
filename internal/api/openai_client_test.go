package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAIClientComplete tests the non-streaming completion flow.
func TestOpenAIClientComplete(t *testing.T) {
	// Mock server that returns a valid OpenAI response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Authorization: Bearer test-key, got %s", auth)
		}

		// Parse request body
		var req openaiChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if req.Model != "gpt-4" {
			t.Errorf("expected model gpt-4, got %s", req.Model)
		}

		// Return mock response
		resp := openaiChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: 1677652288,
			Model:   "gpt-4",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiResponseMessage{
						Role:    "assistant",
						Content: "Hello! How can I help you?",
					},
					FinishReason: "stop",
				},
			},
			Usage: openaiUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	cfg := ClientConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}
	client, err := NewClient(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Make request
	req := &MessageRequest{
		Model:     "gpt-4",
		MaxTokens: 100,
		Messages: []MessageParam{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}
	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Verify response
	if resp.ID != "chatcmpl-123" {
		t.Errorf("expected ID chatcmpl-123, got %s", resp.ID)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", resp.Role)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %s", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Text != "Hello! How can I help you?" {
		t.Errorf("unexpected content: %s", resp.Content[0].Text)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("expected 20 output tokens, got %d", resp.Usage.OutputTokens)
	}
}

// TestOpenAIClientStream tests the streaming completion flow.
func TestOpenAIClientStream(t *testing.T) {
	// Mock SSE response
	sseData := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		_, _ = w.Write([]byte(sseData))
	}))
	defer server.Close()

	// Create client
	cfg := ClientConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}
	client, err := NewClient(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Make streaming request
	req := &MessageRequest{
		Model:     "gpt-4",
		MaxTokens: 100,
		Messages: []MessageParam{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}
	reader, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	defer reader.Close()

	// Collect events
	var events []*StreamEvent
	for {
		ev, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next failed: %v", err)
		}
		events = append(events, ev)
	}

	// Verify we got expected events
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// First event should be message_start
	if events[0].Type != EventMessageStart {
		t.Errorf("expected first event to be message_start, got %s", events[0].Type)
	}

	// Should have content_block_delta events with text
	var textContent strings.Builder
	for _, ev := range events {
		if ev.Type == EventContentBlockDelta && ev.ContentBlockDelta != nil {
			textContent.WriteString(ev.ContentBlockDelta.Delta.Text)
		}
	}
	if textContent.String() != "Hello!" {
		t.Errorf("expected content 'Hello!', got '%s'", textContent.String())
	}

	// Last event should be message_stop
	lastEvent := events[len(events)-1]
	if lastEvent.Type != EventMessageStop {
		t.Errorf("expected last event to be message_stop, got %s", lastEvent.Type)
	}
}

// TestOpenAIClientToolCalls tests tool calling functionality.
func TestOpenAIClientToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse request to verify tool schema
		var req openaiChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if len(req.Tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(req.Tools))
		}
		if req.Tools[0].Function.Name != "get_weather" {
			t.Errorf("expected tool name get_weather, got %s", req.Tools[0].Function.Name)
		}

		// Return tool call response
		resp := openaiChatCompletionResponse{
			ID:    "chatcmpl-123",
			Model: "gpt-4",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiResponseMessage{
						Role: "assistant",
						ToolCalls: []openaiToolCall{
							{
								ID:   "call_123",
								Type: "function",
								Function: openaiToolCallFunction{
									Name:      "get_weather",
									Arguments: `{"location":"San Francisco"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
			Usage: openaiUsage{
				PromptTokens:     15,
				CompletionTokens: 10,
				TotalTokens:      25,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client
	cfg := ClientConfig{
		Provider: ProviderOpenAI,
		APIKey:   "test-key",
		BaseURL:  server.URL,
	}
	client, err := NewClient(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Make request with tool
	req := &MessageRequest{
		Model:     "gpt-4",
		MaxTokens: 100,
		Messages: []MessageParam{
			{
				Role:    "user",
				Content: json.RawMessage(`"What's the weather in San Francisco?"`),
			},
		},
		Tools: []ToolSchema{
			{
				Name:        "get_weather",
				Description: "Get the weather for a location",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
			},
		},
	}
	resp, err := client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Verify tool use in response
	if resp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason tool_use, got %s", resp.StopReason)
	}
	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "tool_use" {
		t.Errorf("expected content type tool_use, got %s", resp.Content[0].Type)
	}
	if resp.Content[0].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %s", resp.Content[0].Name)
	}
	if resp.Content[0].ID != "call_123" {
		t.Errorf("expected tool ID call_123, got %s", resp.Content[0].ID)
	}
}

// TestOpenAIClientError tests error handling.
func TestOpenAIClientError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedKind   APIErrorKind
	}{
		{
			name:           "unauthorized",
			statusCode:     401,
			responseBody:   `{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`,
			expectedKind:   ErrKindUnauthorized,
		},
		{
			name:           "rate_limit",
			statusCode:     429,
			responseBody:   `{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`,
			expectedKind:   ErrKindRateLimit,
		},
		{
			name:           "server_error",
			statusCode:     500,
			responseBody:   `{"error":{"message":"Internal server error","type":"server_error"}}`,
			expectedKind:   ErrKindServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			cfg := ClientConfig{
				Provider: ProviderOpenAI,
				APIKey:   "test-key",
				BaseURL:  server.URL,
			}
			client, err := NewClient(cfg, nil)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			req := &MessageRequest{
				Model:     "gpt-4",
				MaxTokens: 100,
				Messages: []MessageParam{
					{
						Role:    "user",
						Content: json.RawMessage(`"Hello"`),
					},
				},
			}
			_, err = client.Complete(context.Background(), req)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			apiErr, ok := err.(*APIError)
			if !ok {
				t.Fatalf("expected *APIError, got %T", err)
			}
			if apiErr.Kind != tt.expectedKind {
				t.Errorf("expected kind %s, got %s", tt.expectedKind, apiErr.Kind)
			}
		})
	}
}

// TestOpenAIClientHeaders tests that custom headers are properly set.
func TestOpenAIClientHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		resp := openaiChatCompletionResponse{
			ID:    "chatcmpl-123",
			Model: "gpt-4",
			Choices: []openaiChoice{
				{
					Index:        0,
					Message:      openaiResponseMessage{Role: "assistant", Content: "Hi"},
					FinishReason: "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := ClientConfig{
		Provider:           ProviderOpenAI,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		OpenAIOrganization: "org-123",
		OpenAIProject:      "proj-456",
	}
	client, err := NewClient(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req := &MessageRequest{
		Model:     "gpt-4",
		MaxTokens: 100,
		Messages: []MessageParam{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
	}
	_, err = client.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Verify headers
	if auth := receivedHeaders.Get("Authorization"); auth != "Bearer test-key" {
		t.Errorf("expected Authorization: Bearer test-key, got %s", auth)
	}
	if org := receivedHeaders.Get("OpenAI-Organization"); org != "org-123" {
		t.Errorf("expected OpenAI-Organization: org-123, got %s", org)
	}
	if proj := receivedHeaders.Get("OpenAI-Project"); proj != "proj-456" {
		t.Errorf("expected OpenAI-Project: proj-456, got %s", proj)
	}
}

// TestOpenAIMessageConversion tests the message format conversion.
func TestOpenAIMessageConversion(t *testing.T) {
	client := &openaiClient{}

	tests := []struct {
		name     string
		input    MessageParam
		expected []openaiMessage
	}{
		{
			name: "simple_text",
			input: MessageParam{
				Role:    "user",
				Content: json.RawMessage(`"Hello world"`),
			},
			expected: []openaiMessage{
				{Role: "user", Content: "Hello world"},
			},
		},
		{
			name: "text_content_block",
			input: MessageParam{
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"Hello!"}]`),
			},
			expected: []openaiMessage{
				{Role: "assistant", Content: "Hello!"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.convertMessage(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d messages, got %d", len(tt.expected), len(result))
			}
			for i, exp := range tt.expected {
				if result[i].Role != exp.Role {
					t.Errorf("message %d: expected role %s, got %s", i, exp.Role, result[i].Role)
				}
				if result[i].Content != exp.Content {
					t.Errorf("message %d: expected content %s, got %s", i, exp.Content, result[i].Content)
				}
			}
		})
	}
}
