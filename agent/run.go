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

// RequirementKind classifies why execution is suspended.
type RequirementKind string

const (
	RequirementKindApproval          RequirementKind = "approval"
	RequirementKindUserInput         RequirementKind = "user_input"
	RequirementKindExternalExecution RequirementKind = "external_execution"
)

// RequirementOrigin identifies where a pending requirement was created.
type RequirementOrigin string

const (
	RequirementOriginModel RequirementOrigin = "model"
	RequirementOriginTool  RequirementOrigin = "tool"
)

// ResolutionOutcome captures how a pending requirement was resolved.
type ResolutionOutcome string

const (
	ResolutionOutcomeApproved  ResolutionOutcome = "approved"
	ResolutionOutcomeRejected  ResolutionOutcome = "rejected"
	ResolutionOutcomeProvided  ResolutionOutcome = "provided"
	ResolutionOutcomeCompleted ResolutionOutcome = "completed"
)

// PendingRequirement describes the requirement that currently blocks run progress.
type PendingRequirement struct {
	ID          string            `json:"id"`
	Kind        RequirementKind   `json:"kind"`
	Origin      RequirementOrigin `json:"origin"`
	ToolCallID  string            `json:"tool_call_id,omitempty"`
	Fingerprint string            `json:"fingerprint,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
}

// Resolution provides the typed payload required to continue a suspended run.
type Resolution struct {
	RequirementID string            `json:"requirement_id"`
	Kind          RequirementKind   `json:"kind"`
	Outcome       ResolutionOutcome `json:"outcome"`
	Value         string            `json:"value,omitempty"`
}

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
	ID                 RunID               `json:"id"`
	Version            int64               `json:"version"`
	Step               int                 `json:"step"`
	Status             RunStatus           `json:"status"`
	PendingRequirement *PendingRequirement `json:"pending_requirement,omitempty"`
	Output             string              `json:"output,omitempty"`
	Error              string              `json:"error,omitempty"`
	Messages           []Message           `json:"messages,omitempty"`
}

// CloneRunState returns a deep copy safe for in-memory stores.
func CloneRunState(in RunState) RunState {
	out := in
	if in.PendingRequirement != nil {
		requirementCopy := *in.PendingRequirement
		out.PendingRequirement = &requirementCopy
	}
	out.Messages = CloneMessages(in.Messages)
	return out
}

// RunResult is returned by the runtime API.
type RunResult struct {
	State RunState
}
