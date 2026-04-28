package agenttype

// RegisterBuiltins registers all built-in agent profiles into the registry.
// This is called during bootstrap (Phase 4.5) before custom agent loading.
func RegisterBuiltins(r *Registry) {
	for _, p := range builtinProfiles {
		_ = r.Register(p)
	}
}

// builtinProfiles defines the 5 built-in agent types with their configurations.
var builtinProfiles = []*AgentProfile{
	{
		Type:        AgentTypeWorker,
		DisplayName: "Worker",
		Description: "General-purpose agent for complex multi-step tasks.",
		WhenToUse: "Use when you need a sub-agent to execute general software " +
			"engineering tasks including reading, writing, and running code.",
		SystemPrompt: workerSystemPrompt,
		ToolFilter: ToolFilter{
			Mode:  ToolFilterDenylist,
			Tools: CoordinatorOnlyTools,
		},
		CanSpawnSubAgents: false,
	},
	{
		Type:        AgentTypeExplore,
		DisplayName: "Explorer",
		Description: "Fast agent specialized for exploring codebases.",
		WhenToUse: "Use when you need to quickly find files by patterns " +
			"(e.g. \"src/components/**/*.tsx\"), search code for keywords, " +
			"or answer questions about the codebase. Read-only.",
		SystemPrompt: exploreSystemPrompt,
		ToolFilter: ToolFilter{
			Mode:  ToolFilterAllowlist,
			Tools: []string{"Read", "Glob", "Grep", "Bash", "WebSearch", "WebFetch"},
		},
		CanSpawnSubAgents: false,
	},
	{
		Type:        AgentTypePlan,
		DisplayName: "Planner",
		Description: "Software architect agent for designing implementation plans.",
		WhenToUse: "Use when you need to plan the implementation strategy for " +
			"a task. Returns step-by-step plans, identifies critical files, " +
			"and considers architectural trade-offs. Read-only.",
		SystemPrompt: planSystemPrompt,
		ToolFilter: ToolFilter{
			Mode:  ToolFilterAllowlist,
			Tools: []string{"Read", "Glob", "Grep", "Bash", "WebSearch", "WebFetch"},
		},
		CanSpawnSubAgents: false,
	},
	{
		Type:        AgentTypeVerify,
		DisplayName: "Verifier",
		Description: "Verification agent that attempts to break implementations.",
		WhenToUse: "Use when you need to validate changes by running tests, " +
			"builds, and linters. Tries to find issues adversarially.",
		SystemPrompt: verifySystemPrompt,
		ToolFilter: ToolFilter{
			Mode:  ToolFilterAllowlist,
			Tools: []string{"Read", "Bash", "Grep", "Glob"},
		},
		CanSpawnSubAgents: false,
	},
	{
		Type:        AgentTypeGuide,
		DisplayName: "Guide",
		Description: "Expert on Claude Code CLI, Agent SDK, and Claude API.",
		WhenToUse: "Use when the user asks questions about Claude Code features, " +
			"tools, configuration, or best practices.",
		SystemPrompt: guideSystemPrompt,
		ToolFilter: ToolFilter{
			Mode:  ToolFilterAllowlist,
			Tools: []string{"Read", "Glob", "Grep", "WebSearch", "WebFetch"},
		},
		CanSpawnSubAgents: false,
	},
}
