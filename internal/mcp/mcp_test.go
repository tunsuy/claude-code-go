package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	toolpkg "github.com/tunsuy/claude-code-go/internal/tools"
)

// ─── NormalizeToolName ────────────────────────────────────────────────────────

func TestNormalizeToolName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with space", "with_space"},
		{"slash/tool", "slash_tool"},
		{"dots.and.dashes-ok_ok", "dots_and_dashes-ok_ok"},
		{"CamelCase123", "CamelCase123"},
		{"special!@#$chars", "special____chars"},
		{"", ""},
	}
	for _, tc := range cases {
		got := NormalizeToolName(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeToolName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── truncateDescription ─────────────────────────────────────────────────────

func TestTruncateDescription_Short(t *testing.T) {
	s := "short string"
	got := truncateDescription(s)
	if got != s {
		t.Errorf("expected unchanged short string, got %q", got)
	}
}

func TestTruncateDescription_Long(t *testing.T) {
	s := strings.Repeat("a", MaxMCPDescriptionLength+100)
	got := truncateDescription(s)
	if len([]rune(got)) != MaxMCPDescriptionLength {
		t.Errorf("expected %d runes, got %d", MaxMCPDescriptionLength, len([]rune(got)))
	}
}

func TestTruncateDescription_Unicode(t *testing.T) {
	// Each 中文 character is one rune
	s := strings.Repeat("中", MaxMCPDescriptionLength+10)
	got := truncateDescription(s)
	if len([]rune(got)) != MaxMCPDescriptionLength {
		t.Errorf("expected %d runes, got %d", MaxMCPDescriptionLength, len([]rune(got)))
	}
}

// ─── isMCPAuthError ───────────────────────────────────────────────────────────

func TestIsMCPAuthError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"connection refused", false},
		{"401 unauthorized", true},
		{"403 forbidden", true},
		{"token expired", true},
		{"unauthorized access", true},
		{"auth required", true},
		{"timeout", false},
		{"", false},
	}
	for _, tc := range cases {
		err := fmt.Errorf("%s", tc.msg)
		got := isMCPAuthError(err)
		if got != tc.want {
			t.Errorf("isMCPAuthError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

func TestIsMCPAuthError_Nil(t *testing.T) {
	if isMCPAuthError(nil) {
		t.Error("expected false for nil error")
	}
}

// ─── Pool ─────────────────────────────────────────────────────────────────────

func TestPool_GetAll_Empty(t *testing.T) {
	p := NewPool()
	if got := p.GetAll(); len(got) != 0 {
		t.Errorf("expected empty pool, got %d connections", len(got))
	}
}

func TestPool_GetConnected_Empty(t *testing.T) {
	p := NewPool()
	if got := p.GetConnected(); len(got) != 0 {
		t.Errorf("expected no connected, got %d", len(got))
	}
}

func TestPool_Disconnect_Unknown(t *testing.T) {
	p := NewPool()
	// Disconnecting an unknown name should be a no-op
	if err := p.Disconnect("nonexistent"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPool_Reconnect_Unknown(t *testing.T) {
	p := NewPool()
	err := p.Reconnect(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error reconnecting unknown server")
	}
}

func TestPool_Connect_UnsupportedTransport(t *testing.T) {
	p := NewPool()
	err := p.Connect(context.Background(), "test", ServerConfig{
		Transport: "unsupported",
	})
	if err == nil {
		t.Error("expected error for unsupported transport")
	}
	conns := p.GetAll()
	if len(conns) != 1 {
		t.Fatalf("expected 1 conn record, got %d", len(conns))
	}
	if conns[0].Status == StatusConnected {
		t.Error("should not be connected")
	}
}

func TestPool_Disconnect_MarksDisabled(t *testing.T) {
	p := NewPool()
	// Inject a fake connected connection
	p.mu.Lock()
	p.connections["srv"] = &ServerConnection{
		Name:   "srv",
		Status: StatusConnected,
	}
	p.mu.Unlock()

	if err := p.Disconnect("srv"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	conns := p.GetAll()
	if len(conns) != 1 || conns[0].Status != StatusDisabled {
		t.Errorf("expected disabled status, got %v", conns[0].Status)
	}
}

func TestPool_GetConnected_FiltersCorrectly(t *testing.T) {
	p := NewPool()
	p.mu.Lock()
	p.connections["connected"] = &ServerConnection{Name: "connected", Status: StatusConnected}
	p.connections["failed"] = &ServerConnection{Name: "failed", Status: StatusFailed}
	p.connections["disabled"] = &ServerConnection{Name: "disabled", Status: StatusDisabled}
	p.mu.Unlock()

	got := p.GetConnected()
	if len(got) != 1 || got[0].Name != "connected" {
		t.Errorf("expected 1 connected conn, got %d", len(got))
	}
}

func TestServerConnection_IsConnected(t *testing.T) {
	conn := &ServerConnection{Status: StatusConnected}
	if !conn.IsConnected() {
		t.Error("expected IsConnected=true")
	}
	conn.Status = StatusFailed
	if conn.IsConnected() {
		t.Error("expected IsConnected=false")
	}
}

// ─── AdaptToTool ─────────────────────────────────────────────────────────────

func TestAdaptToTool_Naming(t *testing.T) {
	def := MCPToolDef{
		Name:        "my/tool",
		Description: "does stuff",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[]}`),
	}
	adapted := AdaptToTool("server1", def, nil)
	if adapted.Name() != "server1__my_tool" {
		t.Errorf("unexpected name: %q", adapted.Name())
	}
}

func TestAdaptToTool_InputSchema(t *testing.T) {
	def := MCPToolDef{
		Name:        "tool",
		Description: "desc",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]}`),
	}
	adapted := AdaptToTool("srv", def, nil)
	schema := adapted.InputSchema()
	if schema.Type != "object" {
		t.Errorf("expected object type, got %q", schema.Type)
	}
	if len(schema.Properties) == 0 {
		t.Error("expected non-empty properties")
	}
}

// ─── mockTransport for jsonRPCClient tests ────────────────────────────────────

type mockTransport struct {
	sendCh chan *JSONRPCMessage
	recvCh chan *JSONRPCMessage
	closed bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		sendCh: make(chan *JSONRPCMessage, 16),
		recvCh: make(chan *JSONRPCMessage, 16),
	}
}

func (m *mockTransport) Send(_ context.Context, msg *JSONRPCMessage) error {
	if m.closed {
		return fmt.Errorf("transport closed")
	}
	m.sendCh <- msg
	return nil
}

func (m *mockTransport) Recv(ctx context.Context) (*JSONRPCMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-m.recvCh:
		return msg, nil
	}
}

func (m *mockTransport) Close() error {
	m.closed = true
	return nil
}

// autoRespond sets up a goroutine that automatically responds to sent messages.
func (m *mockTransport) autoRespond(result json.RawMessage) {
	go func() {
		for req := range m.sendCh {
			if req.ID == nil {
				continue // notification
			}
			resp := &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			}
			m.recvCh <- resp
		}
	}()
}

func TestJSONRPCClient_ContextCancellation(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Don't respond — just let context expire
	_, err := client.call(ctx, "ping", nil)
	if err == nil {
		t.Error("expected error from context cancellation")
	}
}

func TestJSONRPCClient_CallReturnsResult(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	result := json.RawMessage(`{"ok":true}`)
	mt.autoRespond(result)

	got, err := client.call(context.Background(), "test/method", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(result) {
		t.Errorf("expected %s, got %s", result, got)
	}
}

func TestJSONRPCClient_CallReturnsRPCError(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	go func() {
		req := <-mt.sendCh
		resp := &JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &JSONRPCError{Code: -32601, Message: "Method not found"},
		}
		mt.recvCh <- resp
	}()

	_, err := client.call(context.Background(), "missing/method", nil)
	if err == nil {
		t.Error("expected error for RPC error response")
	}
}

// ─── SSE Transport with httptest ─────────────────────────────────────────────

func TestSSETransport_RecvMessage(t *testing.T) {
	msg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      float64(1),
		Result:  json.RawMessage(`{"pong":true}`),
	}
	msgJSON, _ := json.Marshal(msg)

	// Serve the SSE stream
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/message" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", msgJSON)
	}))
	defer server.Close()

	cfg := SSETransportConfig{URL: server.URL + "/sse"}
	tr, err := newSSETransport(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	got, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got.Result) != string(msg.Result) {
		t.Errorf("unexpected result: %s", got.Result)
	}
}

