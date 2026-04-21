package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ─── NewClient / factory ──────────────────────────────────────────────────────

func TestNewClient_DirectProvider(t *testing.T) {
	c, err := NewClient(ClientConfig{Provider: ProviderDirect, APIKey: "key"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewClient_EmptyProviderDefaultsToDirect(t *testing.T) {
	c, err := NewClient(ClientConfig{APIKey: "key"}, nil)
	if err != nil || c == nil {
		t.Fatalf("expected direct client, got err=%v client=%v", err, c)
	}
}

func TestNewClient_BedrockProvider(t *testing.T) {
	c, err := NewClient(ClientConfig{Provider: ProviderBedrock, AWSRegion: "us-east-1"}, nil)
	if err != nil || c == nil {
		t.Fatalf("unexpected: err=%v, client=%v", err, c)
	}
}

func TestNewClient_VertexProvider(t *testing.T) {
	c, err := NewClient(ClientConfig{Provider: ProviderVertex, GCPProject: "proj", GCPRegion: "us-central1"}, nil)
	if err != nil || c == nil {
		t.Fatalf("unexpected: err=%v, client=%v", err, c)
	}
}

func TestNewClient_FoundryProvider(t *testing.T) {
	c, err := NewClient(ClientConfig{Provider: ProviderFoundry, APIKey: "key"}, nil)
	if err != nil || c == nil {
		t.Fatalf("unexpected: err=%v, client=%v", err, c)
	}
}

func TestNewClient_CustomHeaders(t *testing.T) {
	cfg := ClientConfig{
		Provider:      ProviderDirect,
		APIKey:        "key",
		CustomHeaders: map[string]string{"X-Custom": "value"},
	}
	c, err := NewClient(cfg, nil)
	if err != nil || c == nil {
		t.Fatalf("unexpected: err=%v, client=%v", err, c)
	}
}

func TestBuildDefaultHeaders(t *testing.T) {
	h := buildDefaultHeaders("1.2.3")
	if !strings.Contains(h["User-Agent"], "1.2.3") {
		t.Errorf("expected version in User-Agent, got %q", h["User-Agent"])
	}
}

// ─── directClient.Complete ────────────────────────────────────────────────────

func TestDirectClient_Complete_Success(t *testing.T) {
	resp := MessageResponse{
		ID:    "msg_test",
		Role:  "assistant",
		Model: "claude-3",
		Content: []ContentBlock{
			{Type: "text", Text: "Hi there"},
		},
		StopReason: "end_turn",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := &directClient{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: server.Client(),
		headers:    map[string]string{},
	}

	req := &MessageRequest{
		Model:     "claude-3",
		MaxTokens: 100,
		Messages:  []MessageParam{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
	}
	msg, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ID != "msg_test" {
		t.Errorf("expected msg_test, got %q", msg.ID)
	}
	if len(msg.Content) != 1 || msg.Content[0].Text != "Hi there" {
		t.Errorf("unexpected content: %+v", msg.Content)
	}
}

func TestDirectClient_Complete_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		fmt.Fprintf(w, `{"error":{"type":"rate_limit_error","message":"Rate limited"}}`)
	}))
	defer server.Close()

	c := &directClient{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: server.Client(),
		headers:    map[string]string{},
	}

	_, err := c.Complete(context.Background(), &MessageRequest{
		Model:     "claude-3",
		MaxTokens: 10,
		Messages:  []MessageParam{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *APIError
	if !isAPIError(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 429 {
		t.Errorf("expected 429, got %d", apiErr.StatusCode)
	}
}

// ─── directClient.Stream ─────────────────────────────────────────────────────

func TestDirectClient_Stream_Success(t *testing.T) {
	sseBody := `event: message_start
data: {"type":"message_start","message":{"id":"msg_s","type":"message","role":"assistant","content":[],"model":"claude-3","stop_reason":null,"usage":{"input_tokens":3,"output_tokens":0}}}

event: message_stop
data: {"type":"message_stop"}

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	}))
	defer server.Close()

	c := &directClient{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: server.Client(),
		headers:    map[string]string{},
	}

	reader, err := c.Stream(context.Background(), &MessageRequest{
		Model:     "claude-3",
		MaxTokens: 10,
		Messages:  []MessageParam{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	ev, err := reader.Next()
	if err != nil {
		t.Fatalf("unexpected error reading event: %v", err)
	}
	if ev.Type != EventMessageStart {
		t.Errorf("expected message_start, got %q", ev.Type)
	}
}

func TestDirectClient_Stream_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"type":"server_error","message":"Internal error"}}`)
	}))
	defer server.Close()

	c := &directClient{
		apiKey:     "test-key",
		baseURL:    server.URL,
		httpClient: server.Client(),
		headers:    map[string]string{},
	}

	_, err := c.Stream(context.Background(), &MessageRequest{
		Model:     "claude-3",
		MaxTokens: 10,
		Messages:  []MessageParam{{Role: "user", Content: json.RawMessage(`"Hi"`)}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ─── parseHTTPError ───────────────────────────────────────────────────────────

func TestParseHTTPError_JSONBody(t *testing.T) {
	body := `{"error":{"type":"overloaded_error","message":"Service overloaded"}}`
	resp := &http.Response{
		StatusCode: 529,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	apiErr := parseHTTPError(resp)
	if apiErr.StatusCode != 529 {
		t.Errorf("expected 529, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "Service overloaded" {
		t.Errorf("unexpected message: %q", apiErr.Message)
	}
	if apiErr.Kind != ErrKindOverloaded {
		t.Errorf("expected overloaded kind, got %q", apiErr.Kind)
	}
}

func TestParseHTTPError_PlainBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: 503,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("Service Unavailable")),
	}
	apiErr := parseHTTPError(resp)
	if apiErr.StatusCode != 503 {
		t.Errorf("expected 503, got %d", apiErr.StatusCode)
	}
}

// ─── Error helpers ────────────────────────────────────────────────────────────

func TestIs529Error(t *testing.T) {
	err := &APIError{StatusCode: 529, Kind: ErrKindOverloaded}
	if !Is529Error(err) {
		t.Error("expected Is529Error to return true")
	}
	if Is529Error(nil) {
		t.Error("expected Is529Error(nil) to return false")
	}
}

func TestIsOAuthTokenRevokedError(t *testing.T) {
	err := &APIError{StatusCode: 403, Message: "token is revoked", Kind: ErrKindForbidden}
	if !IsOAuthTokenRevokedError(err) {
		t.Error("expected revoked error to match")
	}
	err2 := &APIError{StatusCode: 403, Message: "forbidden", Kind: ErrKindForbidden}
	if IsOAuthTokenRevokedError(err2) {
		t.Error("should not match generic 403")
	}
}

func TestParseContextOverflowError(t *testing.T) {
	err := &APIError{
		StatusCode: 400,
		Kind:       ErrKindContextWindow,
		Message:    "input length (50000 tokens) exceeds context limit (40000 tokens)",
	}
	input, limit, ok := ParseContextOverflowError(err)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if input != 50000 || limit != 40000 {
		t.Errorf("expected 50000/40000, got %d/%d", input, limit)
	}
}

func TestParseContextOverflowError_NoMatch(t *testing.T) {
	err := &APIError{StatusCode: 400, Kind: ErrKindInvalidRequest, Message: "bad request"}
	_, _, ok := ParseContextOverflowError(err)
	if ok {
		t.Error("expected ok=false for non-context-overflow error")
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		status int
		msg    string
		want   APIErrorKind
	}{
		{401, "", ErrKindUnauthorized},
		{403, "", ErrKindForbidden},
		{429, "", ErrKindRateLimit},
		{529, "", ErrKindOverloaded},
		{500, "", ErrKindServerError},
		{503, "", ErrKindServerError},
		{400, "prompt is too long", ErrKindContextWindow},
		{400, "bad request", ErrKindInvalidRequest},
		{404, "", ErrKindUnknown},
	}
	for _, tc := range cases {
		got := classifyError(tc.status, tc.msg)
		if got != tc.want {
			t.Errorf("classifyError(%d, %q) = %q, want %q", tc.status, tc.msg, got, tc.want)
		}
	}
}

// isAPIError is a helper to extract *APIError (avoids importing errors in test).
func isAPIError(err error, out **APIError) bool {
	if e, ok := err.(*APIError); ok {
		*out = e
		return true
	}
	return false
}
