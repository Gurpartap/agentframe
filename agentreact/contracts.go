package agentreact

import (
	"context"

	"agentruntime/agent"
)

// ModelRequest is the model input contract for the ReAct engine.
type ModelRequest struct {
	Messages []agent.Message
	Tools    []agent.ToolDefinition
}

// Model produces assistant messages that may include tool calls.
type Model interface {
	Generate(ctx context.Context, request ModelRequest) (agent.Message, error)
}

// ToolExecutor resolves and executes tool calls.
type ToolExecutor interface {
	Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error)
}