// ─── HTTP Transport with httptest ────────────────────────────────────────────

func TestHTTPTransport_SendAndSessionID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Mcp-Session-Id", "sess-123")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := HTTPTransportConfig{URL: server.URL}
	tr, err := newHTTPTransport(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := &JSONRPCMessage{JSONRPC: "2.0", ID: int64(1), Method: "ping"}
	if err := tr.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	// Session ID should be stored
	sid, _ := tr.sessionID.Load().(string)
	if sid != "sess-123" {
		t.Errorf("expected session id 'sess-123', got %q", sid)
	}
}

func TestHTTPTransport_Close(t *testing.T) {
	cfg := HTTPTransportConfig{URL: "http://localhost:9999"}
	tr, _ := newHTTPTransport(cfg)
	if err := tr.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !tr.closed {
		t.Error("expected closed=true")
	}
}

// ─── NewTransport ─────────────────────────────────────────────────────────────

func TestNewTransport_Unknown(t *testing.T) {
	_, err := NewTransport("unknown", nil)
	if err == nil {
		t.Error("expected error for unknown transport")
	}
}

func TestNewTransport_WS(t *testing.T) {
	_, err := NewTransport(TransportWS, nil)
	if err == nil {
		t.Error("expected error for WS (not implemented)")
	}
}

