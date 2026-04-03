package bootstrap

import (
	"github.com/anthropics/claude-code-go/internal/tool"
	"github.com/anthropics/claude-code-go/internal/tools/agent"
	"github.com/anthropics/claude-code-go/internal/tools/fileops"
	"github.com/anthropics/claude-code-go/internal/tools/interact"
	"github.com/anthropics/claude-code-go/internal/tools/misc"
	"github.com/anthropics/claude-code-go/internal/tools/mcp"
	"github.com/anthropics/claude-code-go/internal/tools/shell"
	"github.com/anthropics/claude-code-go/internal/tools/tasks"
	"github.com/anthropics/claude-code-go/internal/tools/web"
)

// RegisterBuiltinTools registers all built-in tool singletons into reg.
// Order follows the canonical tool ordering in the TS implementation.
func RegisterBuiltinTools(reg *tool.Registry) {
	// ── File operations ──────────────────────────────────────────────────────
	reg.Register(fileops.FileReadTool)
	reg.Register(fileops.FileWriteTool)
	reg.Register(fileops.FileEditTool)
	reg.Register(fileops.NotebookEditTool)
	reg.Register(fileops.GrepTool)
	reg.Register(fileops.GlobTool)

	// ── Shell ────────────────────────────────────────────────────────────────
	reg.Register(shell.BashTool)

	// ── Web ──────────────────────────────────────────────────────────────────
	reg.Register(web.WebSearchTool)
	reg.Register(web.WebFetchTool)

	// ── Task management ──────────────────────────────────────────────────────
	reg.Register(tasks.TaskCreateTool)
	reg.Register(tasks.TaskGetTool)
	reg.Register(tasks.TaskListTool)
	reg.Register(tasks.TaskUpdateTool)
	reg.Register(tasks.TaskStopTool)
	reg.Register(tasks.TaskOutputTool)

	// ── Agent ────────────────────────────────────────────────────────────────
	reg.Register(agent.AgentTool)
	reg.Register(agent.SendMessageTool)

	// ── Interaction / plan mode ──────────────────────────────────────────────
	reg.Register(interact.TodoWriteTool)
	reg.Register(interact.AskUserQuestionTool)
	reg.Register(interact.EnterPlanModeTool)
	reg.Register(interact.ExitPlanModeTool)
	reg.Register(interact.EnterWorktreeTool)
	reg.Register(interact.ExitWorktreeTool)

	// ── MCP meta-tools ───────────────────────────────────────────────────────
	reg.Register(mcp.ListMcpResourcesTool)
	reg.Register(mcp.ReadMcpResourceTool)

	// ── Misc ─────────────────────────────────────────────────────────────────
	reg.Register(misc.SkillTool)
	reg.Register(misc.BriefTool)
	reg.Register(misc.ToolSearchTool)
	reg.Register(misc.SleepTool)
	reg.Register(misc.SyntheticOutputTool)
}
