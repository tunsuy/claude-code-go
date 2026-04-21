package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// TransportType enumerates supported MCP transport types.
type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportSSE   TransportType = "sse"
	TransportHTTP  TransportType = "http"
	TransportWS    TransportType = "ws" // declared; implementation deferred
)

// JSONRPCMessage is a generic JSON-RPC 2.0 message (request, response, or notification).
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is the error object in a JSON-RPC response.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// Transport is the abstraction for a bidirectional JSON-RPC communication channel.
type Transport interface {
	// Send sends a JSON-RPC message.
	Send(ctx context.Context, msg *JSONRPCMessage) error
	// Recv receives the next JSON-RPC message (blocking).
	Recv(ctx context.Context) (*JSONRPCMessage, error)
	// Close closes the transport.
	Close() error
}

// StdioTransportConfig corresponds to McpStdioServerConfig.
type StdioTransportConfig struct {
	Command string
	Args    []string
	Env     map[string]string
}

// SSETransportConfig corresponds to McpSSEServerConfig (legacy SSE: GET /sse + POST /message).
type SSETransportConfig struct {
	URL     string
	Headers map[string]string
	OAuth   *MCPOAuthConfig
}

// HTTPTransportConfig corresponds to McpHTTPServerConfig (Streamable HTTP with Mcp-Session-Id).
type HTTPTransportConfig struct {
	URL     string
	Headers map[string]string
	OAuth   *MCPOAuthConfig
}

// WSTransportConfig corresponds to McpWSServerConfig.
// WebSocket transport is declared but implementation is deferred.
type WSTransportConfig struct {
	URL     string
	Headers map[string]string
	OAuth   *MCPOAuthConfig
}

// MCPOAuthConfig holds OAuth2 configuration for an MCP server.
type MCPOAuthConfig struct {
	ClientID              string
	CallbackPort          int
	AuthServerMetadataURL string
}

// NewTransport creates a Transport from a TransportType and corresponding config.
func NewTransport(t TransportType, cfg any) (Transport, error) {
	switch t {
	case TransportStdio:
		c, ok := cfg.(StdioTransportConfig)
		if !ok {
			return nil, fmt.Errorf("mcp: NewTransport: expected StdioTransportConfig for stdio transport")
		}
		return newStdioTransport(c)
	case TransportSSE:
		c, ok := cfg.(SSETransportConfig)
		if !ok {
			return nil, fmt.Errorf("mcp: NewTransport: expected SSETransportConfig for sse transport")
		}
		return newSSETransport(c)
	case TransportHTTP:
		c, ok := cfg.(HTTPTransportConfig)
		if !ok {
			return nil, fmt.Errorf("mcp: NewTransport: expected HTTPTransportConfig for http transport")
		}
		return newHTTPTransport(c)
	case TransportWS:
		return nil, fmt.Errorf("mcp: WebSocket transport is not yet implemented")
	default:
		return nil, fmt.Errorf("mcp: unknown transport type: %s", t)
	}
}

// ─── stdio transport ──────────────────────────────────────────────────────────

// stdioRecvResult is a single line result from the background reader.
type stdioRecvResult struct {
	msg *JSONRPCMessage
	err error
}

type stdioTransport struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	recvCh    chan stdioRecvResult
	errCh     chan error
	closeOnce sync.Once
	closed    chan struct{}
	mu        sync.Mutex
	isClosed  bool
}

func newStdioTransport(cfg StdioTransportConfig) (*stdioTransport, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
		env := make([]string, 0, len(cfg.Env))
		for k, v := range cfg.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = append(cmd.Environ(), env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdio: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdio: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: stdio: start process: %w", err)
	}
	t := &stdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		recvCh: make(chan stdioRecvResult, 64),
		errCh:  make(chan error, 1),
		closed: make(chan struct{}),
	}
	// Start a single persistent background reader goroutine (fixes P1-1 goroutine leak).
	go t.readLoop(bufio.NewReader(stdout))
	return t, nil
}

// readLoop is the single persistent goroutine that reads from stdout.
// It exits when the process stdout closes or the transport is closed.
func (t *stdioTransport) readLoop(r *bufio.Reader) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			select {
			case t.errCh <- fmt.Errorf("mcp: stdio: read: %w", err):
			default:
			}
			return
		}
		var msg JSONRPCMessage
		if unmarshalErr := json.Unmarshal([]byte(strings.TrimSpace(line)), &msg); unmarshalErr != nil {
			select {
			case t.recvCh <- stdioRecvResult{nil, fmt.Errorf("mcp: stdio: unmarshal: %w", unmarshalErr)}:
			case <-t.closed:
				return
			}
			continue
		}
		select {
		case t.recvCh <- stdioRecvResult{&msg, nil}:
		case <-t.closed:
			return
		}
	}
}

