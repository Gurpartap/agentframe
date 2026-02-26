package agentreact

import (
	"context"

	"github.com/Gurpartap/agentframe/agent"
)

// ModelRequest is the model input contract for the ReAct engine.
type ModelRequest struct {
	Messages   []agent.Message
	Tools      []agent.ToolDefinition
	Resolution *agent.Resolution
}

// Model produces assistant messages that may include tool calls.
type Model interface {
	Generate(ctx context.Context, request ModelRequest) (agent.Message, error)
}

// ToolExecutor resolves and executes tool calls.
type ToolExecutor interface {
	Execute(ctx context.Context, call agent.ToolCall) (agent.ToolResult, error)
}
