package agenttype

// TaskType classifies the execution mode of a task.
type TaskType string

const (
	// TaskTypeLocalAgent is a task backed by an in-process sub-agent.
	TaskTypeLocalAgent TaskType = "local_agent"
	// TaskTypeLocalBash is a task backed by a local shell command.
	TaskTypeLocalBash TaskType = "local_bash"
	// TaskTypeRemoteAgent is a task backed by a remote agent.
	TaskTypeRemoteAgent TaskType = "remote_agent"
	// TaskTypeInProcessTeammate is a task backed by an in-process teammate (Swarm).
	TaskTypeInProcessTeammate TaskType = "in_process_teammate"
	// TaskTypeLocalWorkflow is a task backed by a local workflow.
	TaskTypeLocalWorkflow TaskType = "local_workflow"
	// TaskTypeMonitorMCP is a task backed by an MCP monitor.
	TaskTypeMonitorMCP TaskType = "monitor_mcp"
	// TaskTypeDream is a background dream task.
	TaskTypeDream TaskType = "dream"
)

// String returns the string representation of the task type.
func (t TaskType) String() string { return string(t) }
