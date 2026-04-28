package agenttype

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadCustomAgents scans dir for .md and .json agent definitions.
// Returns the parsed profiles; invalid files are logged and skipped.
// Returns nil, nil if the directory does not exist.
func LoadCustomAgents(dir string) ([]*AgentProfile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("agenttype: read agents dir: %w", err)
	}

	var profiles []*AgentProfile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(dir, name)

		var profile *AgentProfile
		var parseErr error

		switch {
		case strings.HasSuffix(name, ".json"):
			profile, parseErr = parseJSONAgent(path)
		case strings.HasSuffix(name, ".md"):
			profile, parseErr = parseMarkdownAgent(path)
		default:
			continue // skip unknown extensions
		}

		if parseErr != nil {
			// Log and skip invalid files.
			fmt.Fprintf(os.Stderr, "warning: skipping invalid agent definition %s: %v\n", path, parseErr)
			continue
		}

		if profile.Type == "" {
			// Generate a type from the filename.
			base := strings.TrimSuffix(name, filepath.Ext(name))
			profile.Type = AgentType(base)
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

// jsonAgentDef is the on-disk JSON format for custom agent definitions.
type jsonAgentDef struct {
	Name              string     `json:"name"`
	DisplayName       string     `json:"display_name"`
	Description       string     `json:"description"`
	WhenToUse         string     `json:"when_to_use"`
	SystemPrompt      string     `json:"system_prompt"`
	Model             string     `json:"model"`
	MaxTurns          int        `json:"max_turns"`
	Tools             ToolFilter `json:"tools"`
	CanSpawnSubAgents bool       `json:"can_spawn_sub_agents"`
}

// parseJSONAgent parses a JSON agent definition file.
func parseJSONAgent(path string) (*AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var def jsonAgentDef
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	return &AgentProfile{
		Type:              AgentType(def.Name),
		DisplayName:       def.DisplayName,
		Description:       def.Description,
		WhenToUse:         def.WhenToUse,
		SystemPrompt:      def.SystemPrompt,
		Model:             def.Model,
		MaxTurns:          def.MaxTurns,
		ToolFilter:        def.Tools,
		CanSpawnSubAgents: def.CanSpawnSubAgents,
	}, nil
}

// parseMarkdownAgent extracts YAML-like frontmatter + body from a markdown file.
//
// Format:
//
//	---
//	name: my-agent
//	display_name: My Agent
//	model: haiku
//	max_turns: 10
//	---
//	System prompt body goes here...
func parseMarkdownAgent(path string) (*AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	content := string(data)

	// Extract frontmatter between --- delimiters.
	if !strings.HasPrefix(content, "---") {
		// No frontmatter — treat entire file as system prompt.
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		return &AgentProfile{
			Type:         AgentType(base),
			DisplayName:  base,
			SystemPrompt: strings.TrimSpace(content),
			ToolFilter:   ToolFilter{Mode: ToolFilterDenylist, Tools: CoordinatorOnlyTools},
		}, nil
	}

	// Find end of frontmatter.
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return nil, fmt.Errorf("unclosed frontmatter (missing closing ---)")
	}

	frontmatter := strings.TrimSpace(rest[:endIdx])
	body := strings.TrimSpace(rest[endIdx+4:])

	// Parse frontmatter as simple key: value pairs (not full YAML).
	profile := &AgentProfile{
		SystemPrompt: body,
		ToolFilter:   ToolFilter{Mode: ToolFilterDenylist, Tools: CoordinatorOnlyTools},
	}

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(line[colonIdx+1:])

		switch key {
		case "name":
			profile.Type = AgentType(val)
		case "display_name":
			profile.DisplayName = val
		case "description":
			profile.Description = val
		case "when_to_use":
			profile.WhenToUse = val
		case "model":
			profile.Model = val
		case "max_turns":
			var n int
			if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
				profile.MaxTurns = n
			}
		}
	}

	return profile, nil
}
