package main

import "strings"

// Layer represents an architecture layer.
type Layer struct {
	Name  string // e.g. "core", "tools"
	Label string // e.g. "Core", "Tools"
	Order int    // lower = lower layer
}

var layers = []Layer{
	{Name: "types", Label: "Types (zero-dep)", Order: 0},
	{Name: "infra", Label: "Infra", Order: 1},
	{Name: "services", Label: "Services", Order: 2},
	{Name: "core", Label: "Core", Order: 3},
	{Name: "tools", Label: "Tools", Order: 4},
	{Name: "tui", Label: "TUI", Order: 5},
	{Name: "cli", Label: "CLI", Order: 6},
}

// layerMap maps package import sub-paths to layer names.
// The keys are matched as prefixes against the package path
// relative to the module root (e.g. "internal/coordinator").
var layerMap = map[string]string{
	// CLI layer
	"cmd/claude":          "cli",
	"internal/bootstrap":  "cli",
	"internal/commands":   "cli",

	// TUI layer
	"internal/tui": "tui",

	// Tools layer
	"internal/tools": "tools",

	// Core layer
	"internal/engine":      "core",
	"internal/coordinator": "core",

	// Services layer
	"internal/api":         "services",
	"internal/oauth":       "services",
	"internal/mcp":         "services",
	"internal/compact":     "services",
	"internal/memdir":      "services",
	"internal/permissions": "services",
	"internal/msgqueue":    "services",

	// Infra layer
	"internal/config":    "infra",
	"internal/session":   "infra",
	"internal/state":     "infra",
	"internal/hooks":     "infra",
	"internal/agentctx":  "infra",
	"internal/agenttype": "infra",
	"internal/plugin":    "infra",

	// Types layer (zero-dependency)
	"pkg/types":    "types",
	"pkg/utils":    "types",
	"pkg/testutil": "types",
}

// resolveLayer returns the layer name for the given package path.
// It tries longest-prefix matching so "internal/tools/agent" matches
// "internal/tools" rather than requiring an exact entry.
func resolveLayer(pkgPath string) string {
	// Try exact match first.
	if layer, ok := layerMap[pkgPath]; ok {
		return layer
	}
	// Try prefix match (longest wins).
	best := ""
	bestLen := 0
	for prefix, layer := range layerMap {
		if strings.HasPrefix(pkgPath, prefix) && len(prefix) > bestLen {
			best = layer
			bestLen = len(prefix)
		}
	}
	if best != "" {
		return best
	}
	return "unknown"
}

// layerLabel returns the human-readable label for a layer name.
func layerLabel(name string) string {
	for _, l := range layers {
		if l.Name == name {
			return l.Label
		}
	}
	return name
}
