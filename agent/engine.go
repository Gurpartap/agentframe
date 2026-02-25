package agent

import "context"

// Engine executes run state transitions for one runtime execution slice.
type Engine interface {
	Execute(ctx context.Context, state RunState, input EngineInput) (RunState, error)
}

// EngineInput provides execution constraints and tool contracts.
type EngineInput struct {
	MaxSteps int
	Tools    []ToolDefinition
}
