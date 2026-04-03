package mcp

import (
	"context"
	"fmt"
	"sync"
)

// ConnectionStatus enumerates the states of an MCP server connection.
type ConnectionStatus string

const (
	StatusConnected ConnectionStatus = "connected"
	StatusFailed    ConnectionStatus = "failed"
	StatusNeedsAuth ConnectionStatus = "needs-auth"
	StatusPending   ConnectionStatus = "pending"
	StatusDisabled  ConnectionStatus = "disabled"
)

// MaxMCPDescriptionLength is the maximum length for MCP tool descriptions.
const MaxMCPDescriptionLength = 2048

// DefaultToolTimeout is the default MCP tool call timeout (mirrors TS DEFAULT_MCP_TOOL_TIMEOUT_MS).
// In practice, caller-provided context deadlines take precedence.
const DefaultToolTimeout = 100_000_000 // ms — ~27.8h

// ServerConfig aggregates MCP server configuration.
type ServerConfig struct {
	Transport TransportType
	Stdio     *StdioTransportConfig
	SSE       *SSETransportConfig
	HTTP      *HTTPTransportConfig
	Scope     string // "local" | "user" | "project" | "enterprise"
}

// ServerConnection represents a complete connection record for a single MCP server.
type ServerConnection struct {
	Name             string
	Status           ConnectionStatus
	Config           ServerConfig
	Client           MCPClient // nil when Status != connected
	Error            string    // set when Status == failed
	ReconnectAttempt int
}

// ID implements types.MCPConnection.
func (s *ServerConnection) ID() string {
	return s.Name
}

// IsConnected implements types.MCPConnection.
func (s *ServerConnection) IsConnected() bool {
	return s.Status == StatusConnected
}

// Pool manages the lifecycle of connections to multiple MCP servers.
type Pool struct {
	mu          sync.RWMutex
	connections map[string]*ServerConnection
}

// NewPool creates an empty connection pool.
func NewPool() *Pool {
	return &Pool{
		connections: make(map[string]*ServerConnection),
	}
}

// Connect establishes a connection to a named MCP server (idempotent).
// If a connection already exists and is in the connected state, it returns immediately.
func (p *Pool) Connect(ctx context.Context, name string, cfg ServerConfig) error {
	p.mu.Lock()
	existing, ok := p.connections[name]
	if ok && existing.Status == StatusConnected {
		p.mu.Unlock()
		return nil
	}
	conn := &ServerConnection{
		Name:   name,
		Status: StatusPending,
		Config: cfg,
	}
	p.connections[name] = conn
	p.mu.Unlock()

	client, err := dialServer(ctx, cfg)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		conn.Status = classifyConnError(err)
		conn.Error = err.Error()
		return err
	}
	conn.Client = client
	conn.Status = StatusConnected
	conn.Error = ""
	return nil
}

// Reconnect forcibly re-establishes a connection.
func (p *Pool) Reconnect(ctx context.Context, name string) error {
	p.mu.Lock()
	conn, ok := p.connections[name]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("mcp: pool: server %q not found", name)
	}
	cfg := conn.Config
	// Close old client
	if conn.Client != nil {
		_ = conn.Client.Close()
	}
	conn.Status = StatusPending
	conn.Client = nil
	conn.ReconnectAttempt++
	p.mu.Unlock()

	return p.Connect(ctx, name, cfg)
}

// Disconnect closes a connection and marks it as disabled.
func (p *Pool) Disconnect(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn, ok := p.connections[name]
	if !ok {
		return nil
	}
	if conn.Client != nil {
		_ = conn.Client.Close()
	}
	conn.Status = StatusDisabled
	conn.Client = nil
	return nil
}

// GetConnected returns all connections in the connected state.
func (p *Pool) GetConnected() []*ServerConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var result []*ServerConnection
	for _, c := range p.connections {
		if c.Status == StatusConnected {
			result = append(result, c)
		}
	}
	return result
}

// GetAll returns all connections regardless of status.
func (p *Pool) GetAll() []*ServerConnection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*ServerConnection, 0, len(p.connections))
	for _, c := range p.connections {
		result = append(result, c)
	}
	return result
}

// dialServer creates a Transport and performs the MCP handshake.
func dialServer(ctx context.Context, cfg ServerConfig) (MCPClient, error) {
	var transport Transport
	var err error

	switch cfg.Transport {
	case TransportStdio:
		if cfg.Stdio == nil {
			return nil, fmt.Errorf("mcp: pool: stdio config is nil")
		}
		transport, err = NewTransport(TransportStdio, *cfg.Stdio)
	case TransportSSE:
		if cfg.SSE == nil {
			return nil, fmt.Errorf("mcp: pool: sse config is nil")
		}
		transport, err = NewTransport(TransportSSE, *cfg.SSE)
	case TransportHTTP:
		if cfg.HTTP == nil {
			return nil, fmt.Errorf("mcp: pool: http config is nil")
		}
		transport, err = NewTransport(TransportHTTP, *cfg.HTTP)
	default:
		return nil, fmt.Errorf("mcp: pool: unsupported transport: %s", cfg.Transport)
	}
	if err != nil {
		return nil, err
	}

	client := newJSONRPCClient(transport)
	if err := client.initialize(ctx); err != nil {
		_ = transport.Close()
		return nil, err
	}
	return client, nil
}

// classifyConnError maps a connection error to the appropriate ConnectionStatus.
func classifyConnError(err error) ConnectionStatus {
	if err == nil {
		return StatusConnected
	}
	// Auth errors → NeedsAuth
	if isMCPAuthError(err) {
		return StatusNeedsAuth
	}
	return StatusFailed
}

// isMCPAuthError detects authentication errors from MCP connections.
func isMCPAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, keyword := range []string{"401", "403", "unauthorized", "auth", "token"} {
		for i := 0; i < len(msg)-len(keyword)+1; i++ {
			if msg[i:i+len(keyword)] == keyword {
				return true
			}
		}
	}
	return false
}

// truncateDescription truncates s to MaxMCPDescriptionLength runes.
func truncateDescription(s string) string {
	runes := []rune(s)
	if len(runes) <= MaxMCPDescriptionLength {
		return s
	}
	return string(runes[:MaxMCPDescriptionLength])
}
