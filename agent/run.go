package agent

// RunID is the stable identifier for a runtime execution.
type RunID string

// RunStatus captures coarse execution state for persistence and orchestration.
type RunStatus string

const (
	RunStatusPending          RunStatus = "pending"
	RunStatusRunning          RunStatus = "running"
	RunStatusSuspended        RunStatus = "suspended"
	RunStatusCancelled        RunStatus = "cancelled"
	RunStatusCompleted        RunStatus = "completed"
	RunStatusFailed           RunStatus = "failed"
	RunStatusMaxStepsExceeded RunStatus = "max_steps_exceeded"
)

// RunInput configures a fresh run.
type RunInput struct {
	RunID        RunID
	SystemPrompt string
	UserPrompt   string
	MaxSteps     int
	Tools        []ToolDefinition
}

// RunState is the durable runtime state.
type RunState struct {
	ID       RunID     `json:"id"`
	Step     int       `json:"step"`
	Status   RunStatus `json:"status"`
	Output   string    `json:"output,omitempty"`
	Error    string    `json:"error,omitempty"`
	Messages []Message `json:"messages,omitempty"`
}

// CloneRunState returns a deep copy safe for in-memory stores.
func CloneRunState(in RunState) RunState {
	out := in
	out.Messages = CloneMessages(in.Messages)
	return out
}

// RunResult is returned by the runtime API.
type RunResult struct {
	State RunState
}
