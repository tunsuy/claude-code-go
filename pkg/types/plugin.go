// Package types defines the shared types for claude-code-go.
// All types in this package are stable once reviewed; changes require
// cross-package coordination. Do NOT add non-stdlib imports.
package types

// PluginConfig is the configuration block for a single plugin in settings.json.
type PluginConfig struct {
	Name    string            `json:"name"`
	Enabled bool              `json:"enabled"`
	Options map[string]string `json:"options,omitempty"`
}

// PluginError represents an error that occurred during plugin loading or execution.
type PluginError struct {
	PluginName string `json:"pluginName"`
	Message    string `json:"message"`
	Fatal      bool   `json:"fatal,omitempty"`
}

func (e *PluginError) Error() string {
	return "plugin " + e.PluginName + ": " + e.Message
}

// LoadedPlugin represents a successfully loaded and active plugin.
type LoadedPlugin struct {
	Config   PluginConfig `json:"config"`
	// Commands is the list of commands contributed by this plugin.
	Commands []LocalCommand `json:"-"`
	// Hooks is the set of hook definitions contributed by this plugin.
	Hooks map[HookType][]HookDefinition `json:"-"`
}

// MCPConnection is a forward-declaration interface for an MCP server connection.
// The concrete implementation lives in the services layer (Agent-Services).
// Using an interface here avoids import cycles and allows the state layer to
// hold typed references without depending on the MCP implementation.
type MCPConnection interface {
	// ID returns the unique identifier for this connection.
	ID() string
	// IsConnected reports whether the connection is currently active.
	IsConnected() bool
}
