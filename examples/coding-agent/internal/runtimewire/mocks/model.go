package mocks

import (
	"context"
	"fmt"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
)

// Model is a deterministic mock model implementation.
type Model struct{}

func NewModel() *Model {
	return &Model{}
}

func (m *Model) Generate(_ context.Context, request agentreact.ModelRequest) (agent.Message, error) {
	return agent.Message{
		Role:    agent.RoleAssistant,
		Content: deterministicContent(request),
	}, nil
}

func deterministicContent(request agentreact.ModelRequest) string {
	latestUser := ""
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if request.Messages[i].Role == agent.RoleUser {
			latestUser = request.Messages[i].Content
			break
		}
	}

	return fmt.Sprintf(
		"mock_response messages=%d tools=%d latest_user=%q",
		len(request.Messages),
		len(request.Tools),
		latestUser,
	)
}