func TestNewTransport_SSEWrongConfig(t *testing.T) {
	_, err := NewTransport(TransportSSE, "not a config")
	if err == nil {
		t.Error("expected error for wrong config type")
	}
}

func TestNewTransport_HTTPWrongConfig(t *testing.T) {
	_, err := NewTransport(TransportHTTP, "not a config")
	if err == nil {
		t.Error("expected error for wrong config type")
	}
}

// ─── mcpTool methods ──────────────────────────────────────────────────────────

func newTestMCPTool(serverName, toolName string, client MCPClient) *mcpTool {
	def := MCPToolDef{
		Name:        toolName,
		Description: "test tool description",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}},"required":["x"]}`),
	}
	return AdaptToTool(serverName, def, client).(*mcpTool)
}

func TestMCPTool_Aliases(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	if aliases := mt.Aliases(); aliases != nil {
		t.Errorf("expected nil aliases, got %v", aliases)
	}
}

func TestMCPTool_Description(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	desc := mt.Description(nil, nil)
	if desc != "test tool description" {
		t.Errorf("expected description, got %q", desc)
	}
}

func TestMCPTool_Prompt(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	s, err := mt.Prompt(context.Background(), nil)
	if err != nil || s != "" {
		t.Errorf("expected empty prompt, got %q err=%v", s, err)
	}
}

func TestMCPTool_MaxResultSizeChars(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	if mt.MaxResultSizeChars() != -1 {
		t.Error("expected -1")
	}
}

func TestMCPTool_SearchHint(t *testing.T) {
	mt := newTestMCPTool("srv", "mytool", nil)
	hint := mt.SearchHint()
	if !strings.Contains(hint, "srv") || !strings.Contains(hint, "mytool") {
		t.Errorf("expected server and tool name in hint, got %q", hint)
	}
}

func TestMCPTool_ConcurrencyAndSafety(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	if mt.IsConcurrencySafe(nil) {
		t.Error("expected IsConcurrencySafe=false")
	}
	if mt.IsReadOnly(nil) {
		t.Error("expected IsReadOnly=false")
	}
	if mt.IsDestructive(nil) {
		t.Error("expected IsDestructive=false")
	}
	if !mt.IsEnabled() {
		t.Error("expected IsEnabled=true")
	}
	if mt.InterruptBehavior() != "cancel" {
		t.Errorf("expected cancel, got %q", mt.InterruptBehavior())
	}
}

func TestMCPTool_ValidateInput(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	result, err := mt.ValidateInput(nil, nil)
	if err != nil || !result.OK {
		t.Errorf("expected OK=true, err=nil; got %v %v", result, err)
	}
}

func TestMCPTool_CheckPermissions(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	result, err := mt.CheckPermissions(nil, nil)
	if err != nil || result.Behavior != "ask" {
		t.Errorf("unexpected: %v %v", result, err)
	}
}

func TestMCPTool_PreparePermissionMatcher(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	fn, err := mt.PreparePermissionMatcher(nil)
	if err != nil || fn != nil {
		t.Errorf("expected nil fn and nil err, got fn=<func> err=%v", err)
	}
}

func TestMCPTool_ToAutoClassifierInput(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	if s := mt.ToAutoClassifierInput(nil); s != "" {
		t.Errorf("expected empty, got %q", s)
	}
}

func TestMCPTool_UserFacingName(t *testing.T) {
	mt := newTestMCPTool("myserver", "my/tool", nil)
	name := mt.UserFacingName(nil)
	if !strings.Contains(name, "myserver") || !strings.Contains(name, "my/tool") {
		t.Errorf("unexpected user-facing name: %q", name)
	}
}

func TestMCPTool_MCPInfo(t *testing.T) {
	mt := newTestMCPTool("myserver", "my/tool", nil)
	info := mt.MCPInfo()
	if info.ServerName != "myserver" || info.ToolName != "my/tool" {
		t.Errorf("unexpected MCPInfo: %+v", info)
	}
}

func TestMCPTool_MapResultToToolResultBlock_String(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	raw, err := mt.MapResultToToolResultBlock("hello output", "tool-use-id-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(raw), "tool_result") {
		t.Errorf("expected tool_result in output: %s", raw)
	}
	if !strings.Contains(string(raw), "hello output") {
		t.Errorf("expected content in output: %s", raw)
	}
}

func TestMCPTool_MapResultToToolResultBlock_NonString(t *testing.T) {
	mt := newTestMCPTool("srv", "tool", nil)
	raw, err := mt.MapResultToToolResultBlock(map[string]string{"k": "v"}, "id-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(raw) == 0 {
		t.Error("expected non-empty output")
	}
}

// mockMCPClient implements MCPClient for testing.
type mockMCPClient struct {
	tools     []MCPToolDef
	callResult *MCPToolResult
	callErr   error
	closeErr  error
	resources []MCPResource
	resource  *MCPResourceContent
	resErr    error
	closed    bool
}

func (m *mockMCPClient) ListTools(_ context.Context) ([]MCPToolDef, error) {
	return m.tools, nil
}
func (m *mockMCPClient) CallTool(_ context.Context, _ string, _ map[string]any) (*MCPToolResult, error) {
	return m.callResult, m.callErr
}
func (m *mockMCPClient) ListResources(_ context.Context) ([]MCPResource, error) {
	return m.resources, m.resErr
}
func (m *mockMCPClient) ReadResource(_ context.Context, _ string) (*MCPResourceContent, error) {
	return m.resource, m.resErr
}
func (m *mockMCPClient) Ping(_ context.Context) error { return nil }
func (m *mockMCPClient) Close() error {
	m.closed = true
	return m.closeErr
}
func (m *mockMCPClient) ServerInfo() ServerInfo { return ServerInfo{Name: "mock"} }

func TestMCPTool_Call_Success(t *testing.T) {
	client := &mockMCPClient{
		callResult: &MCPToolResult{
			Content: []MCPContent{
				{Type: "text", Text: "result text"},
			},
		},
	}
	mcpt := newTestMCPTool("srv", "tool", client)
	useCtx := &toolpkg.UseContext{Ctx: context.Background()}
	result, err := mcpt.Call(json.RawMessage(`{"x":"hello"}`), useCtx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false")
	}
	if !strings.Contains(result.Content.(string), "result text") {
		t.Errorf("unexpected content: %v", result.Content)
	}
}

func TestMCPTool_Call_ImageContent(t *testing.T) {
	client := &mockMCPClient{
		callResult: &MCPToolResult{
			Content: []MCPContent{
				{Type: "image", MIMEType: "image/png"},
			},
		},
	}
	mcpt := newTestMCPTool("srv", "tool", client)
	useCtx := &toolpkg.UseContext{Ctx: context.Background()}
	result, err := mcpt.Call(nil, useCtx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content.(string), "image") {
		t.Errorf("unexpected content: %v", result.Content)
	}
}

func TestMCPTool_Call_ClientError(t *testing.T) {
	client := &mockMCPClient{
		callErr: fmt.Errorf("tool call failed"),
	}
	mcpt := newTestMCPTool("srv", "tool", client)
	useCtx := &toolpkg.UseContext{Ctx: context.Background()}
	result, err := mcpt.Call(nil, useCtx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for client error")
	}
}

func TestMCPTool_Call_InvalidInput(t *testing.T) {
	client := &mockMCPClient{}
	mcpt := newTestMCPTool("srv", "tool", client)
	useCtx := &toolpkg.UseContext{Ctx: context.Background()}
	result, err := mcpt.Call(json.RawMessage(`not json`), useCtx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON input")
	}
}

func TestMCPTool_Call_IsError(t *testing.T) {
	client := &mockMCPClient{
		callResult: &MCPToolResult{
			Content: []MCPContent{{Type: "text", Text: "error from mcp"}},
			IsError: true,
		},
	}
	mcpt := newTestMCPTool("srv", "tool", client)
	useCtx := &toolpkg.UseContext{Ctx: context.Background()}
	result, err := mcpt.Call(nil, useCtx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when MCPToolResult.IsError is true")
	}
}

// ─── jsonRPCClient high-level methods ────────────────────────────────────────

func TestJSONRPCClient_ListTools(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	resp := json.RawMessage(`{"tools":[{"name":"bash","description":"runs bash","inputSchema":{"type":"object","properties":{}}}]}`)
	mt.autoRespond(resp)

	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "bash" {
		t.Errorf("unexpected tools: %+v", tools)
	}
}

func TestJSONRPCClient_CallTool(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	resp := json.RawMessage(`{"content":[{"type":"text","text":"hello"}],"isError":false}`)
	mt.autoRespond(resp)

	result, err := client.CallTool(context.Background(), "bash", map[string]any{"cmd": "ls"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "hello" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestJSONRPCClient_CallTool_WithImageContent(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	resp := json.RawMessage(`{"content":[{"type":"image","mimeType":"image/png","data":"abc123"}],"isError":false}`)
	mt.autoRespond(resp)

	result, err := client.CallTool(context.Background(), "screenshot", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "image" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestJSONRPCClient_ListResources(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	resp := json.RawMessage(`{"resources":[{"uri":"file:///foo","name":"foo","description":"a file","mimeType":"text/plain"}]}`)
	mt.autoRespond(resp)

	resources, err := client.ListResources(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resources) != 1 || resources[0].URI != "file:///foo" {
		t.Errorf("unexpected resources: %+v", resources)
	}
}

func TestJSONRPCClient_ReadResource(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	resp := json.RawMessage(`{"contents":[{"uri":"file:///foo","mimeType":"text/plain","text":"file content"}]}`)
	mt.autoRespond(resp)

	rc, err := client.ReadResource(context.Background(), "file:///foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.Text != "file content" {
		t.Errorf("unexpected text: %q", rc.Text)
	}
}

func TestJSONRPCClient_ReadResource_EmptyContents(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	resp := json.RawMessage(`{"contents":[]}`)
	mt.autoRespond(resp)

	_, err := client.ReadResource(context.Background(), "file:///bar")
	if err == nil {
		t.Error("expected error for empty contents")
	}
}

func TestJSONRPCClient_Ping(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	mt.autoRespond(json.RawMessage(`{}`))

	if err := client.Ping(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestJSONRPCClient_Close(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)
	if err := client.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !mt.closed {
		t.Error("expected transport closed")
	}
}

func TestJSONRPCClient_ServerInfo(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)
	info := client.ServerInfo()
	// Default is zero-value, just check no panic
	_ = info
}

func TestJSONRPCClient_Initialize(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	initResp := json.RawMessage(`{
		"protocolVersion":"2024-11-05",
		"serverInfo":{"name":"test-srv","version":"1.0"},
		"capabilities":{"tools":{},"resources":null}
	}`)

	go func() {
		// Respond to initialize call
		req := <-mt.sendCh
		if req.ID == nil {
			return
		}
		mt.recvCh <- &JSONRPCMessage{JSONRPC: "2.0", ID: req.ID, Result: initResp}
		// Consume the initialized notification (no ID)
		<-mt.sendCh
	}()

	if err := client.initialize(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info := client.ServerInfo()
	if info.Name != "test-srv" {
		t.Errorf("expected test-srv, got %q", info.Name)
	}
	if !info.Capabilities.Tools {
		t.Error("expected tools capability")
	}
}

// ─── classifyConnError ────────────────────────────────────────────────────────

func TestClassifyConnError_Nil(t *testing.T) {
	if classifyConnError(nil) != StatusConnected {
		t.Error("expected connected for nil error")
	}
}

func TestClassifyConnError_Auth(t *testing.T) {
	err := fmt.Errorf("401 unauthorized")
	if classifyConnError(err) != StatusNeedsAuth {
		t.Error("expected needs-auth for 401 error")
	}
}

func TestClassifyConnError_Other(t *testing.T) {
	err := fmt.Errorf("connection refused")
	if classifyConnError(err) != StatusFailed {
		t.Error("expected failed for non-auth error")
	}
}

// ─── Pool.Connect edge cases ──────────────────────────────────────────────────

func TestPool_Connect_AlreadyConnected(t *testing.T) {
	p := NewPool()
	// Inject a connected connection
	p.mu.Lock()
	p.connections["srv"] = &ServerConnection{
		Name:   "srv",
		Status: StatusConnected,
	}
	p.mu.Unlock()

	// Connect again — should be a no-op
	err := p.Connect(context.Background(), "srv", ServerConfig{Transport: "unsupported"})
	if err != nil {
		t.Errorf("expected nil for already-connected server, got %v", err)
	}
}

func TestPool_Connect_SSEConfigNil(t *testing.T) {
	p := NewPool()
	err := p.Connect(context.Background(), "srv", ServerConfig{Transport: TransportSSE})
	if err == nil {
		t.Error("expected error for nil SSE config")
	}
}

func TestPool_Connect_HTTPConfigNil(t *testing.T) {
	p := NewPool()
	err := p.Connect(context.Background(), "srv", ServerConfig{Transport: TransportHTTP})
	if err == nil {
		t.Error("expected error for nil HTTP config")
	}
}

func TestPool_Connect_StdioConfigNil(t *testing.T) {
	p := NewPool()
	err := p.Connect(context.Background(), "srv", ServerConfig{Transport: TransportStdio})
	if err == nil {
		t.Error("expected error for nil stdio config")
	}
}

func TestPool_ID(t *testing.T) {
	conn := &ServerConnection{Name: "my-server"}
	if conn.ID() != "my-server" {
		t.Errorf("expected my-server, got %q", conn.ID())
	}
}

func TestPool_Reconnect_ClosesOldClient(t *testing.T) {
	p := NewPool()
	mc := &mockMCPClient{}
	p.mu.Lock()
	p.connections["srv"] = &ServerConnection{
		Name:   "srv",
		Status: StatusConnected,
		Client: mc,
		Config: ServerConfig{Transport: "unsupported"},
	}
	p.mu.Unlock()

	_ = p.Reconnect(context.Background(), "srv") // will fail to re-connect
	if !mc.closed {
		t.Error("expected old client to be closed during reconnect")
	}
}

// ─── jsonRPCClient recvErr propagation ───────────────────────────────────────

func TestJSONRPCClient_RecvError_Propagates(t *testing.T) {
	mt := newMockTransport()
	client := newJSONRPCClient(mt)

	// Close transport immediately so Recv returns error
	mt.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.call(ctx, "anything", nil)
	if err == nil {
		t.Error("expected error when transport is closed")
	}
}

// ─── Transport layer tests ────────────────────────────────────────────────────

func TestNewTransport_WrongConfigType(t *testing.T) {
	_, err := NewTransport(TransportStdio, "wrong-type")
	if err == nil {
		t.Error("expected error for wrong config type (stdio)")
	}
	_, err = NewTransport(TransportSSE, "wrong-type")
	if err == nil {
		t.Error("expected error for wrong config type (sse)")
	}
	_, err = NewTransport(TransportHTTP, "wrong-type")
	if err == nil {
		t.Error("expected error for wrong config type (http)")
	}
}

func TestNewTransport_WebSocket_NotImplemented(t *testing.T) {
	_, err := NewTransport(TransportWS, nil)
	if err == nil {
		t.Error("expected error for websocket transport")
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNewTransport_UnknownType(t *testing.T) {
	_, err := NewTransport("unknown", nil)
	if err == nil {
		t.Error("expected error for unknown transport type")
	}
}

func TestJSONRPCError_Error(t *testing.T) {
	e := &JSONRPCError{Code: -32600, Message: "Invalid Request"}
	s := e.Error()
	if !strings.Contains(s, "-32600") || !strings.Contains(s, "Invalid Request") {
		t.Errorf("unexpected Error() output: %q", s)
	}
}

func TestSSETransport_SendAndRecv(t *testing.T) {
	// Create a server that serves SSE events and accepts POST messages
	recvCh := make(chan string, 1)
	sseCh := make(chan string, 1)
	sseCh <- `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}` + "\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// SSE endpoint
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}
			msg := <-sseCh
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
			// Wait for close
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		} else if r.Method == http.MethodPost {
			recvCh <- "posted"
			w.WriteHeader(200)
		}
	}))
	defer server.Close()

	transport, err := newSSETransport(SSETransportConfig{URL: server.URL + "/sse"})
	if err != nil {
		t.Fatalf("newSSETransport error: %v", err)
	}
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Test Send (POST)
	msg := &JSONRPCMessage{JSONRPC: "2.0", Method: "test", ID: 1}
	if err := transport.Send(ctx, msg); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	select {
	case <-recvCh:
		// OK - POST was received
	case <-time.After(time.Second):
		t.Error("POST request not received")
	}

	// Test Recv (SSE)
	recvMsg, err := transport.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv error: %v", err)
	}
	if recvMsg == nil {
		t.Error("expected non-nil message")
	}
}

func TestSSETransport_Close_Idempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Hang until closed
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	transport, err := newSSETransport(SSETransportConfig{URL: server.URL})
	if err != nil {
		t.Fatalf("newSSETransport error: %v", err)
	}
	transport.Close()
	if err := transport.Close(); err != nil {
		t.Errorf("second Close should not error: %v", err)
	}
}

func TestSSETransport_Recv_ContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer server.Close()

	transport, err := newSSETransport(SSETransportConfig{URL: server.URL})
	if err != nil {
		t.Fatalf("newSSETransport error: %v", err)
	}
	defer transport.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	_, err = transport.Recv(ctx)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestHTTPTransport_SendAndRecv(t *testing.T) {
	// A server that accepts POST and returns an SSE response on GET
	postReceived := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postReceived <- struct{}{}
			w.Header().Set("Mcp-Session-Id", "sess-123")
			w.WriteHeader(200)
		} else if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n")
		}
	}))
	defer server.Close()

	transport, err := newHTTPTransport(HTTPTransportConfig{URL: server.URL + "/mcp"})
	if err != nil {
		t.Fatalf("newHTTPTransport error: %v", err)
	}
	defer transport.Close()

	ctx := context.Background()

	// Test Send – should set session ID from response
	msg := &JSONRPCMessage{JSONRPC: "2.0", Method: "test", ID: 1}
	if err := transport.Send(ctx, msg); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	select {
	case <-postReceived:
	case <-time.After(time.Second):
		t.Error("POST not received")
	}

	// Verify session ID was stored
	if sid, ok := transport.sessionID.Load().(string); !ok || sid != "sess-123" {
		t.Errorf("expected session ID 'sess-123', got %q", sid)
	}

	// Test Recv
	recvMsg, err := transport.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv error: %v", err)
	}
	if recvMsg == nil {
		t.Error("expected non-nil message from Recv")
	}
}

func TestHTTPTransport_CloseSetsClosed(t *testing.T) {
	transport, err := newHTTPTransport(HTTPTransportConfig{URL: "http://localhost"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
	if !transport.closed {
		t.Error("expected closed=true after Close()")
	}
}

func TestHTTPTransport_Recv_EOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty response (no SSE data → EOF)
		w.WriteHeader(200)
	}))
	defer server.Close()

	transport, err := newHTTPTransport(HTTPTransportConfig{URL: server.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer transport.Close()

	_, err = transport.Recv(context.Background())
	if err == nil {
		t.Error("expected EOF or error from empty response")
	}
}
