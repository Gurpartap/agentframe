package mocks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Gurpartap/agentframe/agent"
)

var toolDefinitions = []agent.ToolDefinition{
	{
		Name:        "mock_lookup",
		Description: "Deterministic mock lookup tool.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	},
}

// Tools is a deterministic mock tool executor.
type Tools struct{}

func NewTools() *Tools {
	return &Tools{}
}

func Definitions() []agent.ToolDefinition {
	return agent.CloneToolDefinitions(toolDefinitions)
}

func (t *Tools) Execute(_ context.Context, call agent.ToolCall) (agent.ToolResult, error) {
	if call.Name == "" {
		return agent.ToolResult{}, fmt.Errorf("mock tools: empty call name")
	}

	arguments := "{}"
	if len(call.Arguments) > 0 {
		encoded, err := json.Marshal(call.Arguments)
		if err != nil {
			return agent.ToolResult{}, fmt.Errorf("mock tools: encode arguments: %w", err)
		}
		arguments = string(encoded)
	}

	return agent.ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: fmt.Sprintf("mock_tool_result name=%s args=%s", call.Name, arguments),
	}, nil
}
