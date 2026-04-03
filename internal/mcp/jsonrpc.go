package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// jsonRPCClient implements MCPClient over any Transport using JSON-RPC 2.0.
type jsonRPCClient struct {
	transport  Transport
	serverInfo ServerInfo
	nextID     atomic.Int64
	mu         sync.Mutex
	pending    map[int64]chan *JSONRPCMessage
	recvErr    chan error // buffered(1); background reader writes the first error then exits
	startOnce  sync.Once // ensures startReader is called only once
}

func newJSONRPCClient(transport Transport) *jsonRPCClient {
	c := &jsonRPCClient{
		transport: transport,
		pending:   make(map[int64]chan *JSONRPCMessage),
		recvErr:   make(chan error, 1),
	}
	return c
}

// startReader launches a single background goroutine (at most once) that owns
// the transport's Recv loop and dispatches responses to the appropriate pending
// channel. This prevents multiple concurrent call() invocations from racing on
// the underlying bufio.Reader.
func (c *jsonRPCClient) startReader(ctx context.Context) {
	c.startOnce.Do(func() {
		go func() {
			for {
				msg, err := c.transport.Recv(ctx)
				if err != nil {
					select {
					case c.recvErr <- err:
					default:
					}
					return
				}
				// Determine the response ID.
				var respID int64
				if msg.ID != nil {
					switch v := msg.ID.(type) {
					case float64:
						respID = int64(v)
					case int64:
						respID = v
					}
				}
				c.mu.Lock()
				target, ok := c.pending[respID]
				c.mu.Unlock()
				if ok {
					select {
					case target <- msg:
					default:
					}
				}
			}
		}()
	})
}

// initialize performs the MCP handshake and returns the server info.
func (c *jsonRPCClient) initialize(ctx context.Context) error {
	params, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo": map[string]any{
			"name":    "claude-code-go",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{},
	})
	resp, err := c.call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("mcp: initialize: %w", err)
	}
	var result struct {
		ProtocolVersion string     `json:"protocolVersion"`
		ServerInfo      ServerInfo `json:"serverInfo"`
		Capabilities    struct {
			Tools     *struct{} `json:"tools"`
			Resources *struct{} `json:"resources"`
			Prompts   *struct{} `json:"prompts"`
		} `json:"capabilities"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("mcp: initialize: unmarshal: %w", err)
	}
	c.serverInfo = result.ServerInfo
	c.serverInfo.Capabilities.Tools = result.Capabilities.Tools != nil
	c.serverInfo.Capabilities.Resources = result.Capabilities.Resources != nil
	c.serverInfo.Capabilities.Prompts = result.Capabilities.Prompts != nil
	c.serverInfo.Instructions = result.Instructions

	// Send initialized notification
	notif := &JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.transport.Send(ctx, notif)
}

// call sends a JSON-RPC request and waits for the response.
func (c *jsonRPCClient) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan *JSONRPCMessage, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	msg := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := c.transport.Send(ctx, msg); err != nil {
		return nil, err
	}

	// Ensure the single background reader is running before we wait.
	c.startReader(ctx)

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case err := <-c.recvErr:
		// Propagate the error to other pending callers, then return.
		select {
		case c.recvErr <- err:
		default:
		}
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ListTools implements MCPClient.
func (c *jsonRPCClient) ListTools(ctx context.Context) ([]MCPToolDef, error) {
	result, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: list tools: %w", err)
	}
	var resp struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mcp: list tools: unmarshal: %w", err)
	}
	tools := make([]MCPToolDef, len(resp.Tools))
	for i, t := range resp.Tools {
		tools[i] = MCPToolDef{
			Name:        t.Name,
			Description: truncateDescription(t.Description),
			InputSchema: t.InputSchema,
		}
	}
	return tools, nil
}

// CallTool implements MCPClient.
func (c *jsonRPCClient) CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error) {
	params, err := json.Marshal(map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp: call tool: marshal: %w", err)
	}
	result, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("mcp: call tool: %w", err)
	}
	var resp struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Data     string `json:"data"`
			MIMEType string `json:"mimeType"`
		} `json:"content"`
		IsError bool           `json:"isError"`
		Meta    map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mcp: call tool: unmarshal: %w", err)
	}
	content := make([]MCPContent, len(resp.Content))
	for i, c := range resp.Content {
		content[i] = MCPContent{
			Type:     c.Type,
			Text:     c.Text,
			Data:     c.Data,
			MIMEType: c.MIMEType,
		}
	}
	return &MCPToolResult{
		Content: content,
		IsError: resp.IsError,
		Meta:    resp.Meta,
	}, nil
}

// ListResources implements MCPClient.
func (c *jsonRPCClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	result, err := c.call(ctx, "resources/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: list resources: %w", err)
	}
	var resp struct {
		Resources []struct {
			URI         string `json:"uri"`
			Name        string `json:"name"`
			Description string `json:"description"`
			MIMEType    string `json:"mimeType"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mcp: list resources: unmarshal: %w", err)
	}
	resources := make([]MCPResource, len(resp.Resources))
	for i, r := range resp.Resources {
		resources[i] = MCPResource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MIMEType:    r.MIMEType,
		}
	}
	return resources, nil
}

// ReadResource implements MCPClient.
func (c *jsonRPCClient) ReadResource(ctx context.Context, uri string) (*MCPResourceContent, error) {
	params, _ := json.Marshal(map[string]string{"uri": uri})
	result, err := c.call(ctx, "resources/read", params)
	if err != nil {
		return nil, fmt.Errorf("mcp: read resource: %w", err)
	}
	var resp struct {
		Contents []struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
			Text     string `json:"text"`
			Blob     string `json:"blob"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("mcp: read resource: unmarshal: %w", err)
	}
	if len(resp.Contents) == 0 {
		return nil, fmt.Errorf("mcp: read resource: empty contents")
	}
	rc := resp.Contents[0]
	return &MCPResourceContent{
		URI:      rc.URI,
		MIMEType: rc.MIMEType,
		Text:     rc.Text,
		Blob:     rc.Blob,
	}, nil
}

// Ping implements MCPClient.
func (c *jsonRPCClient) Ping(ctx context.Context) error {
	_, err := c.call(ctx, "ping", nil)
	return err
}

// Close implements MCPClient.
func (c *jsonRPCClient) Close() error {
	return c.transport.Close()
}

// ServerInfo implements MCPClient.
func (c *jsonRPCClient) ServerInfo() ServerInfo {
	return c.serverInfo
}
