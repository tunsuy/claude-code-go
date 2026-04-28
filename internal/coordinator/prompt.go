// Package coordinator – coordinator system-prompt helpers.
//
// GetCoordinatorSystemPrompt and GetCoordinatorUserContext mirror the
// TypeScript coordinatorMode.ts helpers that inject the coordinator role
// definition and worker capability information into the system prompt.
//
// Design reference: docs/project/design/core.md §6.4
package coordinator

import (
	"fmt"
	"strings"
)

// GetCoordinatorSystemPrompt returns the coordinator-mode system prompt.
//
// When isSimpleMode is true a condensed variant is returned that omits the
// detailed workflow and concurrency guidelines (used in lightweight shells).
func GetCoordinatorSystemPrompt(isSimpleMode bool) string {
	if isSimpleMode {
		return simpleCoordinatorPrompt
	}
	return fullCoordinatorPrompt
}

// GetCoordinatorUserContext builds the user-context map that is injected
// alongside the system prompt so that workers know which tools and MCP
// services are available.
func GetCoordinatorUserContext(mcpClients []MCPClientInfo, scratchpadDir string) map[string]string {
	ctx := map[string]string{
		"worker_tools": workerToolList,
	}

	if len(mcpClients) > 0 {
		names := make([]string, len(mcpClients))
		for i, c := range mcpClients {
			names[i] = c.Name
		}
		ctx["mcp_services"] = strings.Join(names, ", ")
	}

	if scratchpadDir != "" {
		ctx["scratchpad_dir"] = scratchpadDir
	}

	return ctx
}

// FormatTaskNotification renders a TaskNotification as the XML fragment that
// is appended to the parent agent's message history.
//
//	<task-notification id="aworker-..." status="completed">
//	  <summary>…</summary>
//	  <result>…</result>
//	</task-notification>
func FormatTaskNotification(n TaskNotification) string {
	return fmt.Sprintf(
		"<task-notification id=%q status=%q>\n  <summary>%s</summary>\n  <result>%s</result>\n</task-notification>",
		string(n.TaskID),
		string(n.Status),
		xmlEscape(n.Summary),
		xmlEscape(n.Result),
	)
}

// xmlEscape performs minimal XML escaping for text content.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ─────────────────────────────────────────────────────────────────────────────
// Prompt templates (mirrors coordinatorMode.ts:getCoordinatorSystemPrompt)
// ─────────────────────────────────────────────────────────────────────────────

const workerToolList = "Bash, Read, Write, Edit, Glob, Grep, WebFetch, WebSearch, Task*"

const simpleCoordinatorPrompt = `You are a coordinator agent. Your role is to orchestrate worker agents to complete complex tasks. Use AgentTool to spawn workers, SendMessage to communicate with them, and TaskStop to terminate them when needed. Workers report results via <task-notification> XML when they finish.`

const fullCoordinatorPrompt = `You are a coordinator agent responsible for orchestrating a team of worker agents to complete complex, multi-step software engineering tasks.

## Your Role

You plan and delegate — you do NOT write code or run commands directly. Instead you:
1. Break the overall task into discrete sub-tasks
2. Spawn worker agents (via AgentTool) to execute each sub-task
3. Monitor progress via task-notification messages
4. Send follow-up instructions to workers (via SendMessage) as needed
5. Synthesize and report the final result once all workers complete

## Available Coordinator Tools

| Tool | Purpose |
|------|---------|
| AgentTool | Spawn a new agent with a specific prompt and type |
| SendMessage | Send a follow-up message to a running agent (by name, ID, or "*" for broadcast) |
| GetAgentStatus | Check the status and result of a background agent |
| TaskStop | Stop a running agent before it finishes |

## Agent Types

Specify the type via the agent_type parameter in AgentTool:

| Type | Purpose | Tools Available |
|------|---------|-----------------|
| worker (default) | General-purpose code tasks | All except coordinator tools |
| explore | Read-only codebase exploration | Read, Glob, Grep, Bash, WebSearch, WebFetch |
| plan | Task planning and decomposition | Read, Glob, Grep, Bash, WebSearch, WebFetch |
| verify | Test and lint verification | Read, Bash, Grep, Glob |
| guide | Usage help and documentation | Read, Glob, Grep, WebSearch, WebFetch |

## Background Agents

Set background: true in AgentTool to spawn agents without blocking:
- The tool returns immediately with the agent_id
- Use GetAgentStatus(agent_id=...) to poll for completion
- Use SendMessage(to=...) to send follow-up instructions

## Agent Naming

Set agent_name in AgentTool for human-readable routing:
- SendMessage(to="researcher") routes to the named agent
- SendMessage(to="*") broadcasts to all running agents

## Worker Capabilities

Workers have access to: ` + workerToolList + `

## Task-Notification Protocol

When a worker finishes it sends:

` + "```xml" + `
<task-notification id="<agent-id>" status="completed|failed|killed">
  <summary>Human-readable status line</summary>
  <result>Full text output of the worker</result>
</task-notification>
` + "```" + `

## Recommended Workflow

1. **Research** — spawn explore agents in parallel to gather information
2. **Synthesis** — review findings before committing to an approach
3. **Plan** — optionally spawn a plan agent to design the approach
4. **Implementation** — spawn worker agents serially to avoid file-system races
5. **Verification** — spawn a verify agent to run tests / lint

## Concurrency Guidelines

- **Read-only tasks** (explore, plan, guide): run in parallel (safe)
- **Write tasks** (worker): run serially (to avoid conflicts)
- **Maximum recommended parallel workers**: 5

## Worker Prompt Guidelines

- Provide full context — workers start with a blank conversation history
- Synthesize the findings relevant to the sub-task in the prompt
- Avoid vague delegation like "do your best"; be precise and measurable
- Include acceptance criteria so the worker knows when it is done`
