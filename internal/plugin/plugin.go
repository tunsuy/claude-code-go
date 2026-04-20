// Package plugin manages plugin loading and lifecycle.
package plugin

import (
	"fmt"

	"github.com/tunsuy/claude-code-go/pkg/types"
)

// Manager loads and manages the plugin set.
type Manager struct {
	enabled  []types.LoadedPlugin
	disabled []types.LoadedPlugin
}

// NewManager creates a Manager from a list of PluginConfig entries.
// Plugins with Enabled == false are tracked but not initialised.
func NewManager(configs []types.PluginConfig) (*Manager, []error) {
	m := &Manager{}
	var errs []error
	for _, cfg := range configs {
		if !cfg.Enabled {
			m.disabled = append(m.disabled, types.LoadedPlugin{Config: cfg})
			continue
		}
		lp, err := loadPlugin(cfg)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		m.enabled = append(m.enabled, *lp)
	}
	return m, errs
}

// Enabled returns the list of successfully loaded plugins.
func (m *Manager) Enabled() []types.LoadedPlugin { return m.enabled }

// Disabled returns the list of plugins that were configured but not enabled.
func (m *Manager) Disabled() []types.LoadedPlugin { return m.disabled }

// loadPlugin initialises a single plugin.
// TODO(dep): full plugin loading (dynamic library / wasm / etc.) once the
// plugin subsystem design is finalised.
func loadPlugin(cfg types.PluginConfig) (*types.LoadedPlugin, error) {
	if cfg.Name == "" {
		return nil, &types.PluginError{PluginName: "(unnamed)", Message: "plugin name must not be empty", Fatal: true}
	}
	return &types.LoadedPlugin{
		Config: cfg,
		// Commands and Hooks are populated by the plugin's own init logic.
		// This stub leaves them empty until the plugin subsystem is implemented.
	}, nil
}

// EnsureNoFatalErrors returns an error if any of the provided errors is a
// fatal PluginError.
func EnsureNoFatalErrors(errs []error) error {
	for _, err := range errs {
		var pe *types.PluginError
		if ok := asFatal(err, &pe); ok {
			return fmt.Errorf("fatal plugin error: %w", err)
		}
	}
	return nil
}

// asFatal checks if err is a *PluginError with Fatal == true.
func asFatal(err error, target **types.PluginError) bool {
	var pe *types.PluginError
	if !asPluginError(err, &pe) {
		return false
	}
	*target = pe
	return pe.Fatal
}

// asPluginError is a manual errors.As for *PluginError (avoids importing errors).
func asPluginError(err error, target **types.PluginError) bool {
	if err == nil {
		return false
	}
	if pe, ok := err.(*types.PluginError); ok {
		*target = pe
		return true
	}
	return false
}