func (t *stdioTransport) Send(_ context.Context, msg *JSONRPCMessage) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.isClosed {
		return fmt.Errorf("mcp: stdio: transport closed")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcp: stdio: marshal: %w", err)
	}
	data = append(data, '\n')
	_, err = t.stdin.Write(data)
	return err
}

func (t *stdioTransport) Recv(ctx context.Context) (*JSONRPCMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-t.recvCh:
		return r.msg, r.err
	case err := <-t.errCh:
		// Re-broadcast for other waiters
		select {
		case t.errCh <- err:
		default:
		}
		return nil, err
	}
}

func (t *stdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.isClosed {
		return nil
	}
	t.isClosed = true
	t.closeOnce.Do(func() { close(t.closed) })
	t.stdin.Close()
	return t.cmd.Process.Kill()
}

// ─── SSE transport ────────────────────────────────────────────────────────────

// sseTransport implements the legacy SSE+POST MCP transport.
// Sending uses POST /message; receiving uses GET /sse.
type sseTransport struct {
	cfg        SSETransportConfig
	httpClient *http.Client
	recvCh     chan *JSONRPCMessage
	errCh      chan error
	closeOnce  sync.Once
	closed     chan struct{}
	_sessionID string // nolint: unused // reserved for future session management
}

func newSSETransport(cfg SSETransportConfig) (*sseTransport, error) {
	t := &sseTransport{
		cfg:        cfg,
		httpClient: &http.Client{},
		recvCh:     make(chan *JSONRPCMessage, 64),
		errCh:      make(chan error, 1),
		closed:     make(chan struct{}),
	}
	go t.readSSE()
	return t, nil
}

func (t *sseTransport) readSSE() {
	req, err := http.NewRequest(http.MethodGet, t.cfg.URL, nil)
	if err != nil {
		t.errCh <- err
		return
	}
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := t.httpClient.Do(req)
	if err != nil {
		t.errCh <- err
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var dataLine string
	for scanner.Scan() {
		select {
		case <-t.closed:
			return
		default:
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		} else if line == "" && dataLine != "" {
			var msg JSONRPCMessage
			if err := json.Unmarshal([]byte(dataLine), &msg); err == nil {
				select {
				case t.recvCh <- &msg:
				case <-t.closed:
					return
				}
			}
			dataLine = ""
		}
	}
	if err := scanner.Err(); err != nil {
		select {
		case t.errCh <- err:
		default:
		}
	}
}

func (t *sseTransport) Send(_ context.Context, msg *JSONRPCMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcp: sse: marshal: %w", err)
	}
	postURL := t.cfg.URL
	// Convention: replace /sse with /message for POST endpoint
	postURL = strings.TrimSuffix(postURL, "/sse") + "/message"
	req, err := http.NewRequest(http.MethodPost, postURL, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("mcp: sse: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mcp: sse: post: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (t *sseTransport) Recv(ctx context.Context) (*JSONRPCMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-t.recvCh:
		return msg, nil
	case err := <-t.errCh:
		return nil, err
	}
}

func (t *sseTransport) Close() error {
	t.closeOnce.Do(func() { close(t.closed) })
	return nil
}

// ─── Streamable HTTP transport ────────────────────────────────────────────────

// httpTransport implements the Streamable HTTP MCP transport.
// Uses Mcp-Session-Id header for session tracking.
type httpTransport struct {
	cfg        HTTPTransportConfig
	httpClient *http.Client
	sessionID  atomic.Value // string
	mu         sync.Mutex
	closed     bool
}

func newHTTPTransport(cfg HTTPTransportConfig) (*httpTransport, error) {
	return &httpTransport{
		cfg:        cfg,
		httpClient: &http.Client{},
	}, nil
}

func (t *httpTransport) Send(ctx context.Context, msg *JSONRPCMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcp: http: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("mcp: http: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	if sid, ok := t.sessionID.Load().(string); ok && sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mcp: http: post: %w", err)
	}
	defer resp.Body.Close()
	// Store session ID from response headers
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.sessionID.Store(sid)
	}
	return nil
}

func (t *httpTransport) Recv(ctx context.Context) (*JSONRPCMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.cfg.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: http: build recv request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	if sid, ok := t.sessionID.Load().(string); ok && sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: http: get: %w", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var dataLine string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		} else if line == "" && dataLine != "" {
			var msg JSONRPCMessage
			if err := json.Unmarshal([]byte(dataLine), &msg); err != nil {
				return nil, fmt.Errorf("mcp: http: unmarshal: %w", err)
			}
			return &msg, nil
		}
	}
	return nil, io.EOF
}

func (t *httpTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}
