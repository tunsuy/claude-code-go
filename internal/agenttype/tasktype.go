package agenttype

// TaskType classifies the execution mode of a task.
type TaskType string

const (
	// TaskTypeLocalAgent is a task backed by an in-process sub-agent.
	TaskTypeLocalAgent TaskType = "local_agent"
	// TaskTypeLocalBash is a task backed by a local shell command.
	TaskTypeLocalBash TaskType = "local_bash"
	// TaskTypeRemote is a task backed by a remote agent.
	TaskTypeRemote TaskType = "remote"
	// TaskTypeInProcessTeammate is a task backed by an in-process teammate (Swarm).
	TaskTypeInProcessTeammate TaskType = "in_process_teammate"
)

// String returns the string representation of the task type.
func (t TaskType) String() string { return string(t) }
